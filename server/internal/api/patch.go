package api

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// pickUpdates 只保留白名单标量字段，丢掉关联对象 / 数组 / 只读时间戳。
// 前端编辑时常把列表行（含 nested provider/project）整包回传，直接 Updates 会触发 GORM invalid field。
func pickUpdates(body map[string]any, fields ...string) map[string]any {
	allow := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		allow[f] = struct{}{}
	}
	out := make(map[string]any, len(fields))
	for k, v := range body {
		if _, ok := allow[k]; !ok {
			continue
		}
		if isAssociationJSON(v) {
			continue
		}
		out[k] = v
	}
	return out
}

func isAssociationJSON(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}

func bindUpdates(c *gin.Context) (map[string]any, bool) {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		BadRequest(c, err)
		return nil, false
	}
	return body, true
}

func (h *Handlers) saveUpdates(c *gin.Context, db *gorm.DB, dest any, fields ...string) bool {
	body, ok := bindUpdates(c)
	if !ok {
		return false
	}
	updates := pickUpdates(body, fields...)
	if len(updates) == 0 {
		return true
	}
	if err := db.Model(dest).Updates(updates).Error; err != nil {
		BadRequest(c, err)
		return false
	}
	return true
}
