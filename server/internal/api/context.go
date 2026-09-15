package api

import (
	"net/http"

	"github.com/automedic/automedic/internal/auth"
	"github.com/automedic/automedic/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// tenant 当前生效租户 ID
func (h *Handlers) tenant(c *gin.Context) uint {
	if h.auth == nil {
		return 0
	}
	return auth.TenantID(c)
}

// tdb 按租户过滤的 DB 会话（业务表均有 tenant_id 列）
func (h *Handlers) tdb(c *gin.Context) *gorm.DB {
	return h.tdbOn(c, "")
}

// tdbOn 在 JOIN 查询里必须带表名，否则 PostgreSQL 报 tenant_id 不明确。
func (h *Handlers) tdbOn(c *gin.Context, table string) *gorm.DB {
	tid := h.tenant(c)
	if tid == 0 {
		// 平台超管未指定租户时不做过滤（跨租户视图）
		return h.db
	}
	col := "tenant_id"
	if table != "" {
		col = table + ".tenant_id"
	}
	return h.db.Where(col+" = ?", tid)
}

// principal 当前登录主体
func (h *Handlers) principal(c *gin.Context) *auth.Principal {
	return auth.Current(c)
}

// principalID 当前主体用户 ID（未登录返回 0）
func (h *Handlers) principalID(c *gin.Context) uint {
	if p := h.principal(c); p != nil {
		return p.User.ID
	}
	return 0
}

// isSuper 当前主体是否平台超管
func (h *Handlers) isSuper(c *gin.Context) bool {
	p := h.principal(c)
	return p != nil && p.IsSuper
}

// requireWriteTenant 写操作必须落在明确的租户上。
// 平台超管未通过 X-Tenant-ID 指定租户时 tenant(c)=0，直接写库会产生任何租户都看不到的 tenant_id=0 孤儿数据。
func (h *Handlers) requireWriteTenant(c *gin.Context) (uint, bool) {
	tid := h.tenant(c)
	if tid == 0 {
		Fail(c, http.StatusBadRequest, "请在 X-Tenant-ID 指定租户后再写入")
		return 0, false
	}
	return tid, true
}

// requireTenantOfCredential 校验凭证属于指定租户：仓库只能引用同租户凭证，
// 否则可借他人 git 凭证把密钥发到攻击者服务器。
func (h *Handlers) requireTenantOfCredential(c *gin.Context, tid, credentialID uint) bool {
	if credentialID == 0 {
		return true
	}
	var cd model.Credential
	if err := h.db.Select("id", "tenant_id").First(&cd, credentialID).Error; err != nil || cd.TenantID != tid {
		Fail(c, http.StatusBadRequest, "凭证不存在或不属于当前租户")
		return false
	}
	return true
}

// tenantOfProject 取项目所属租户，并校验归属
func (h *Handlers) tenantOfProject(tid, projectID uint) (uint, bool) {
	if projectID == 0 {
		return 0, false
	}
	var p model.Project
	q := h.db
	if tid != 0 {
		q = q.Where("tenant_id = ?", tid)
	}
	if err := q.Select("id", "tenant_id").First(&p, projectID).Error; err != nil {
		return 0, false
	}
	return p.TenantID, true
}

// tenantOfRepo 取仓库所属租户
func (h *Handlers) tenantOfRepo(tid, repoID uint) (uint, bool) {
	if repoID == 0 {
		return 0, false
	}
	var r model.Repository
	q := h.db
	if tid != 0 {
		q = q.Where("tenant_id = ?", tid)
	}
	if err := q.Select("id", "tenant_id").First(&r, repoID).Error; err != nil {
		return 0, false
	}
	return r.TenantID, true
}

// requireTenantOfProject 校验项目属于当前租户，返回租户 ID
func (h *Handlers) requireTenantOfProject(c *gin.Context, projectID uint) (uint, bool) {
	tid, ok := h.tenantOfProject(h.tenant(c), projectID)
	if !ok {
		Fail(c, http.StatusBadRequest, "项目不存在或不属于当前租户")
		return 0, false
	}
	return tid, true
}
