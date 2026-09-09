package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Init 初始化全局日志。
// dir 为空时行为与原来完全一致：仅写 os.Stdout。
// dir 非空时：创建目录，按天打开 dir/app-YYYY-MM-DD.log（追加），同时写 stdout；
// 启动时清理 mtime 早于 retain 天的 app-*.log（retain<=0 按 14 天）。
// 打开/清理失败仅告警并回退到仅写 stdout，不会让进程启动失败。
func Init(level, format, dir string, retain int) {
	lvl := slog.LevelInfo
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: lvl}

	var out io.Writer = os.Stdout
	if dir != "" {
		if f, ok := openLogFile(dir, retain); ok {
			out = io.MultiWriter(os.Stdout, f)
		}
	}

	var h slog.Handler
	if strings.EqualFold(format, "json") {
		h = slog.NewJSONHandler(out, opts)
	} else {
		h = slog.NewTextHandler(out, opts)
	}
	slog.SetDefault(slog.New(h))
}

// openLogFile 创建目录、打开当日日志文件、清理过期文件。返回文件与是否成功。
func openLogFile(dir string, retain int) (*os.File, bool) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Warn("日志目录创建失败，回退仅写 stdout", "dir", dir, "err", err)
		return nil, false
	}
	cleanOldLogs(dir, retain)

	name := filepath.Join(dir, "app-"+time.Now().Format("2006-01-02")+".log")
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		slog.Warn("日志文件打开失败，回退仅写 stdout", "file", name, "err", err)
		return nil, false
	}
	return f, true
}

// cleanOldLogs 删除 dir 下 mtime 早于 retain 天的 app-*.log。
func cleanOldLogs(dir string, retain int) {
	if retain <= 0 {
		retain = 14
	}
	cut := time.Now().AddDate(0, 0, -retain)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "app-") || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cut) {
			p := filepath.Join(dir, e.Name())
			if err := os.Remove(p); err != nil {
				slog.Warn("过期日志删除失败", "file", p, "err", err)
			} else {
				slog.Info("已清理过期日志", "file", p)
			}
		}
	}
}
