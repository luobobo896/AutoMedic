package api

import (
	"strings"

	"github.com/automedic/automedic/internal/model"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) ListDicts(c *gin.Context) {
	var list []model.DictItem
	q := h.db.Model(&model.DictItem{})
	if g := strings.TrimSpace(c.Query("group")); g != "" {
		q = q.Where("\"group\" = ?", g)
	}
	if c.Query("all") != "1" {
		q = q.Where("enabled = ?", true)
	}
	if err := q.Order("\"group\" ASC, sort ASC, id ASC").Find(&list).Error; err != nil {
		ServerError(c, err)
		return
	}
	OK(c, gin.H{"list": list, "groups": dictGroups()})
}

func (h *Handlers) CreateDict(c *gin.Context) {
	var in model.DictItem
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err)
		return
	}
	in.Group = strings.TrimSpace(in.Group)
	in.Value = strings.TrimSpace(in.Value)
	if in.Group == "" || in.Value == "" {
		BadRequest(c, "分组与取值不能为空")
		return
	}
	if in.Label == "" {
		in.Label = in.Value
	}
	in.Enabled = true
	if err := h.db.Create(&in).Error; err != nil {
		BadRequest(c, err)
		return
	}
	OK(c, in)
}

func (h *Handlers) UpdateDict(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var it model.DictItem
	if err := h.db.First(&it, id).Error; err != nil {
		NotFound(c, "字典项不存在")
		return
	}
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		BadRequest(c, err)
		return
	}
	updates := pickUpdates(body, "label", "sort", "enabled", "value")
	if raw, ok := body["extra"]; ok {
		js, err := extraParamsJSON(raw)
		if err != nil {
			BadRequest(c, err)
			return
		}
		updates["extra"] = js
	}
	if len(updates) > 0 {
		if err := h.db.Model(&it).Updates(updates).Error; err != nil {
			BadRequest(c, err)
			return
		}
	}
	OK(c, it)
}

func (h *Handlers) DeleteDict(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	if err := h.db.Delete(&model.DictItem{}, id).Error; err != nil {
		ServerError(c, err)
		return
	}
	OK(c, gin.H{"deleted": id})
}

func dictGroups() []gin.H {
	return []gin.H{
		{"key": "log_level", "name": "日志级别"},
		{"key": "hit_keyword", "name": "命中关键字"},
		{"key": "exclude_keyword", "name": "排除关键字"},
		{"key": "event_source", "name": "事件来源"},
		{"key": "language", "name": "仓库语言"},
		{"key": "git_branch", "name": "Git 分支"},
		{"key": "code_path", "name": "关注路径"},
		{"key": "window_sec", "name": "频次窗口（秒）"},
		{"key": "cooldown_sec", "name": "冷却时间（秒）"},
		{"key": "temperature", "name": "模型温度"},
		{"key": "provider_kind", "name": "厂家类型"},
		{"key": "model_slug", "name": "模型标识"},
	}
}
