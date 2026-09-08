package api

import (
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/automedic/automedic/internal/crypto"
	"github.com/automedic/automedic/internal/model"
	"github.com/automedic/automedic/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// IngestEvent 外部采集器投递接口（令牌鉴权）
func (h *Handlers) IngestEvent(c *gin.Context) {
	var in service.IngestInput
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err)
		return
	}
	tok, err := h.authenticateToken(c)
	if err != nil {
		Fail(c, 401, err.Error())
		return
	}
	res, err := service.Ingest(h.db, tok.ProjectID, &tok.ID, &in)
	if err != nil {
		BadRequest(c, err)
		return
	}
	now := time.Now()
	h.db.Model(&model.IngestToken{}).Where("id = ?", tok.ID).Update("last_used_at", now)
	for _, id := range res.TaskIDs {
		h.exec.Enqueue(id)
	}
	OK(c, res)
}

func (h *Handlers) IngestEventBatch(c *gin.Context) {
	var body struct {
		Events []service.IngestInput `json:"events"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		BadRequest(c, err)
		return
	}
	tok, err := h.authenticateToken(c)
	if err != nil {
		Fail(c, 401, err.Error())
		return
	}
	if len(body.Events) == 0 || len(body.Events) > 100 {
		BadRequest(c, "单批事件数量需在 1~100 之间")
		return
	}
	out := make([]*service.IngestResult, 0, len(body.Events))
	for i := range body.Events {
		res, err := service.Ingest(h.db, tok.ProjectID, &tok.ID, &body.Events[i])
		if err != nil {
			out = append(out, &service.IngestResult{Action: "error", Reason: err.Error()})
			continue
		}
		for _, id := range res.TaskIDs {
			h.exec.Enqueue(id)
		}
		out = append(out, res)
	}
	OK(c, out)
}

// authenticateToken 校验 X-AM-Token / Bearer
func (h *Handlers) authenticateToken(c *gin.Context) (*model.IngestToken, error) {
	raw := c.GetHeader("X-AM-Token")
	if raw == "" {
		auth := c.GetHeader("Authorization")
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			raw = strings.TrimSpace(auth[7:])
		}
	}
	if raw == "" {
		raw = c.Query("token")
	}
	if raw == "" {
		return nil, errMissingToken
	}
	hash := crypto.SHA256(raw)
	var t model.IngestToken
	if err := h.db.Where("token_hash = ?", hash).First(&t).Error; err != nil {
		return nil, errBadToken
	}
	if !t.Enabled {
		return nil, errTokenDisabled
	}
	if t.ExpiresAt != nil && t.ExpiresAt.Before(time.Now()) {
		return nil, errTokenExpired
	}
	if t.AllowCIDR != "" && !cidrAllow(t.AllowCIDR, c.ClientIP()) {
		return nil, errIPDenied
	}
	return &t, nil
}

type ingErr string

func (e ingErr) Error() string { return string(e) }

var (
	errMissingToken  = ingErr("缺少投递令牌（X-AM-Token）")
	errBadToken      = ingErr("令牌无效")
	errTokenDisabled = ingErr("令牌已禁用")
	errTokenExpired  = ingErr("令牌已过期")
	errIPDenied      = ingErr("来源 IP 不在允许范围")
)

func cidrAllow(list, ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, item := range strings.Split(list, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			if _, n, err := net.ParseCIDR(item); err == nil && n.Contains(parsed) {
				return true
			}
			continue
		}
		if item == ip {
			return true
		}
	}
	return false
}

// ListEvents 事件列表
func (h *Handlers) ListEvents(c *gin.Context) {
	var list []model.Event
	q := h.tdbOn(c, "events").Model(&model.Event{}).Preload("Project").Preload("Rule")
	if pid := c.Query("project_id"); pid != "" {
		q = q.Where("events.project_id = ?", pid)
	}
	if status := c.Query("status"); status != "" {
		q = q.Where("events.status = ?", status)
	}
	if level := c.Query("level"); level != "" {
		q = q.Where("events.level = ?", level)
	}
	if source := c.Query("source"); source != "" {
		q = q.Where("events.source = ?", source)
	}
	if kw := c.Query("keyword"); kw != "" {
		q = q.Where("events.title LIKE ? OR events.message LIKE ?", "%"+kw+"%", "%"+kw+"%")
	}
	if days := c.Query("days"); days != "" {
		var d int
		for _, ch := range days {
			if ch >= '0' && ch <= '9' {
				d = d*10 + int(ch-'0')
			}
		}
		if d > 0 {
			q = q.Where("events.occurred_at >= ?", time.Now().AddDate(0, 0, -d))
		}
	}
	unique := c.DefaultQuery("unique", "1") != "0"
	if unique {
		latest := q.Session(&gorm.Session{}).Select("MAX(events.id)").
			Group("CASE WHEN COALESCE(events.fingerprint, '') = '' THEN CAST(events.id AS text) ELSE events.fingerprint END")
		q = q.Where("events.id IN (?)", latest)
	}
	var total int64
	q.Count(&total)
	page, size := QueryPage(c)
	if err := q.Order("events.id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error; err != nil {
		ServerError(c, err)
		return
	}
	fillEventOccurrence(h.tdbOn(c, "events"), list)
	OKPage(c, list, Page{Page: page, PageSize: size, Total: total})
}

func fillEventOccurrence(db *gorm.DB, list []model.Event) {
	fps := make([]string, 0, len(list))
	seen := map[string]struct{}{}
	for i := range list {
		fp := list[i].Fingerprint
		if fp == "" {
			list[i].OccurrenceN = 1
			continue
		}
		if _, ok := seen[fp]; ok {
			continue
		}
		seen[fp] = struct{}{}
		fps = append(fps, fp)
	}
	if len(fps) == 0 {
		return
	}
	type row struct {
		Fingerprint string
		N           int64
	}
	var rows []row
	_ = db.Model(&model.Event{}).Select("fingerprint, count(*) as n").
		Where("fingerprint IN ?", fps).Group("fingerprint").Scan(&rows).Error
	nByFP := map[string]int{}
	for _, r := range rows {
		nByFP[r.Fingerprint] = int(r.N)
	}
	for i := range list {
		if list[i].Fingerprint == "" {
			continue
		}
		if n := nByFP[list[i].Fingerprint]; n > 0 {
			list[i].OccurrenceN = n
		} else {
			list[i].OccurrenceN = 1
		}
	}
}

func (h *Handlers) GetEvent(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var ev model.Event
	if err := h.tdb(c).Preload("Project").Preload("Rule").First(&ev, id).Error; err != nil {
		NotFound(c, "事件不存在")
		return
	}
	var tasks []model.Task
	h.tdb(c).Preload("Repo").Where("event_id = ?", id).Order("id DESC").Find(&tasks)
	OK(c, gin.H{"event": ev, "tasks": tasks})
}

// ReplayEvent 手动触发事件重新走一次过滤与修复
func (h *Handlers) ReplayEvent(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var ev model.Event
	if err := h.tdb(c).First(&ev, id).Error; err != nil {
		NotFound(c, "事件不存在")
		return
	}
	if ev.Status == model.EventStatusFixed {
		BadRequest(c, "该事件已修复成功，不能重放")
		return
	}
	if ev.Status == model.EventStatusFixing {
		BadRequest(c, "该事件正在修复，不能重放")
		return
	}
	in := &service.IngestInput{
		Source: ev.Source, Level: ev.Level, Title: ev.Title,
		Message: ev.Message, Stack: ev.Stack, Fingerprint: ev.Fingerprint,
		OccurredAt: &ev.OccurredAt,
	}
	res, err := service.Ingest(h.db, ev.ProjectID, nil, in)
	if err != nil {
		BadRequest(c, err)
		return
	}
	for _, tid := range res.TaskIDs {
		h.exec.Enqueue(tid)
	}
	slog.Info("event replayed", "event", id)
	OK(c, res)
}
