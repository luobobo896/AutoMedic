package execx

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Sink 接收流式输出；stream: stdout | stderr
type Sink func(stream string, line string)

// Spec 命令描述
type Spec struct {
	Name    string            // 展示名
	Bin     string            // 可执行文件
	Args    []string          // 参数
	Dir     string            // 工作目录
	Env     map[string]string // 追加/覆盖的环境变量（值含 __REMOVE__ 表示删除）
	Timeout time.Duration
	UsePTY  bool // 预留：是否使用伪终端（dsh 输出非 TTY 时无需）
}

// Result 执行结果
type Result struct {
	ExitCode int
	Err      error
	Output   string // 合并输出（截断保护由调用方处理）
}

// Run 流式执行命令，输出按行回调给 sink
func Run(ctx context.Context, spec Spec, sink Sink) Result {
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, spec.Bin, spec.Args...)
	cmd.Dir = spec.Dir
	if len(spec.Env) > 0 {
		cmd.Env = append(osEnvironFiltered(), buildEnv(spec.Env)...)
	}
	// 独立进程组，便于超时时整组回收
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{Err: err}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Result{Err: err}
	}

	if err := cmd.Start(); err != nil {
		return Result{Err: err}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var sb strings.Builder

	collect := func(stream string, r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			mu.Lock()
			if sb.Len() < 4*1024*1024 {
				sb.WriteString(line)
				sb.WriteString("\n")
			}
			mu.Unlock()
			if sink != nil {
				sink(stream, line)
			}
		}
	}
	wg.Add(2)
	go collect("stdout", stdout)
	go collect("stderr", stderr)
	wg.Wait()

	err = cmd.Wait()
	res := Result{Output: sb.String()}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.ExitCode = ee.ExitCode()
		} else {
			res.ExitCode = -1
		}
		res.Err = err
		if ctx.Err() == context.DeadlineExceeded {
			res.Err = fmt.Errorf("%w (%v)", context.DeadlineExceeded, spec.Timeout)
		}
	}
	slog.Debug("exec done", "bin", spec.Bin, "args", strings.Join(spec.Args, " "), "code", res.ExitCode)
	return res
}

func osEnvironFiltered() []string { return execEnviron() }

func buildEnv(kv map[string]string) []string {
	out := make([]string, 0, len(kv))
	for k, v := range kv {
		if v == "__REMOVE__" {
			continue
		}
		out = append(out, k+"="+v)
	}
	return out
}

func execEnviron() []string { return execEnvList() }

// RunSimple 无流式的简单执行，返回合并输出
func RunSimple(ctx context.Context, dir, bin string, args ...string) (string, int, error) {
	return RunSimpleEnv(ctx, dir, nil, bin, args...)
}

// RunSimpleEnv 带环境变量覆盖的简单执行
func RunSimpleEnv(ctx context.Context, dir string, env map[string]string, bin string, args ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(osEnvironFiltered(), buildEnv(env)...)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	return string(out), code, err
}
