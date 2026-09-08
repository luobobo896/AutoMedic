package api

import (
	"errors"
	"strings"

	"github.com/automedic/automedic/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
	if err := h.resolveDictParent(&in); err != nil {
		BadRequest(c, err)
		return
	}
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
	updates := pickUpdates(body, "label", "sort", "enabled", "value", "parent_id")
	if raw, ok := body["extra"]; ok {
		js, err := extraParamsJSON(raw)
		if err != nil {
			BadRequest(c, err)
			return
		}
		updates["extra"] = js
	}
	if raw, ok := updates["parent_id"]; ok {
		pid, err := coerceParentID(raw)
		if err != nil {
			BadRequest(c, err)
			return
		}
		if pid == 0 {
			updates["parent_id"] = nil
		} else {
			if err := h.validateDictParent(it.Group, pid); err != nil {
				BadRequest(c, err)
				return
			}
			updates["parent_id"] = pid
		}
	}
	if len(updates) > 0 {
		if err := h.db.Model(&it).Updates(updates).Error; err != nil {
			BadRequest(c, err)
			return
		}
	}
	if err := h.db.First(&it, id).Error; err != nil {
		ServerError(c, err)
		return
	}
	OK(c, it)
}

func (h *Handlers) DeleteDict(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var n int64
	if err := h.db.Model(&model.DictItem{}).Where("parent_id = ?", id).Count(&n).Error; err != nil {
		ServerError(c, err)
		return
	}
	if n > 0 {
		BadRequest(c, "请先删除或移走子选项")
		return
	}
	if err := h.db.Delete(&model.DictItem{}, id).Error; err != nil {
		ServerError(c, err)
		return
	}
	OK(c, gin.H{"deleted": id})
}

func (h *Handlers) resolveDictParent(in *model.DictItem) error {
	if in.ParentID != nil && *in.ParentID > 0 {
		return h.validateDictParent(in.Group, *in.ParentID)
	}
	if in.Group != "model_slug" {
		return nil
	}
	kind := extraKind(in.Extra)
	if kind == "" {
		return nil
	}
	var p model.DictItem
	if err := h.db.Where(`"group" = ? AND value = ?`, "provider_kind", kind).First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	in.ParentID = &p.ID
	return nil
}

func (h *Handlers) validateDictParent(childGroup string, pid uint) error {
	if pid == 0 {
		return nil
	}
	var p model.DictItem
	if err := h.db.First(&p, pid).Error; err != nil {
		return errors.New("父级选项不存在")
	}
	if childGroup == "model_slug" && p.Group != "provider_kind" {
		return errors.New("模型标识的父级必须是厂家类型")
	}
	if p.Group == childGroup {
		return errors.New("不能把同组选项设为父级")
	}
	return nil
}

func coerceParentID(raw any) (uint, error) {
	switch v := raw.(type) {
	case nil:
		return 0, nil
	case float64:
		if v < 0 {
			return 0, errors.New("parent_id 非法")
		}
		return uint(v), nil
	case int:
		if v < 0 {
			return 0, errors.New("parent_id 非法")
		}
		return uint(v), nil
	case uint:
		return v, nil
	default:
		return 0, errors.New("parent_id 非法")
	}
}

func extraKind(j model.JSON) string {
	var m map[string]any
	if err := j.Unmarshal(&m); err != nil || m == nil {
		return ""
	}
	v, _ := m["kind"].(string)
	return strings.TrimSpace(v)
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
		{"key": "provider_kind", "name": "厂家类型", "child_group": "model_slug"},
		{"key": "model_slug", "name": "模型标识", "parent_group": "provider_kind"},
	}
}
