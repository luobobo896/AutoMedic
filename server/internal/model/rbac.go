package model

import "time"

// ---------- 多租户 ----------

// Tenant 租户：业务数据的隔离边界（项目/仓库/凭证/规则/事件/任务）
type Tenant struct {
	BaseModel
	Name   string `gorm:"size:128;not null" json:"name"`
	Key    string `gorm:"size:64;uniqueIndex;not null" json:"key"`
	Status string `gorm:"size:16;default:'active'" json:"status"` // active | disabled
	Remark string `gorm:"size:512" json:"remark"`
}

// User 平台用户：属于某个租户（平台超管 TenantID 可为 0）
type User struct {
	BaseModel
	TenantID     uint   `gorm:"index;not null" json:"tenant_id"`
	Username     string `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash string `gorm:"size:128;not null" json:"-"`
	DisplayName  string `gorm:"size:128" json:"display_name"`
	Email        string `gorm:"size:128" json:"email"`
	Status       string `gorm:"size:16;default:'active'" json:"status"` // active | disabled
	// IsSuper 平台超级管理员：可跨租户管理，拥有全部权限
	IsSuper     bool       `gorm:"default:false" json:"is_super"`
	LastLoginAt *time.Time `json:"last_login_at"`

	Tenant *Tenant `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Roles  []Role  `gorm:"many2many:user_roles;" json:"roles,omitempty"`
}

// Role 角色：TenantID=0 表示平台级内置角色（如 super_admin）
type Role struct {
	BaseModel
	TenantID uint   `gorm:"index;not null;default:0" json:"tenant_id"`
	Code     string `gorm:"size:64;not null" json:"code"`
	Name     string `gorm:"size:128;not null" json:"name"`
	Builtin  bool   `gorm:"default:false" json:"builtin"` // 内置角色不可删除；权限可在 Web 调整
	Remark   string `gorm:"size:512" json:"remark"`

	// 非持久化：角色关联的权限码（读写接口填充）
	Permissions []string `gorm:"-" json:"permissions,omitempty"`
	// 非持久化：内置角色出厂权限，供「恢复默认」使用
	DefaultPermissions []string `gorm:"-" json:"default_permissions,omitempty"`
}

func (Role) TableName() string { return "roles" }

// RolePermission 角色-权限关联
type RolePermission struct {
	ID     uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	RoleID uint   `gorm:"uniqueIndex:idx_role_perm;not null" json:"role_id"`
	Code   string `gorm:"size:64;uniqueIndex:idx_role_perm;not null" json:"code"`
}

// UserRole 用户-角色关联
type UserRole struct {
	ID     uint `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID uint `gorm:"uniqueIndex:idx_user_role;not null" json:"user_id"`
	RoleID uint `gorm:"uniqueIndex:idx_user_role;not null" json:"role_id"`
}

// AuthToken 刷新令牌：用于续期与登出失效
type AuthToken struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint      `gorm:"index;not null" json:"user_id"`
	JTI         string    `gorm:"size:64;uniqueIndex" json:"jti"`
	RefreshHash string    `gorm:"size:128" json:"-"` // sha256(刷新令牌随机段)
	RefreshTTL  int64     `json:"refresh_ttl"`
	ExpiresAt   time.Time `gorm:"index" json:"expires_at"`
	Revoked     bool      `json:"revoked"`
	UserAgent   string    `gorm:"size:256" json:"user_agent"`
	IP          string    `gorm:"size:64" json:"ip"`
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
}

// ---------- 权限目录 ----------

// 权限码命名：资源:动作
const (
	PermOverviewRead = "overview:read"

	PermProjectRead   = "project:read"
	PermProjectCreate = "project:create"
	PermProjectUpdate = "project:update"
	PermProjectDelete = "project:delete"

	PermRepoRead   = "repo:read"
	PermRepoCreate = "repo:create"
	PermRepoUpdate = "repo:update"
	PermRepoDelete = "repo:delete"
	PermRepoReview = "repo:review"

	PermCredentialRead   = "credential:read"
	PermCredentialCreate = "credential:create"
	PermCredentialUpdate = "credential:update"
	PermCredentialDelete = "credential:delete"

	PermRuleRead   = "rule:read"
	PermRuleCreate = "rule:create"
	PermRuleUpdate = "rule:update"
	PermRuleDelete = "rule:delete"

	PermEventRead   = "event:read"
	PermEventReplay = "event:replay"

	PermTaskRead    = "task:read"
	PermTaskRetry   = "task:retry"
	PermTaskCancel  = "task:cancel"
	PermTaskIgnore  = "task:ignore"
	PermTaskConfirm = "task:confirm"

	PermStatsRead = "stats:read"

	PermTokenRead   = "token:read"
	PermTokenCreate = "token:create"
	PermTokenUpdate = "token:update"
	PermTokenDelete = "token:delete"

	PermSettingsRead   = "settings:read"
	PermSettingsUpdate = "settings:update"

	// 平台级：大模型厂家与模型配置（全局共享，不按租户隔离）
	PermModelRead   = "model:read"
	PermModelUpdate = "model:update"

	// 平台级：租户管理
	PermTenantRead   = "tenant:read"
	PermTenantCreate = "tenant:create"
	PermTenantUpdate = "tenant:update"
	PermTenantDelete = "tenant:delete"

	// 租户级：用户与角色
	PermUserRead   = "user:read"
	PermUserCreate = "user:create"
	PermUserUpdate = "user:update"
	PermUserDelete = "user:delete"

	PermRoleRead   = "role:read"
	PermRoleCreate = "role:create"
	PermRoleUpdate = "role:update"
	PermRoleDelete = "role:delete"
)

// Permission 权限项（接口返回用，不落库；权限集合由代码定义）
type Permission struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Group string `json:"group"`
}

// PermissionCatalog 全量权限目录
var PermissionCatalog = []Permission{
	{PermOverviewRead, "查看概览", "概览"},

	{PermProjectRead, "查看项目", "项目"},
	{PermProjectCreate, "创建项目", "项目"},
	{PermProjectUpdate, "修改项目", "项目"},
	{PermProjectDelete, "删除项目", "项目"},

	{PermRepoRead, "查看仓库", "仓库"},
	{PermRepoCreate, "创建仓库", "仓库"},
	{PermRepoUpdate, "修改仓库", "仓库"},
	{PermRepoDelete, "删除仓库", "仓库"},
	{PermRepoReview, "发起代码审查", "仓库"},

	{PermCredentialRead, "查看凭证", "凭证"},
	{PermCredentialCreate, "创建凭证", "凭证"},
	{PermCredentialUpdate, "修改凭证", "凭证"},
	{PermCredentialDelete, "删除凭证", "凭证"},

	{PermRuleRead, "查看规则", "规则"},
	{PermRuleCreate, "创建规则", "规则"},
	{PermRuleUpdate, "修改规则", "规则"},
	{PermRuleDelete, "删除规则", "规则"},

	{PermEventRead, "查看事件", "事件"},
	{PermEventReplay, "重放事件", "事件"},

	{PermTaskRead, "查看任务", "任务"},
	{PermTaskRetry, "重试任务", "任务"},
	{PermTaskCancel, "取消任务", "任务"},
	{PermTaskIgnore, "忽略任务", "任务"},
	{PermTaskConfirm, "确认/驳回任务", "任务"},

	{PermStatsRead, "查看统计", "统计"},

	{PermTokenRead, "查看采集令牌", "采集令牌"},
	{PermTokenCreate, "创建采集令牌", "采集令牌"},
	{PermTokenUpdate, "修改采集令牌", "采集令牌"},
	{PermTokenDelete, "删除采集令牌", "采集令牌"},

	{PermSettingsRead, "查看运行设置", "运行设置"},
	{PermSettingsUpdate, "修改运行设置", "运行设置"},

	{PermModelRead, "查看模型配置", "模型配置"},
	{PermModelUpdate, "维护模型配置", "模型配置"},

	{PermTenantRead, "查看租户", "租户"},
	{PermTenantCreate, "创建租户", "租户"},
	{PermTenantUpdate, "修改租户", "租户"},
	{PermTenantDelete, "删除租户", "租户"},

	{PermUserRead, "查看用户", "用户"},
	{PermUserCreate, "创建用户", "用户"},
	{PermUserUpdate, "修改用户", "用户"},
	{PermUserDelete, "删除用户", "用户"},

	{PermRoleRead, "查看角色", "角色"},
	{PermRoleCreate, "创建角色", "角色"},
	{PermRoleUpdate, "修改角色", "角色"},
	{PermRoleDelete, "删除角色", "角色"},
}

// AllPermissions 全量权限码
func AllPermissions() []string {
	out := make([]string, 0, len(PermissionCatalog))
	for _, p := range PermissionCatalog {
		out = append(out, p.Code)
	}
	return out
}

// 内置角色码
const (
	RoleSuperAdmin  = "super_admin"  // 平台超管：全部权限，跨租户
	RoleTenantAdmin = "tenant_admin" // 租户管理员：本租户全部业务权限 + 用户/角色管理
	RoleDeveloper   = "developer"    // 开发：日常使用 + 任务处置
	RoleViewer      = "viewer"       // 只读
)

// BuiltinRoles 内置角色与其默认权限
var BuiltinRoles = map[string][]string{
	RoleSuperAdmin: AllPermissions(),
	RoleTenantAdmin: append([]string{
		PermUserRead, PermUserCreate, PermUserUpdate, PermUserDelete,
		PermRoleRead, PermRoleCreate, PermRoleUpdate, PermRoleDelete,
	}, tenantBusinessPermissions()...),
	RoleDeveloper: []string{
		PermOverviewRead,
		PermProjectRead, PermProjectCreate, PermProjectUpdate,
		PermRepoRead, PermRepoCreate, PermRepoUpdate, PermRepoReview,
		PermCredentialRead,
		PermRuleRead, PermRuleCreate, PermRuleUpdate, PermRuleDelete,
		PermEventRead, PermEventReplay,
		PermTaskRead, PermTaskRetry, PermTaskCancel, PermTaskIgnore, PermTaskConfirm,
		PermStatsRead,
		PermTokenRead, PermTokenCreate, PermTokenUpdate,
		PermSettingsRead,
		PermModelRead,
	},
	RoleViewer: []string{
		PermOverviewRead,
		PermProjectRead, PermRepoRead, PermCredentialRead,
		PermRuleRead, PermEventRead, PermTaskRead,
		PermStatsRead, PermTokenRead, PermSettingsRead, PermModelRead,
		PermUserRead, PermRoleRead,
	},
}

// tenantBusinessPermissions 租户内业务权限（不含平台级租户管理）
func tenantBusinessPermissions() []string {
	skip := map[string]bool{
		PermTenantRead: true, PermTenantCreate: true, PermTenantUpdate: true, PermTenantDelete: true,
		PermModelUpdate: true,
	}
	out := make([]string, 0, len(PermissionCatalog))
	for _, p := range PermissionCatalog {
		if skip[p.Code] {
			continue
		}
		out = append(out, p.Code)
	}
	return out
}
