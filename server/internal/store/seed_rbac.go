package store

import (
	"errors"
	"log/slog"

	"github.com/automedic/automedic/internal/auth"
	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/model"
	"gorm.io/gorm"
)

// SeedRBAC 初始化租户、角色权限与引导管理员（幂等）
func SeedRBAC(db *gorm.DB, cfg *config.Config) error {
	// 1. 平台级内置角色：超级管理员
	if err := ensureRole(db, 0, model.RoleSuperAdmin, "平台超级管理员", true, model.BuiltinRoles[model.RoleSuperAdmin]); err != nil {
		return err
	}

	// 2. 默认租户
	tenantName, tenantKey := "默认租户", "default"
	if cfg.Auth.BootstrapAdmin.TenantName != "" {
		tenantName = cfg.Auth.BootstrapAdmin.TenantName
	}
	if cfg.Auth.BootstrapAdmin.TenantKey != "" {
		tenantKey = cfg.Auth.BootstrapAdmin.TenantKey
	}
	var tenant model.Tenant
	if err := db.Where("key = ?", tenantKey).FirstOrCreate(&tenant, model.Tenant{
		Name: tenantName, Key: tenantKey, Status: "active", Remark: "系统初始化创建",
	}).Error; err != nil {
		return err
	}

	// 3. 租户内置角色
	for _, rc := range []string{model.RoleTenantAdmin, model.RoleDeveloper, model.RoleViewer} {
		if err := ensureRole(db, tenant.ID, rc, roleName(rc), true, model.BuiltinRoles[rc]); err != nil {
			return err
		}
	}

	// 4. 引导管理员
	username := cfg.Auth.BootstrapAdmin.Username
	if username == "" {
		username = "admin"
	}
	pwd := cfg.Auth.BootstrapAdmin.Password
	if pwd == "" {
		pwd = "admin123"
	}
	var count int64
	db.Model(&model.User{}).Count(&count)
	if count == 0 {
		hash, err := auth.HashPassword(pwd)
		if err != nil {
			return err
		}
		u := model.User{
			TenantID: tenant.ID, Username: username, PasswordHash: hash,
			DisplayName: "超级管理员", Status: "active", IsSuper: true,
		}
		if err := db.Create(&u).Error; err != nil {
			return err
		}
		var super model.Role
		db.Where("code = ? AND tenant_id = ?", model.RoleSuperAdmin, 0).First(&super)
		if super.ID != 0 {
			db.Where("user_id = ? AND role_id = ?", u.ID, super.ID).
				FirstOrCreate(&model.UserRole{}, model.UserRole{UserID: u.ID, RoleID: super.ID})
		}
		slog.Info("已创建引导管理员", "username", username, "password_hint", "请登录后立即修改密码")
	}
	return nil
}

func roleName(code string) string {
	switch code {
	case model.RoleTenantAdmin:
		return "租户管理员"
	case model.RoleDeveloper:
		return "开发工程师"
	case model.RoleViewer:
		return "只读访客"
	}
	return code
}

// ensureRole 保证角色与其权限存在（幂等）
func ensureRole(db *gorm.DB, tenantID uint, code, name string, builtin bool, perms []string) error {
	var r model.Role
	err := db.Where("tenant_id = ? AND code = ?", tenantID, code).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		r = model.Role{TenantID: tenantID, Code: code, Name: name, Builtin: builtin}
		if err := db.Create(&r).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	// 内置角色权限以代码目录为准，每次启动补齐
	if builtin {
		for _, p := range perms {
			rp := model.RolePermission{RoleID: r.ID, Code: p}
			db.Where("role_id = ? AND code = ?", r.ID, p).FirstOrCreate(&rp)
		}
	}
	return nil
}

// CreateTenantRoles 新建租户时同步创建内置角色
func CreateTenantRoles(db *gorm.DB, tenantID uint) error {
	for _, rc := range []string{model.RoleTenantAdmin, model.RoleDeveloper, model.RoleViewer} {
		if err := ensureRole(db, tenantID, rc, roleName(rc), true, model.BuiltinRoles[rc]); err != nil {
			return err
		}
	}
	return nil
}

// SetRolePermissions 设置角色权限（自动补齐/回收）
func SetRolePermissions(db *gorm.DB, roleID uint, codes []string) error {
	want := map[string]bool{}
	for _, c := range codes {
		want[c] = true
	}
	var exist []model.RolePermission
	if err := db.Where("role_id = ?", roleID).Find(&exist).Error; err != nil {
		return err
	}
	for _, e := range exist {
		if !want[e.Code] {
			db.Delete(&e)
		}
	}
	for c := range want {
		rp := model.RolePermission{RoleID: roleID, Code: c}
		db.Where("role_id = ? AND code = ?", roleID, c).FirstOrCreate(&rp)
	}
	return nil
}
