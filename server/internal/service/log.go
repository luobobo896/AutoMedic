package service

import (
	"log/slog"
	"sync"
	"time"

	"github.com/automedic/automedic/internal/model"
	"github.com/automedic/automedic/internal/ws"
	"gorm.io/gorm"
)

// LogWriter 终端日志采集：一行输出 → WS 实时广播 + DB 批量落库
type LogWriter struct {
	db     *gorm.DB
	hub    *ws.Hub
	taskID uint

	mu     sync.Mutex
	buf    []model.TaskLog
	seq    int64
	closed bool
}

func NewLogWriter(db *gorm.DB, hub *ws.Hub, taskID uint) *LogWriter {
	w := &LogWriter{db: db, hub: hub, taskID: taskID}
	var last model.TaskLog
	if err := db.Where("task_id = ?", taskID).Order("seq DESC").First(&last).Error; err == nil {
		w.seq = last.Seq
	}
	go w.loop()
	return w
}

// Write 接收一行输出（stream: stdout | stderr | sys）
func (w *LogWriter) Write(stream, line string) {
	w.mu.Lock()
	w.seq++
	entry := model.TaskLog{TaskID: w.taskID, Seq: w.seq, Stream: stream, Content: line}
	w.buf = append(w.buf, entry)
	flush := len(w.buf) >= 30
	seq := w.seq
	w.mu.Unlock()

	// 带上 seq，前端可据此与轮询结果去重
	w.hub.Publish(ws.Message{Type: "log", TaskID: w.taskID, Stream: stream, Content: line, Seq: seq})
	if flush {
		w.Flush()
	}
}

// Flush 立即落库
func (w *LogWriter) Flush() {
	w.mu.Lock()
	if len(w.buf) == 0 {
		w.mu.Unlock()
		return
	}
	batch := w.buf
	w.buf = nil
	w.mu.Unlock()
	if err := w.db.Create(&batch).Error; err != nil {
		slog.Error("write task log failed", "task", w.taskID, "err", err)
	}
}

// Close 结束采集
func (w *LogWriter) Close() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	w.mu.Unlock()
	w.Flush()
}

func (w *LogWriter) loop() {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	for range t.C {
		w.mu.Lock()
		if w.closed {
			w.mu.Unlock()
			return
		}
		w.mu.Unlock()
		w.Flush()
	}
}

// 便捷构造：把 LogWriter 转成 execx.Sink
func (w *LogWriter) Sink() func(string, string) {
	return func(stream, line string) { w.Write(stream, line) }
}
