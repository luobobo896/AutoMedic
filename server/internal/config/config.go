package config

import (
	"crypto/rand"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	DB       DBConfig       `yaml:"db"`
	Auth     AuthConfig     `yaml:"auth"`
	Security SecurityConfig `yaml:"security"`
	DSH      DSHConfig      `yaml:"dsh"`
	OCR      OCRConfig      `yaml:"ocr"`
	Git      GitConfig      `yaml:"git"`
	Log      LogConfig      `yaml:"log"`
}

type ServerConfig struct {
	Addr         string `yaml:"addr"`          // :8080
	Mode         string `yaml:"mode"`          // debug | release
	WebDir       string `yaml:"web_dir"`       // 前端静态资源目录（留空则不托管）
	AdminToken   string `yaml:"admin_token"`   // 管理 API 鉴权令牌（X-Admin-Token）
	ReadTimeout  int    `yaml:"read_timeout"`  // 秒
	WriteTimeout int    `yaml:"write_timeout"` // 秒
	// 允许的前端源（CORS）。空则不挂 CORS 中间件：同源部署可用；
	// 前后端分离必须显式填写，否则浏览器会拦截跨域响应。
	AllowOrigins []string `yaml:"allow_origins"`
	// 可信反向代理 IP/CIDR；为空表示不信任任何代理，ClientIP 取 TCP 对端
	TrustedProxies []string `yaml:"trusted_proxies"`
}

type DBConfig struct {
	Driver   string `yaml:"driver"` // 仅 postgres
	DSN      string `yaml:"dsn"`    // host=127.0.0.1 user=automedic password=xxx dbname=automedic port=5432 sslmode=disable TimeZone=Asia/Shanghai
	MaxOpen  int    `yaml:"max_open"`
	MaxIdle  int    `yaml:"max_idle"`
	LogLevel string `yaml:"log_level"` // silent | error | warn | info
}

// AuthConfig 账号密码登录与 RBAC
type AuthConfig struct {
	// JWT 签名密钥；为空则回退使用 security.secret_key
	JWTSecret string `yaml:"jwt_secret"`
	// 访问令牌有效期（分钟），默认 480
	AccessTokenTTL int `yaml:"access_token_ttl"`
	// 刷新令牌有效期（分钟），默认 1440
	RefreshTokenTTL int `yaml:"refresh_token_ttl"`
	// 首次启动时创建的超级管理员账号
	BootstrapAdmin BootstrapAdmin `yaml:"bootstrap_admin"`
}

type BootstrapAdmin struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	// 默认租户名称（超管归属）
	TenantName string `yaml:"tenant_name"`
	TenantKey  string `yaml:"tenant_key"`
}

type SecurityConfig struct {
	SecretKey     string `yaml:"secret_key"`      // 敏感字段加密主密钥（AES-256-GCM，base64 32 字节）
	SecretKeyFile string `yaml:"secret_key_file"` // 或从文件读取，优先级低于 SecretKey
}

type DSHConfig struct {
	// dsh 可执行文件路径；为空则从 PATH 查找
	Bin string `yaml:"bin"`
	// dsh home；为空则继承进程环境
	Home string `yaml:"home"`
	// dsh 启动器参数模板，占位符：{{.Bin}} {{.Profile}} {{.Patches}} {{.TaskFile}}
	// 默认：{{.Bin}} --profile headless {{.Patches}} "$(cat {{.TaskFile}})"
	// 注意：headless profile 仅接受一个 task 文本参数，因此模型/上下文通过 --patch 注入。
	CommandTemplate string `yaml:"command_template"`
	// 是否使用 shell 执行模板（模板内含管道/引号时需要为 true）
	UseShell bool `yaml:"use_shell"`
	// 单次修复超时（秒）
	TimeoutSec int `yaml:"timeout_sec"`
	// 权限模式：workspace-write（仅工作区可写）| danger-full-access
	PermissionMode string `yaml:"permission_mode"`
	// 传给 dsh 的额外环境变量（KEY=VALUE）
	Env []string `yaml:"env"`
	// patch 覆盖层模板（cordis patch yaml），占位符：{{.Provider}} {{.Model}} {{.InputContext}} {{.OutputContext}} {{.ExtraYAML}}
	PatchTemplate string `yaml:"patch_template"`
	// 额外工作区指令文件（写入工作区供 dsh 读取）
	InstructionFile string `yaml:"instruction_file"`
	// headless 结束后解析结果的方式：text | json
	OutputFormat string `yaml:"output_format"`
	// 追加到 dsh 任务的兜底约束提示
	SystemGuard string `yaml:"system_guard"`
}

type OCRConfig struct {
	// ocr 可执行文件路径；为空则从 PATH 查找
	Bin string `yaml:"bin"`
	// 单次审查超时（秒），默认 600
	TimeoutSec int `yaml:"timeout_sec"`
	// 平台审查模型（大模型配置中心的模型 id）。为空则走仓库/项目审查模型，再回退修复/全局默认。
	ModelID *uint `yaml:"model_id"`
	// 传给 ocr 进程的额外环境变量（仅运维 PATH 等，不在 Web 暴露）
	Env []string `yaml:"env"`
}

type GitConfig struct {
	// 隔离工作区根目录
	WorkspaceRoot string `yaml:"workspace_root"`
	// clone 深度，0 表示全量
	Depth int `yaml:"depth"`
	// 是否复用已有工作区（增量 fetch），false 则每次全新 clone
	ReuseWorkspace bool `yaml:"reuse_workspace"`
	// 修复分支前缀，如 automedic/fix-
	BranchPrefix string `yaml:"branch_prefix"`
	// commit 作者
	AuthorName  string `yaml:"author_name"`
	AuthorEmail string `yaml:"author_email"`
	// 是否自动推送
	AutoPush bool `yaml:"auto_push"`
	// 推送后触发的发布钩子命令（在仓库目录执行，可为空）
	ReleaseHook string `yaml:"release_hook"`
	// git 可执行文件路径
	Bin string `yaml:"bin"`
	// 工作区保留天数（清理用）
	KeepDays int `yaml:"keep_days"`
}

type LogConfig struct {
	Level  string `yaml:"level"`  // debug | info | warn | error
	Dir    string `yaml:"dir"`    // 日志目录，留空仅输出到 stdout
	Retain int    `yaml:"retain"` // 保留天数
	Format string `yaml:"format"` // text | json
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Addr:         ":8080",
			Mode:         "release",
			AdminToken:   "change-me",
			ReadTimeout:  60,
			WriteTimeout: 120,
		},
		DB: DBConfig{
			Driver:   "postgres",
			DSN:      "host=127.0.0.1 user=automedic password=automedic dbname=automedic port=5432 sslmode=disable TimeZone=Asia/Shanghai",
			MaxOpen:  50,
			MaxIdle:  10,
			LogLevel: "warn",
		},
		Auth: AuthConfig{
			AccessTokenTTL:  480,  // 8 小时
			RefreshTokenTTL: 1440, // 24 小时
			BootstrapAdmin: BootstrapAdmin{
				Username:   "admin",
				Password:   "admin123",
				TenantName: "默认租户",
				TenantKey:  "default",
			},
		},
		Security: SecurityConfig{},
		DSH: DSHConfig{
			Bin:             "dsh",
			CommandTemplate: `{{.Bin}} --profile headless {{.Patches}} "$(cat {{.TaskFile}})"`,
			UseShell:        true,
			TimeoutSec:      1800,
			PermissionMode:  "workspace-write",
			PatchTemplate: `- id: agent-default-model
  config:
    provider: {{.Provider}}
    model: {{.Model}}
    inputContextTokens: {{.InputContext}}
    outputContextTokens: {{.OutputContext}}
{{.ExtraYAML}}`,
			InstructionFile: "AUTOMEDIC.md",
			OutputFormat:    "text",
			SystemGuard:     "",
		},
		OCR: OCRConfig{
			Bin:        "ocr",
			TimeoutSec: 600,
		},
		Git: GitConfig{
			WorkspaceRoot:  "data/workspaces",
			Depth:          1,
			ReuseWorkspace: true,
			BranchPrefix:   "automedic/fix-",
			AuthorName:     "AutoMedic",
			AuthorEmail:    "automedic@local",
			AutoPush:       true,
			KeepDays:       7,
			Bin:            "git",
		},
		Log: LogConfig{Level: "info", Format: "text", Retain: 14},
	}
}

// Load 读取配置文件，环境变量可覆盖：AUTOMEDIC_ 前缀 + 下划线路径（如 AUTOMEDIC_DB_DSN）
func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if err == nil {
			if err := yaml.Unmarshal(b, cfg); err != nil {
				return nil, err
			}
		}
	}
	applyEnv(cfg)
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}
	if cfg.DSH.Bin == "" {
		cfg.DSH.Bin = "dsh"
	}
	if cfg.OCR.Bin == "" {
		cfg.OCR.Bin = "ocr"
	}
	if cfg.OCR.TimeoutSec <= 0 {
		cfg.OCR.TimeoutSec = 600
	}
	if cfg.Git.Bin == "" {
		cfg.Git.Bin = "git"
	}
	if cfg.Git.WorkspaceRoot == "" {
		cfg.Git.WorkspaceRoot = "data/workspaces"
	}
	return cfg, nil
}

func applyEnv(cfg *Config) {
	set := func(dst *string, key string) {
		if v := os.Getenv("AUTOMEDIC_" + key); v != "" {
			*dst = v
		}
	}
	// 逗号分隔的切片字段
	setSlice := func(dst *[]string, key string) {
		if v := os.Getenv("AUTOMEDIC_" + key); v != "" {
			*dst = strings.Split(v, ",")
		}
	}
	set(&cfg.Server.Addr, "SERVER_ADDR")
	set(&cfg.Server.Mode, "SERVER_MODE")
	set(&cfg.Server.WebDir, "SERVER_WEB_DIR")
	set(&cfg.Server.AdminToken, "SERVER_ADMIN_TOKEN")
	setSlice(&cfg.Server.AllowOrigins, "SERVER_ALLOW_ORIGINS")
	setSlice(&cfg.Server.TrustedProxies, "SERVER_TRUSTED_PROXIES")
	set(&cfg.DB.Driver, "DB_DRIVER")
	set(&cfg.DB.DSN, "DB_DSN")
	set(&cfg.Auth.JWTSecret, "AUTH_JWT_SECRET")
	set(&cfg.Auth.BootstrapAdmin.Username, "AUTH_BOOTSTRAP_USERNAME")
	set(&cfg.Auth.BootstrapAdmin.Password, "AUTH_BOOTSTRAP_PASSWORD")
	set(&cfg.Security.SecretKey, "SECURITY_SECRET_KEY")
	set(&cfg.Security.SecretKeyFile, "SECURITY_SECRET_KEY_FILE")
	set(&cfg.DSH.Bin, "DSH_BIN")
	set(&cfg.DSH.Home, "DSH_HOME")
	set(&cfg.DSH.PermissionMode, "DSH_PERMISSION_MODE")
	set(&cfg.OCR.Bin, "OCR_BIN")
	set(&cfg.Git.WorkspaceRoot, "GIT_WORKSPACE_ROOT")
	set(&cfg.Git.Bin, "GIT_BIN")
	set(&cfg.Log.Level, "LOG_LEVEL")
}

// ResolveSecretKey 返回用于敏感字段加密的主密钥（32 字节）
func (c *Config) ResolveSecretKey() ([]byte, error) {
	if c.Security.SecretKey != "" {
		return []byte(padOrTrim(c.Security.SecretKey)), nil
	}
	if c.Security.SecretKeyFile != "" {
		b, err := os.ReadFile(c.Security.SecretKeyFile)
		if err != nil {
			return nil, err
		}
		return []byte(padOrTrim(strings.TrimSpace(string(b)))), nil
	}
	return nil, errNoKey
}

// ResolveJWTSecret JWT 签名密钥，按以下优先级解析：
//  1. auth.jwt_secret（padOrTrim 到 32 字节）
//  2. security.secret_key + "-jwt"
//  3. security.secret_key_file：读取文件内容（TrimSpace）后 padOrTrim 再拼 "-jwt"
//     （读文件失败记录错误并回退到随机临时密钥，不静默使用弱默认值）
//  4. 以上皆无：用 crypto/rand 生成 32 字节随机密钥（重启后登录态失效，仅兜底）
//
// 注意：padOrTrim 用 '0' 填充不足 32 字节的弱密钥仅保证长度，并不增强强度；
// 因此不要改动加密算法本身（改了会导致已按 32 字节约定存储/签发的令牌/密文解不开）。
func (c *Config) ResolveJWTSecret() []byte {
	if c.Auth.JWTSecret != "" {
		return []byte(padOrTrim(c.Auth.JWTSecret))
	}
	if c.Security.SecretKey != "" {
		return []byte(padOrTrim(c.Security.SecretKey + "-jwt"))
	}
	if c.Security.SecretKeyFile != "" {
		b, err := os.ReadFile(c.Security.SecretKeyFile)
		if err != nil {
			slog.Error("读取 security.secret_key_file 失败，使用随机临时密钥", "err", err)
		} else {
			return []byte(padOrTrim(strings.TrimSpace(string(b))) + "-jwt")
		}
	}
	slog.Warn("JWT 密钥未配置，已使用随机临时密钥，重启后登录态失效，请配置 auth.jwt_secret")
	return randomJWTSecret()
}

// randomJWTSecret 生成 32 字节密码学随机密钥（兜底用）
func randomJWTSecret() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// 绝不能回退到任何硬编码常量：那等于给所有人一把能自签超管令牌的钥匙。
		panic("无法生成随机 JWT 密钥，请显式配置 auth.jwt_secret: " + err.Error())
	}
	return b
}

var errNoKey = &ConfigError{Msg: "security.secret_key 未配置（或设置 AUTOMEDIC_SECURITY_SECRET_KEY）"}

type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }

func padOrTrim(s string) string {
	if len(s) >= 32 {
		return s[:32]
	}
	out := make([]byte, 32)
	copy(out, s)
	for i := len(s); i < 32; i++ {
		out[i] = '0'
	}
	return string(out)
}

func ensureDir(p string) error {
	if p == "" {
		return nil
	}
	return os.MkdirAll(filepath.Dir(p), 0o755)
}
