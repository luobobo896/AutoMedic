package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	DB       DBConfig       `yaml:"db"`
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
}

type DBConfig struct {
	Driver   string `yaml:"driver"` // mysql | sqlite
	DSN      string `yaml:"dsn"`    // mysql: user:pass@tcp(127.0.0.1:3306)/automedic?charset=utf8mb4&parseTime=True&loc=Local
	MaxOpen  int    `yaml:"max_open"`
	MaxIdle  int    `yaml:"max_idle"`
	LogLevel string `yaml:"log_level"` // silent | error | warn | info
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
	// 传给 ocr 进程的额外环境变量（KEY=VALUE），用于其独立 LLM 配置
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
			Driver:   "sqlite",
			DSN:      "data/automedic.db",
			MaxOpen:  50,
			MaxIdle:  10,
			LogLevel: "warn",
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
	set(&cfg.Server.Addr, "SERVER_ADDR")
	set(&cfg.Server.Mode, "SERVER_MODE")
	set(&cfg.Server.WebDir, "SERVER_WEB_DIR")
	set(&cfg.Server.AdminToken, "SERVER_ADMIN_TOKEN")
	set(&cfg.DB.Driver, "DB_DRIVER")
	set(&cfg.DB.DSN, "DB_DSN")
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
