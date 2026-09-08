package ocr

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const sessionPoll = 400 * time.Millisecond

type sessionState struct {
	lastFile string
	lastKind string
	doneN    int
}

func shortReviewPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	parts := strings.Split(p, "/")
	if len(parts) <= 2 {
		return p
	}
	return strings.Join(parts[len(parts)-2:], "/")
}

func formatElapsed(ms int64) string {
	if ms < 1000 {
		return "不到 1 秒"
	}
	s := (ms + 500) / 1000
	if s < 60 {
		return strconv.FormatInt(s, 10) + " 秒"
	}
	m := s / 60
	rem := s % 60
	if rem == 0 {
		return strconv.FormatInt(m, 10) + " 分"
	}
	return strconv.FormatInt(m, 10) + " 分 " + strconv.FormatInt(rem, 10) + " 秒"
}

// SummarizeSessionLine 把 OCR session jsonl 一行转成给人看的进度；无关事件返回空串。
func SummarizeSessionLine(line string, st *sessionState) string {
	if st == nil {
		st = &sessionState{}
	}
	line = strings.TrimSpace(line)
	if line == "" || line[0] != '{' {
		return ""
	}
	var ev map[string]any
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return ""
	}
	typ, _ := ev["type"].(string)
	fp, _ := ev["filePath"].(string)
	model, _ := ev["model"].(string)
	switch typ {
	case "session_start":
		st.lastKind = typ
		return "OCR 会话已启动，开始按文件审查"
	case "llm_request":
		if strings.Contains(fp, "dedup") {
			st.lastKind = "dedup"
			return "正在合并重复意见"
		}
		if strings.Contains(fp, "summary") {
			st.lastKind = "summary"
			return "正在生成项目摘要"
		}
		if fp == "" {
			return ""
		}
		if st.lastFile == fp && st.lastKind == "llm_request" {
			return ""
		}
		st.lastFile, st.lastKind = fp, "llm_request"
		name := shortReviewPath(fp)
		if model != "" {
			return "正在审查 " + name + "（" + model + "）"
		}
		return "正在审查 " + name
	case "review_item_done":
		st.doneN++
		st.lastKind = typ
		name := shortReviewPath(fp)
		if name == "" {
			return "已完成 " + strconv.Itoa(st.doneN) + " 个文件"
		}
		return "已完成 " + name + "（第 " + strconv.Itoa(st.doneN) + " 个文件）"
	case "llm_response":
		dur := jsonNumber(ev["duration_ms"])
		if dur >= 15000 && fp != "" && !strings.HasPrefix(fp, "__") {
			return shortReviewPath(fp) + " 模型响应 " + formatElapsed(dur)
		}
		return ""
	default:
		return ""
	}
}

func jsonNumber(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}

func findSessionJSONL(workDir string) string {
	base := filepath.Base(workDir)
	home, _ := os.UserHomeDir()
	if home == "" {
		home = os.Getenv("HOME")
	}
	dirs := []string{
		filepath.Join(home, ".opencodereview", "sessions", "tmp-"+base),
		filepath.Join(home, ".opencodereview", "sessions", base),
	}
	for _, dir := range dirs {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
		if len(matches) > 0 {
			return matches[0]
		}
	}
	return ""
}

// watchSession 在 ocr 进程运行期间读 ~/.opencodereview/sessions jsonl，把进度交给 sink。
func watchSession(ctx context.Context, workDir string, sink func(string)) {
	if sink == nil {
		return
	}
	st := &sessionState{}
	var path string
	var offset int64
	lastEmit := time.Now()
	lastFile := ""
	started := time.Now()
	t := time.NewTicker(sessionPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if path == "" {
			path = findSessionJSONL(workDir)
			if path == "" {
				if time.Since(started) >= 3*time.Second && time.Since(lastEmit) >= 3*time.Second {
					sink("正在等待 OCR 开始输出进度…")
					lastEmit = time.Now()
				}
				continue
			}
		}
		n, last := readNewSessionLines(path, offset, st, sink)
		offset += n
		if last != "" {
			lastFile = last
			lastEmit = time.Now()
		} else if lastFile != "" && time.Since(lastEmit) >= 5*time.Second {
			sink("等待模型返回：" + lastFile + " · 已 " + formatElapsed(time.Since(started).Milliseconds()))
			lastEmit = time.Now()
		}
	}
}

func readNewSessionLines(path string, offset int64, st *sessionState, sink func(string)) (int64, string) {
	f, err := os.Open(path)
	if err != nil {
		return 0, ""
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return 0, ""
	}
	r := bufio.NewReader(f)
	var n int64
	lastFile := ""
	for {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			n += int64(len(line))
			msg := SummarizeSessionLine(strings.TrimRight(line, "\r\n"), st)
			if msg != "" {
				sink(msg)
				if st.lastFile != "" {
					lastFile = shortReviewPath(st.lastFile)
				}
			}
		}
		if err != nil {
			break
		}
	}
	return n, lastFile
}
