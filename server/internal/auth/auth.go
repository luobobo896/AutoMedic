// Package auth 账号密码登录、JWT 签发校验与 RBAC 权限控制。
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ---------- 密码 ----------

func HashPassword(pwd string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, pwd string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pwd)) == nil
}

// ---------- 服务 ----------

type Service struct {
	db  *gorm.DB
	cfg *config.Config
	key []byte

	// refreshSeen 记录刷新令牌最近一次成功轮换的时间，用于并发重放宽限（见 refreshGrace）。
	// 仅本机内存态：多实例部署时各实例独立判定，最坏情况是宽限期失效回退到"轮换后旧令牌立即不可用"。
	refreshMu   sync.Mutex
	refreshSeen map[string]time.Time
}

// refreshGrace 刷新令牌轮换宽限期。
// 轮换（旧 refresh 换新的）后旧 refresh 立即吊销，但多标签页/页面刷新会在几乎同一时刻
// 用同一个旧 refresh 并发续期，严格一次性和多端可用不可兼得。
// 权衡：窗口内旧 refresh 若已泄露仍可续期，因此窗口取 30 秒而不是分钟级。
const refreshGrace = 30 * time.Second

func NewService(db *gorm.DB, cfg *config.Config) *Service {
	return &Service{db: db, cfg: cfg, key: cfg.ResolveJWTSecret(), refreshSeen: map[string]time.Time{}}
}

// markRefresh 记录 jti 最近一次轮换时间，并顺带清理窗口外的条目
func (s *Service) markRefresh(jti string, now time.Time) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	for k, t := range s.refreshSeen {
		if now.Sub(t) > refreshGrace {
			delete(s.refreshSeen, k)
		}
	}
	s.refreshSeen[jti] = now
}

// inRefreshGrace 判断该 jti 是否处于轮换宽限期内（并发重放）
func (s *Service) inRefreshGrace(jti string, now time.Time) bool {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	t, ok := s.refreshSeen[jti]
	return ok && now.Sub(t) <= refreshGrace
}

// Claims JWT 载荷
type Claims struct {
	UserID   uint   `json:"uid"`
	TenantID uint   `json:"tid"`
	Username string `json:"usr"`
	IsSuper  bool   `json:"sup"`
	jwt.RegisteredClaims
}

// Principal 当前登录主体
type Principal struct {
	User        model.User `json:"user"`
	TenantID    uint       `json:"tenant_id"`
	IsSuper     bool       `json:"is_super"`
	Roles       []string   `json:"roles"`
	Permissions []string   `json:"permissions"`
}

func (p *Principal) Can(code string) bool {
	if p.IsSuper {
		return true
	}
	for _, c := range p.Permissions {
		if c == code {
			return true
		}
	}
	return false
}

// Login 校验账号密码，返回主体与令牌
func (s *Service) Login(username, password, ip, ua string) (*Principal, string, string, error) {
	var u model.User
	if err := s.db.Preload("Roles").Where("username = ?", username).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", "", errors.New("账号或密码错误")
		}
		return nil, "", "", err
	}
	if u.Status != "active" {
		return nil, "", "", errors.New("账号已停用")
	}
	if !CheckPassword(u.PasswordHash, password) {
		return nil, "", "", errors.New("账号或密码错误")
	}
	now := time.Now()
	s.db.Model(&model.User{}).Where("id = ?", u.ID).Update("last_login_at", now)
	u.LastLoginAt = &now

	p, err := s.loadPrincipal(u)
	if err != nil {
		return nil, "", "", err
	}
	access, refresh, err := s.issueTokens(p, ip, ua)
	if err != nil {
		return nil, "", "", err
	}
	return p, access, refresh, nil
}

func (s *Service) loadPrincipal(u model.User) (*Principal, error) {
	if len(u.Roles) == 0 {
		s.db.Preload("Roles").First(&u, u.ID)
	}
	roleCodes := make([]string, 0, len(u.Roles))
	roleIDs := make([]uint, 0, len(u.Roles))
	isSuper := u.IsSuper
	for _, r := range u.Roles {
		roleCodes = append(roleCodes, r.Code)
		roleIDs = append(roleIDs, r.ID)
		if r.Code == model.RoleSuperAdmin {
			isSuper = true
		}
	}
	perms := []string{}
	if len(roleIDs) > 0 {
		var codes []string
		s.db.Model(&model.RolePermission{}).Where("role_id IN ?", roleIDs).Distinct().Pluck("code", &codes)
		perms = codes
	}
	return &Principal{User: u, TenantID: u.TenantID, IsSuper: isSuper, Roles: roleCodes, Permissions: perms}, nil
}

func (s *Service) accessTTL() time.Duration {
	min := s.cfg.Auth.AccessTokenTTL
	if min <= 0 {
		min = 480
	}
	return time.Duration(min) * time.Minute
}

func (s *Service) refreshTTL() time.Duration {
	min := s.cfg.Auth.RefreshTokenTTL
	if min <= 0 {
		min = 1440
	}
	return time.Duration(min) * time.Minute
}

func (s *Service) issueTokens(p *Principal, ip, ua string) (string, string, error) {
	jti, err := randomHex(16)
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	claims := Claims{
		UserID: p.User.ID, TenantID: p.TenantID, Username: p.User.Username, IsSuper: p.IsSuper,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   p.User.Username,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL())),
		},
	}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.key)
	if err != nil {
		return "", "", err
	}
	refresh, err := randomHex(32)
	if err != nil {
		return "", "", err
	}
	tok := model.AuthToken{
		UserID:      p.User.ID,
		JTI:         jti,
		RefreshHash: HashRefresh(refresh),
		ExpiresAt:   now.Add(s.refreshTTL()),
		IP:          ip,
		UserAgent:   truncate(ua, 250),
		CreatedAt:   now,
	}
	if err := s.db.Create(&tok).Error; err != nil {
		return "", "", err
	}
	// refresh = jti.secret，服务端只存 secret 的 sha256
	return access, jti + "." + refresh, nil
}

// Refresh 用刷新令牌换新的访问令牌
func (s *Service) Refresh(refresh string) (*Principal, string, string, error) {
	parts := strings.SplitN(refresh, ".", 2)
	if len(parts) != 2 {
		return nil, "", "", errors.New("刷新令牌无效")
	}
	now := time.Now()
	var tok model.AuthToken
	if err := s.db.Where("jti = ?", parts[0]).First(&tok).Error; err != nil {
		return nil, "", "", errors.New("刷新令牌无效或已失效")
	}
	if now.After(tok.ExpiresAt) {
		return nil, "", "", errors.New("刷新令牌已过期")
	}
	if tok.RefreshHash != "" && tok.RefreshHash != HashRefresh(parts[1]) {
		return nil, "", "", errors.New("刷新令牌无效或已失效")
	}
	// 已吊销：只有处于轮换宽限期内才视为并发重放（同一旧 refresh 被多处同时提交），
	// 否则按失效处理，防止已轮换掉的旧令牌被无限复用。
	if tok.Revoked && !s.inRefreshGrace(parts[0], now) {
		return nil, "", "", errors.New("刷新令牌无效或已失效")
	}
	var u model.User
	if err := s.db.Preload("Roles").First(&u, tok.UserID).Error; err != nil {
		return nil, "", "", errors.New("用户不存在")
	}
	if u.Status != "active" {
		return nil, "", "", errors.New("账号已停用")
	}
	p, err := s.loadPrincipal(u)
	if err != nil {
		return nil, "", "", err
	}
	if !tok.Revoked {
		s.db.Model(&model.AuthToken{}).Where("id = ?", tok.ID).Update("revoked", true)
		// 只有真正发生轮换时才记录宽限起点。重放请求不得续期窗口，
		// 否则持续出示同一个已吊销令牌就能把 30 秒窗口滚动延长，旧令牌可被无限复用。
		s.markRefresh(parts[0], now)
	}
	access, newRefresh, err := s.issueTokens(p, tok.IP, tok.UserAgent)
	if err != nil {
		return nil, "", "", err
	}
	return p, access, newRefresh, nil
}

// Logout 吊销刷新令牌
func (s *Service) Logout(refresh string) {
	parts := strings.SplitN(refresh, ".", 2)
	if len(parts) != 2 {
		return
	}
	s.db.Model(&model.AuthToken{}).Where("jti = ?", parts[0]).Update("revoked", true)
}

// ParseToken 解析访问令牌
func (s *Service) ParseToken(token string) (*Claims, error) {
	c := &Claims{}
	if _, err := jwt.ParseWithClaims(token, c, func(*jwt.Token) (any, error) { return s.key, nil }); err != nil {
		return nil, errors.New("登录已失效，请重新登录")
	}
	return c, nil
}

// PrincipalFromToken 由访问令牌还原主体（含最新权限）
func (s *Service) PrincipalFromToken(token string) (*Principal, error) {
	claims, err := s.ParseToken(token)
	if err != nil {
		return nil, err
	}
	var u model.User
	if err := s.db.Preload("Roles").First(&u, claims.UserID).Error; err != nil {
		return nil, errors.New("用户不存在")
	}
	if u.Status != "active" {
		return nil, errors.New("账号已停用")
	}
	return s.loadPrincipal(u)
}

// LoadByID 按 ID 加载主体（修改自己密码后刷新）
func (s *Service) LoadByID(id uint) (*Principal, error) {
	var u model.User
	if err := s.db.Preload("Roles").First(&u, id).Error; err != nil {
		return nil, errors.New("用户不存在")
	}
	return s.loadPrincipal(u)
}

// ---------- 中间件 ----------

type ctxKey struct{}

// Middleware 解析 Authorization: Bearer <token>，注入主体到 gin 与 request context
func (s *Service) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tok := bearerToken(c)
		if tok == "" {
			fail(c, http.StatusUnauthorized, "未登录")
			return
		}
		p, err := s.PrincipalFromToken(tok)
		if err != nil {
			fail(c, http.StatusUnauthorized, err.Error())
			return
		}
		c.Set("principal", p)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxKey{}, p))
		c.Next()
	}
}

// RequirePerm 权限校验
func RequirePerm(code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := Current(c)
		if p == nil {
			fail(c, http.StatusUnauthorized, "未登录")
			return
		}
		if !p.Can(code) {
			fail(c, http.StatusForbidden, "无权限："+code)
			return
		}
		c.Next()
	}
}

func fail(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"code": code, "message": msg})
	c.Abort()
}

func bearerToken(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	if sub := c.GetHeader("Sec-WebSocket-Protocol"); sub != "" {
		for _, part := range strings.Split(sub, ",") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "automedic.") {
				return strings.TrimPrefix(part, "automedic.")
			}
		}
	}
	return ""
}

// Current 取出当前主体
func Current(c *gin.Context) *Principal {
	if v, ok := c.Get("principal"); ok {
		if p, ok := v.(*Principal); ok {
			return p
		}
	}
	return nil
}

// TenantID 当前生效租户（超管可通过 X-Tenant-ID 切换）
func TenantID(c *gin.Context) uint {
	p := Current(c)
	if p == nil {
		return 0
	}
	if p.IsSuper {
		if v := c.GetHeader("X-Tenant-ID"); v != "" {
			var id uint
			for _, r := range v {
				if r < '0' || r > '9' {
					return p.TenantID
				}
				id = id*10 + uint(r-'0')
			}
			if id > 0 {
				return id
			}
		}
	}
	return p.TenantID
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashRefresh 刷新令牌摘要（预留：如需持久化可调用）
func HashRefresh(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
