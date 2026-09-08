package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/automedic/automedic/internal/auth"
	"github.com/automedic/automedic/internal/model"
	"github.com/automedic/automedic/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ---------- 登录 / 令牌 ----------

type loginIn struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handlers) Login(c *gin.Context) {
	var in loginIn
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err)
		return
	}
	if in.Username == "" || in.Password == "" {
		BadRequest(c, "请输入账号与密码")
		return
	}
	p, access, refresh, err := h.auth.Login(in.Username, in.Password, c.ClientIP(), c.GetHeader("User-Agent"))
	if err != nil {
		Fail(c, http.StatusUnauthorized, err.Error())
		return
	}
	OK(c, gin.H{"token": access, "refresh_token": refresh, "user": profileOf(p)})
}

func (h *Handlers) RefreshToken(c *gin.Context) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = c.ShouldBindJSON(&in)
	if in.RefreshToken == "" {
		in.RefreshToken = c.Query("refresh_token")
	}
	p, access, refresh, err := h.auth.Refresh(in.RefreshToken)
	if err != nil {
		Fail(c, http.StatusUnauthorized, err.Error())
		return
	}
	OK(c, gin.H{"token": access, "refresh_token": refresh, "user": profileOf(p)})
}

func (h *Handlers) Logout(c *gin.Context) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = c.ShouldBindJSON(&in)
	h.auth.Logout(in.RefreshToken)
	OK(c, gin.H{"ok": true})
}

// Profile 当前登录用户信息（含角色与权限）
func (h *Handlers) Profile(c *gin.Context) {
	p := h.principal(c)
	if p == nil {
		Fail(c, http.StatusUnauthorized, "未登录")
		return
	}
	OK(c, profileOf(p))
}

func profileOf(p *auth.Principal) gin.H {
	u := p.User
	u.PasswordHash = ""
	return gin.H{
		"id": u.ID, "username": u.Username, "display_name": u.DisplayName,
		"email": u.Email, "status": u.Status, "is_super": p.IsSuper,
		"tenant_id": p.TenantID, "roles": p.Roles, "permissions": p.Permissions,
	}
}

// ChangePassword 修改自己的密码
func (h *Handlers) ChangePassword(c *gin.Context) {
	p := h.principal(c)
	if p == nil {
		Fail(c, http.StatusUnauthorized, "未登录")
		return
	}
	var in struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err)
		return
	}
	if len(in.NewPassword) < 6 {
		BadRequest(c, "新密码至少 6 位")
		return
	}
	if !auth.CheckPassword(p.User.PasswordHash, in.OldPassword) {
		Fail(c, http.StatusBadRequest, "原密码不正确")
		return
	}
	hash, err := auth.HashPassword(in.NewPassword)
	if err != nil {
		ServerError(c, err)
		return
	}
	if err := h.db.Model(&model.User{}).Where("id = ?", p.User.ID).Update("password_hash", hash).Error; err != nil {
		ServerError(c, err)
		return
	}
	OK(c, gin.H{"updated": true})
}

// ---------- 用户管理 ----------

func (h *Handlers) ListUsers(c *gin.Context) {
	var list []model.User
	q := h.db.Preload("Roles").Preload("Tenant")
	if tid := h.tenant(c); tid != 0 {
		q = q.Where("tenant_id = ?", tid)
	}
	if kw := c.Query("keyword"); kw != "" {
		q = q.Where("username LIKE ? OR display_name LIKE ?", "%"+kw+"%", "%"+kw+"%")
	}
	var total int64
	q.Model(&model.User{}).Count(&total)
	page, size := QueryPage(c)
	if err := q.Order("id ASC").Offset((page - 1) * size).Limit(size).Find(&list).Error; err != nil {
		ServerError(c, err)
		return
	}
	for i := range list {
		list[i].PasswordHash = ""
	}
	OKPage(c, list, Page{Page: page, PageSize: size, Total: total})
}

type userIn struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	TenantID    uint   `json:"tenant_id"`
	Status      string `json:"status"`
	IsSuper     bool   `json:"is_super"`
	RoleIDs     []uint `json:"role_ids"`
}

func (h *Handlers) CreateUser(c *gin.Context) {
	var in userIn
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err)
		return
	}
	if in.Username == "" || in.Password == "" {
		BadRequest(c, "账号与密码不能为空")
		return
	}
	if len(in.Password) < 6 {
		BadRequest(c, "密码至少 6 位")
		return
	}
	tid := in.TenantID
	if p := h.principal(c); p != nil && !p.IsSuper {
		tid = p.TenantID // 非超管只能在自己租户下建用户
	}
	if tid == 0 {
		BadRequest(c, "必须指定租户")
		return
	}
	var n int64
	h.db.Model(&model.User{}).Where("username = ?", in.Username).Count(&n)
	if n > 0 {
		BadRequest(c, "账号已存在")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		ServerError(c, err)
		return
	}
	if in.Status == "" {
		in.Status = "active"
	}
	u := model.User{
		TenantID: tid, Username: in.Username, PasswordHash: hash,
		DisplayName: in.DisplayName, Email: in.Email, Status: in.Status, IsSuper: in.IsSuper,
	}
	if err := h.db.Create(&u).Error; err != nil {
		BadRequest(c, err)
		return
	}
	h.replaceUserRoles(u.ID, in.RoleIDs)
	u.PasswordHash = ""
	OK(c, u)
}

func (h *Handlers) UpdateUser(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var u model.User
	q := h.db
	if tid := h.tenant(c); tid != 0 {
		q = q.Where("tenant_id = ?", tid)
	}
	if err := q.First(&u, id).Error; err != nil {
		NotFound(c, "用户不存在")
		return
	}
	var in userIn
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err)
		return
	}
	if in.DisplayName != "" {
		u.DisplayName = in.DisplayName
	}
	if in.Email != "" {
		u.Email = in.Email
	}
	if in.Status != "" {
		u.Status = in.Status
	}
	if h.principal(c) != nil && h.principal(c).IsSuper {
		if in.IsSuper {
			u.IsSuper = true
		}
		if in.TenantID != 0 {
			u.TenantID = in.TenantID
		}
	}
	if in.Username != "" && in.Username != u.Username {
		var n int64
		h.db.Model(&model.User{}).Where("username = ? AND id <> ?", in.Username, u.ID).Count(&n)
		if n > 0 {
			BadRequest(c, "账号已存在")
			return
		}
		u.Username = in.Username
	}
	if in.Password != "" {
		if len(in.Password) < 6 {
			BadRequest(c, "密码至少 6 位")
			return
		}
		hash, err := auth.HashPassword(in.Password)
		if err != nil {
			ServerError(c, err)
			return
		}
		u.PasswordHash = hash
	}
	if err := h.db.Save(&u).Error; err != nil {
		BadRequest(c, err)
		return
	}
	if in.RoleIDs != nil {
		h.replaceUserRoles(u.ID, in.RoleIDs)
	}
	u.PasswordHash = ""
	OK(c, u)
}

func (h *Handlers) DeleteUser(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	if p := h.principal(c); p != nil && p.User.ID == id {
		BadRequest(c, "不能删除当前登录账号")
		return
	}
	q := h.db
	if tid := h.tenant(c); tid != 0 {
		q = q.Where("tenant_id = ?", tid)
	}
	if err := q.Delete(&model.User{}, id).Error; err != nil {
		ServerError(c, err)
		return
	}
	h.db.Where("user_id = ?", id).Delete(&model.UserRole{})
	OK(c, gin.H{"deleted": id})
}

func (h *Handlers) replaceUserRoles(userID uint, roleIDs []uint) {
	if roleIDs == nil {
		return
	}
	h.db.Where("user_id = ?", userID).Delete(&model.UserRole{})
	for _, rid := range roleIDs {
		h.db.Where("user_id = ? AND role_id = ?", userID, rid).
			FirstOrCreate(&model.UserRole{}, model.UserRole{UserID: userID, RoleID: rid})
	}
}

// ---------- 角色管理 ----------

func (h *Handlers) ListRoles(c *gin.Context) {
	var list []model.Role
	tid := h.tenant(c)
	q := h.db
	p := h.principal(c)
	if tid != 0 {
		if p != nil && p.IsSuper {
			q = q.Where("tenant_id = ? OR tenant_id = 0", tid)
		} else {
			q = q.Where("tenant_id = ?", tid)
		}
	}
	if err := q.Order("tenant_id ASC, id ASC").Find(&list).Error; err != nil {
		ServerError(c, err)
		return
	}
	var perms []model.RolePermission
	h.db.Find(&perms)
	byRole := map[uint][]string{}
	for _, p := range perms {
		byRole[p.RoleID] = append(byRole[p.RoleID], p.Code)
	}
	for i := range list {
		list[i].Permissions = byRole[list[i].ID]
		if list[i].Builtin {
			if def, ok := model.BuiltinRoles[list[i].Code]; ok {
				list[i].DefaultPermissions = def
			}
		}
	}
	OK(c, list)
}

func (h *Handlers) ListPermissions(c *gin.Context) {
	OK(c, model.PermissionCatalog)
}

type roleIn struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Remark      string   `json:"remark"`
	Permissions []string `json:"permissions"`
}

func (h *Handlers) CreateRole(c *gin.Context) {
	var in roleIn
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err)
		return
	}
	if in.Code == "" || in.Name == "" {
		BadRequest(c, "角色编码与名称不能为空")
		return
	}
	tid := h.tenant(c)
	var n int64
	h.db.Model(&model.Role{}).Where("tenant_id = ? AND code = ?", tid, in.Code).Count(&n)
	if n > 0 {
		BadRequest(c, "该角色编码已存在")
		return
	}
	r := model.Role{TenantID: tid, Code: in.Code, Name: in.Name, Remark: in.Remark}
	if err := h.db.Create(&r).Error; err != nil {
		BadRequest(c, err)
		return
	}
	if len(in.Permissions) > 0 {
		store.SetRolePermissions(h.db, r.ID, in.Permissions)
	}
	OK(c, r)
}

func (h *Handlers) UpdateRole(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	tid := h.tenant(c)
	var r model.Role
	q := h.db
	if tid != 0 {
		q = q.Where("tenant_id = ?", tid)
	}
	if err := q.First(&r, id).Error; err != nil {
		NotFound(c, "角色不存在")
		return
	}
	var in roleIn
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err)
		return
	}
	if in.Name != "" {
		r.Name = in.Name
	}
	if in.Remark != "" {
		r.Remark = in.Remark
	}
	if err := h.db.Save(&r).Error; err != nil {
		BadRequest(c, err)
		return
	}
	// 平台超管始终全量权限（鉴权还走 IsSuper）；其余角色（含内置）按提交覆盖。
	if r.Code == model.RoleSuperAdmin && r.TenantID == 0 {
		if err := store.SetRolePermissions(h.db, r.ID, model.AllPermissions()); err != nil {
			ServerError(c, err)
			return
		}
	} else if in.Permissions != nil {
		if err := store.SetRolePermissions(h.db, r.ID, in.Permissions); err != nil {
			ServerError(c, err)
			return
		}
	}
	var perms []string
	h.db.Model(&model.RolePermission{}).Where("role_id = ?", r.ID).Pluck("code", &perms)
	r.Permissions = perms
	OK(c, r)
}

func (h *Handlers) DeleteRole(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var r model.Role
	if err := h.db.First(&r, id).Error; err != nil {
		NotFound(c, "角色不存在")
		return
	}
	if r.Builtin {
		BadRequest(c, "内置角色不可删除")
		return
	}
	if tid := h.tenant(c); tid != 0 && r.TenantID != tid {
		NotFound(c, "角色不存在")
		return
	}
	var n int64
	h.db.Model(&model.UserRole{}).Where("role_id = ?", id).Count(&n)
	if n > 0 {
		BadRequest(c, "仍有用户绑定该角色，请先解除")
		return
	}
	h.db.Where("role_id = ?", id).Delete(&model.RolePermission{})
	if err := h.db.Delete(&model.Role{}, id).Error; err != nil {
		ServerError(c, err)
		return
	}
	OK(c, gin.H{"deleted": id})
}

// ---------- 租户管理（平台级） ----------

func (h *Handlers) ListTenants(c *gin.Context) {
	var list []model.Tenant
	if err := h.db.Order("id ASC").Find(&list).Error; err != nil {
		ServerError(c, err)
		return
	}
	type item struct {
		model.Tenant
		UserCount int64 `json:"user_count"`
		ProjCount int64 `json:"project_count"`
	}
	out := make([]item, 0, len(list))
	for _, t := range list {
		it := item{Tenant: t}
		h.db.Model(&model.User{}).Where("tenant_id = ?", t.ID).Count(&it.UserCount)
		h.db.Model(&model.Project{}).Where("tenant_id = ?", t.ID).Count(&it.ProjCount)
		out = append(out, it)
	}
	OK(c, out)
}

func (h *Handlers) CreateTenant(c *gin.Context) {
	var in struct {
		Name   string `json:"name"`
		Key    string `json:"key"`
		Remark string `json:"remark"`
		// 是否同时创建管理员
		AdminUsername string `json:"admin_username"`
		AdminPassword string `json:"admin_password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err)
		return
	}
	if in.Name == "" || in.Key == "" {
		BadRequest(c, "租户名称与标识不能为空")
		return
	}
	var n int64
	h.db.Model(&model.Tenant{}).Where("key = ?", in.Key).Count(&n)
	if n > 0 {
		BadRequest(c, "租户标识已存在")
		return
	}
	t := model.Tenant{Name: in.Name, Key: in.Key, Status: "active", Remark: in.Remark}
	if err := h.db.Create(&t).Error; err != nil {
		BadRequest(c, err)
		return
	}
	if err := store.CreateTenantRoles(h.db, t.ID); err != nil {
		slog.Warn("创建租户内置角色失败", "err", err)
	}
	// 可选：同时创建租户管理员
	if in.AdminUsername != "" && in.AdminPassword != "" {
		if len(in.AdminPassword) < 6 {
			BadRequest(c, "管理员密码至少 6 位")
			return
		}
		var cnt int64
		h.db.Model(&model.User{}).Where("username = ?", in.AdminUsername).Count(&cnt)
		if cnt > 0 {
			BadRequest(c, "管理员账号已存在")
			return
		}
		hash, err := auth.HashPassword(in.AdminPassword)
		if err != nil {
			ServerError(c, err)
			return
		}
		u := model.User{TenantID: t.ID, Username: in.AdminUsername, PasswordHash: hash,
			DisplayName: in.Name + " 管理员", Status: "active"}
		if err := h.db.Create(&u).Error; err != nil {
			BadRequest(c, err)
			return
		}
		var admin model.Role
		h.db.Where("tenant_id = ? AND code = ?", t.ID, model.RoleTenantAdmin).First(&admin)
		if admin.ID != 0 {
			h.db.Where("user_id = ? AND role_id = ?", u.ID, admin.ID).
				FirstOrCreate(&model.UserRole{}, model.UserRole{UserID: u.ID, RoleID: admin.ID})
		}
	}
	OK(c, t)
}

func (h *Handlers) UpdateTenant(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var t model.Tenant
	if err := h.db.First(&t, id).Error; err != nil {
		NotFound(c, "租户不存在")
		return
	}
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		BadRequest(c, err)
		return
	}
	delete(body, "id")
	delete(body, "key")
	if err := h.db.Model(&t).Updates(body).Error; err != nil {
		BadRequest(c, err)
		return
	}
	OK(c, t)
}

func (h *Handlers) DeleteTenant(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	if idStr := strconv.FormatUint(uint64(id), 10); idStr == "" {
		BadRequest(c, "id 非法")
		return
	}
	var cnt int64
	h.db.Model(&model.Project{}).Where("tenant_id = ?", id).Count(&cnt)
	if cnt > 0 {
		BadRequest(c, "租户下仍有项目，请先清理")
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		tx.Where("tenant_id = ?", id).Delete(&model.User{})
		tx.Where("tenant_id = ?", id).Delete(&model.Role{})
		return tx.Delete(&model.Tenant{}, id).Error
	}); err != nil {
		ServerError(c, err)
		return
	}
	OK(c, gin.H{"deleted": id})
}

var _ = errors.New
