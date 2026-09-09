package ws

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Message struct {
	Type    string `json:"type"` // log | status | done
	TaskID  uint   `json:"task_id"`
	Stream  string `json:"stream,omitempty"`
	Content string `json:"content,omitempty"`
	Status  string `json:"status,omitempty"`
	Stage   string `json:"stage,omitempty"`
	Seq     int64  `json:"seq,omitempty"` // 日志序号，与 GET /tasks/:id/logs 的 seq 对齐，便于去重
	TS      int64  `json:"ts"`
}

// LogLine 历史日志行
type LogLine struct {
	Seq       int64     `json:"seq"`
	Stream    string    `json:"stream"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Source 为 WebSocket 提供历史回放与初始状态，由外部注入以避免 ws 包依赖存储层
type Source interface {
	// History 返回任务的历史日志（按 seq 升序，最多 limit 条）
	History(taskID uint, afterSeq int64, limit int) []LogLine
	// Status 返回任务的当前状态与阶段
	Status(taskID uint) (status string, stage string)
}

type client struct {
	conn *websocket.Conn
	send chan []byte
	once sync.Once
	// pending 连接建立瞬间缓存的历史帧，先于 send 写出，避免超出 channel 容量被丢弃
	pending [][]byte
}

// Hub 任务日志广播中心
type Hub struct {
	mu      sync.RWMutex
	clients map[uint]map[*client]struct{}
	src     Source
}

func NewHub() *Hub {
	return &Hub{clients: map[uint]map[*client]struct{}{}}
}

// SetSource 注入历史回放数据源（可选；未注入则不回放历史）
func (h *Hub) SetSource(s Source) {
	h.mu.Lock()
	h.src = s
	h.mu.Unlock()
}

func (h *Hub) source() Source {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.src
}

func (h *Hub) subscribe(taskID uint) *client {
	c := &client{send: make(chan []byte, 512)}
	h.mu.Lock()
	if h.clients[taskID] == nil {
		h.clients[taskID] = map[*client]struct{}{}
	}
	h.clients[taskID][c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *Hub) unsubscribe(taskID uint, c *client) {
	h.mu.Lock()
	if m, ok := h.clients[taskID]; ok {
		delete(m, c)
		if len(m) == 0 {
			delete(h.clients, taskID)
		}
		// 仅当从 map 成功移除时才关闭 channel；用 once 兜底，杜绝重复 close 导致 panic
		c.once.Do(func() { close(c.send) })
	}
	h.mu.Unlock()
}

// Publish 广播消息给订阅该任务的客户端
func (h *Hub) Publish(msg Message) {
	if msg.TS == 0 {
		msg.TS = time.Now().UnixMilli()
	}
	payload, err := jsonMarshal(msg)
	if err != nil {
		return
	}
	h.mu.RLock()
	targets := make([]*client, 0, 4)
	for c := range h.clients[msg.TaskID] {
		targets = append(targets, c)
	}
	h.mu.RUnlock()
	for _, c := range targets {
		select {
		case c.send <- payload:
		default:
			// 客户端消费过慢，丢弃该帧，避免阻塞修复流程
			slog.Debug("ws client slow, drop frame", "task", msg.TaskID)
		}
	}
}

// Count 订阅数（用于判断是否有人在实时观看）
func (h *Hub) Count(taskID uint) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[taskID])
}

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	Subprotocols:    []string{"automedic"},
}

// ServeTask 处理 /ws/tasks/:id 订阅
func (h *Hub) ServeTask(c *gin.Context) {
	taskID, ok := parseID(c.Param("id"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id"})
		return
	}
	// 支持通过 WebSocket 子协议传递 JWT：Sec-WebSocket-Protocol: automedic.<token>
	// 避免 token 出现在 URL query（会进入访问日志）。缺失时保留 ?token= 回退路径。
	if sub := c.Request.Header.Get("Sec-WebSocket-Protocol"); sub != "" {
		var protocols []string
		for _, part := range strings.Split(sub, ",") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "automedic.") {
				token := strings.TrimPrefix(part, "automedic.")
				c.Request.Header.Set("Authorization", "Bearer "+token)
				protocols = append(protocols, "automedic")
			} else if part == "automedic" {
				protocols = append(protocols, "automedic")
			}
		}
		if len(protocols) > 0 {
			c.Request.Header.Set("Sec-WebSocket-Protocol", strings.Join(protocols, ", "))
		}
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	cl := h.subscribe(taskID)
	defer h.unsubscribe(taskID, cl)

	// 建连瞬间回放：先补一帧当前状态，再回放已有日志，保证打开页面即可看到完整过程
	h.fillBacklog(taskID, cl)

	done := make(chan struct{})
	go func() {
		defer close(done)
		// 先写出 backlog（历史帧），再进入实时广播
		for _, b := range cl.pending {
			if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
				return
			}
		}
		cl.pending = nil
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case msg, ok := <-cl.send:
				if !ok {
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// 读循环：客户端关闭时退出
	go func() {
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				h.unsubscribe(taskID, cl)
				return
			}
		}
	}()
	<-done
	conn.Close()
}

// fillBacklog 在 writer goroutine 启动前填充历史帧，避免并发写 conn
func (h *Hub) fillBacklog(taskID uint, cl *client) {
	src := h.source()
	if src == nil {
		return
	}
	now := time.Now().UnixMilli()
	push := func(m Message) {
		if m.TS == 0 {
			m.TS = now
		}
		b, err := jsonMarshal(m)
		if err == nil {
			cl.pending = append(cl.pending, b)
		}
	}
	if status, stage := src.Status(taskID); status != "" {
		push(Message{Type: "status", TaskID: taskID, Status: status, Stage: stage})
	}
	for _, l := range src.History(taskID, 0, historyLimit) {
		push(Message{Type: "log", TaskID: taskID, Stream: l.Stream, Content: l.Content,
			Seq: l.Seq, TS: l.CreatedAt.UnixMilli()})
	}
	push(Message{Type: "backlog_end", TaskID: taskID})
}

// historyLimit 建连时回放的历史日志上限
const historyLimit = 5000
