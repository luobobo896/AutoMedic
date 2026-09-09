package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// JSON 通用 JSON 字段类型（PostgreSQL text / jsonb 均可 Scan）
type JSON json.RawMessage

func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return "{}", nil
	}
	return string(j), nil
}

func (j *JSON) Scan(v any) error {
	if v == nil {
		*j = JSON("{}")
		return nil
	}
	switch b := v.(type) {
	case []byte:
		*j = JSON(append([]byte(nil), b...))
	case string:
		*j = JSON(b)
	default:
		return errors.New("unsupported JSON source")
	}
	return nil
}

func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

func (j *JSON) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*j = JSON("{}")
		return nil
	}
	*j = JSON(append([]byte(nil), b...))
	return nil
}

func (j JSON) Unmarshal(dst any) error {
	if len(j) == 0 {
		return nil
	}
	return json.Unmarshal(j, dst)
}

func MustJSON(v any) JSON {
	b, _ := json.Marshal(v)
	if len(b) == 0 {
		b = []byte("{}")
	}
	return JSON(b)
}

// ---------- 枚举 ----------

type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"    // 待处理
	TaskStatusRunning    TaskStatus = "running"    // 处理中
	TaskStatusConfirming TaskStatus = "confirming" // 待人工确认
	TaskStatusSuccess    TaskStatus = "success"    // 成功
	TaskStatusFailed     TaskStatus = "failed"     // 失败
	TaskStatusIgnored    TaskStatus = "ignored"    // 已忽略
	TaskStatusCancelled  TaskStatus = "cancelled"  // 已取消
	TaskStatusRejected   TaskStatus = "rejected"   // 人工驳回
)

type FixMode string

const (
	FixModeAuto FixMode = "auto" // 全自动
	FixModeSemi FixMode = "semi" // 半自动确认
)

type CredType string

const (
	CredTypeSSHKey    CredType = "ssh_key"
	CredTypeHTTPAuth  CredType = "http_auth"
	CredTypeHTTPToken CredType = "http_token"
)

type EventStatus string

const (
	EventStatusReceived EventStatus = "received"
	EventStatusMatched  EventStatus = "matched"
	EventStatusFixing   EventStatus = "fixing"
	EventStatusFixed    EventStatus = "fixed"
	EventStatusFailed   EventStatus = "failed"
	EventStatusIgnored  EventStatus = "ignored"
	EventStatusDropped  EventStatus = "dropped"
)

type ReviewStatus string

const (
	ReviewStatusPending ReviewStatus = "pending"
	ReviewStatusRunning ReviewStatus = "running"
	ReviewStatusSuccess ReviewStatus = "success"
	ReviewStatusFailed  ReviewStatus = "failed"
)

// ---------- 基础实体 ----------

type BaseModel struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Project 项目：组织单元，可关联多个仓库
type Project struct {
	TenantID uint `gorm:"index;not null;default:0" json:"tenant_id"`
	BaseModel
	Name        string  `gorm:"size:128;not null" json:"name"`
	Key         string  `gorm:"size:64;uniqueIndex;not null" json:"key"`
	Description string  `gorm:"size:512" json:"description"`
	FixMode     FixMode `gorm:"size:16;default:'semi'" json:"fix_mode"` // 项目级默认模式
	// 默认使用的模型（为空则用全局默认模型）
	DefaultModelID *uint `json:"default_model_id"`
	// 审查用模型（为空则与修复模型同一套）
	DefaultReviewModelID *uint `json:"default_review_model_id"`
	// 默认发布钩子（覆盖全局）
	ReleaseHook string `gorm:"size:1024" json:"release_hook"`
	// 是否启用事件自动触发
	Enabled bool `gorm:"default:true" json:"enabled"`
	// 项目备注/上下文：写入 dsh 指令文件，帮助定位业务语义
	Context string `gorm:"type:text" json:"context"`

	// 非持久化：关联数量（列表接口填充，用于展示接入进度）
	RepoCount  int64 `gorm:"-" json:"repo_count"`
	RuleCount  int64 `gorm:"-" json:"rule_count"`
	TokenCount int64 `gorm:"-" json:"token_count"`
}

// Repository 仓库
type Repository struct {
	TenantID uint `gorm:"index;not null;default:0" json:"tenant_id"`
	BaseModel
	ProjectID uint   `gorm:"index;not null" json:"project_id"`
	Name      string `gorm:"size:128;not null" json:"name"`
	URL       string `gorm:"size:512;not null" json:"url"`
	Branch    string `gorm:"size:128;default:'main'" json:"branch"`
	// 该仓库内与代码定位相关的路径前缀（逗号分隔），用于生成 dsh 上下文
	CodePaths string `gorm:"size:1024" json:"code_paths"`
	// 该仓库负责的语言/技术栈，辅助 dsh 判断
	Language     string `gorm:"size:64" json:"language"`
	CredentialID *uint  `json:"credential_id"`
	// 该仓库默认使用的模型
	ModelID *uint `json:"model_id"`
	// 该仓库审查用模型（为空则：仓库修复模型 → 项目审查模型 → 项目/全局修复模型）
	ReviewModelID *uint `json:"review_model_id"`
	// 是否允许自动推送
	AutoPush   bool   `gorm:"default:true" json:"auto_push"`
	Enabled    bool   `gorm:"default:true" json:"enabled"`
	LastCommit string `gorm:"size:64" json:"last_commit"`

	Project     *Project    `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	Credential  *Credential `gorm:"foreignKey:CredentialID" json:"credential,omitempty"`
	Model       *LLMModel   `gorm:"foreignKey:ModelID" json:"model,omitempty"`
	ReviewModel *LLMModel   `gorm:"foreignKey:ReviewModelID" json:"review_model,omitempty"`
}

// Credential git 凭证，可被多个项目/仓库复用
type Credential struct {
	TenantID uint `gorm:"index;not null;default:0" json:"tenant_id"`
	BaseModel
	Name string   `gorm:"size:128;not null" json:"name"`
	Type CredType `gorm:"size:32;not null" json:"type"`
	// http_auth 专用
	Username string `gorm:"size:128" json:"username"`
	// 加密存储的密文（密码 / token / 私钥 / 私钥口令）
	SecretEnc     string `gorm:"type:text" json:"-"`
	PassphraseEnc string `gorm:"type:text" json:"-"`
	Description   string `gorm:"size:512" json:"description"`
	Enabled       bool   `gorm:"default:true" json:"enabled"`
	// 是否脱敏展示（前端只返回 masked）
}

type CredentialUsage struct {
	TenantID     uint      `gorm:"index;not null;default:0" json:"tenant_id"`
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CredentialID uint      `gorm:"index;not null" json:"credential_id"`
	RefType      string    `gorm:"size:32" json:"ref_type"` // repo | task
	RefID        uint      `json:"ref_id"`
	Action       string    `gorm:"size:32" json:"action"` // clone | fetch | push
	Result       string    `gorm:"size:16" json:"result"` // ok | fail
	Message      string    `gorm:"size:512" json:"message"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`

	Credential *Credential `gorm:"foreignKey:CredentialID" json:"credential,omitempty"`
}

// Provider 大模型厂家
type Provider struct {
	BaseModel
	Name    string `gorm:"size:128;not null" json:"name"`
	Key     string `gorm:"size:64;uniqueIndex;not null" json:"key"` // dsh provider 标识，如 deepseek-official
	Kind    string `gorm:"size:64" json:"kind"`                     // 便于前端分组展示
	BaseURL string `gorm:"size:512" json:"base_url"`
	// 加密存储的 API Key
	APIKeyEnc string `gorm:"type:text" json:"-"`
	Enabled   bool   `gorm:"default:true" json:"enabled"`
	Remark    string `gorm:"size:512" json:"remark"`
}

// LLMModel 模型：每个模型独立配置输入/输出上下文大小
type LLMModel struct {
	BaseModel
	ProviderID    uint   `gorm:"index;not null" json:"provider_id"`
	Name          string `gorm:"size:128;not null" json:"name"` // 展示名
	Slug          string `gorm:"size:128;not null" json:"slug"` // 传给 dsh 的模型标识
	InputContext  int64  `json:"input_context"`                 // 输入上下文大小，如 1048576
	OutputContext int64  `json:"output_context"`                // 输出上下文大小，如 131072
	MaxTurns      int    `json:"max_turns"`                     // 最大推理轮次
	Temperature   string `gorm:"size:16" json:"temperature"`    // 字符串避免浮点精度噪声
	ExtraParams   JSON   `gorm:"type:text" json:"extra_params"` // 透传到 dsh patch 的额外参数（JSON）
	Enabled       bool   `gorm:"default:true" json:"enabled"`
	IsDefault     bool   `json:"is_default"`
	Remark        string `gorm:"size:512" json:"remark"`

	Provider *Provider `gorm:"foreignKey:ProviderID" json:"provider,omitempty"`
}

// IngestToken 外部日志采集器投递令牌
type IngestToken struct {
	TenantID uint `gorm:"index;not null;default:0" json:"tenant_id"`
	BaseModel
	ProjectID  uint       `gorm:"index;not null" json:"project_id"`
	Name       string     `gorm:"size:128;not null" json:"name"`
	Prefix     string     `gorm:"size:16;index" json:"prefix"`   // 明文前缀，便于识别
	TokenHash  string     `gorm:"size:128;uniqueIndex" json:"-"` // sha256
	Enabled    bool       `gorm:"default:true" json:"enabled"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	// 允许的来源 IP 段（逗号分隔，为空不限制）
	AllowCIDR string `gorm:"size:512" json:"allow_cidr"`

	Project *Project `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
}

// Event 生产事件
type Event struct {
	TenantID uint `gorm:"index;not null;default:0" json:"tenant_id"`
	BaseModel
	ProjectID uint   `gorm:"index;not null" json:"project_id"`
	TokenID   *uint  `json:"token_id"`
	Source    string `gorm:"size:64;index" json:"source"` // sentry | loki | k8s | custom
	Level     string `gorm:"size:32;index" json:"level"`  // fatal | error | warn | info
	Title     string `gorm:"size:512" json:"title"`
	Message   string `gorm:"type:text" json:"message"`
	Stack     string `gorm:"type:text" json:"stack"`
	// 指纹：用于去重与告警聚合
	Fingerprint string      `gorm:"size:128;index" json:"fingerprint"`
	Payload     JSON        `gorm:"type:text" json:"payload"`
	Status      EventStatus `gorm:"size:16;default:'received';index" json:"status"`
	// 命中的规则
	RuleID *uint `json:"rule_id"`
	// 处置说明
	DisposeMsg string     `gorm:"size:512" json:"dispose_msg"`
	OccurredAt time.Time  `gorm:"index" json:"occurred_at"`
	LastSeenAt *time.Time `json:"last_seen_at"`

	// OccurrenceN 同指纹出现次数（合并重复后落库）
	OccurrenceN int `gorm:"not null;default:1" json:"occurrence_n"`

	Project *Project `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	Rule    *Rule    `gorm:"foreignKey:RuleID" json:"rule,omitempty"`
}

// Rule 规则：决定告警是否进入修复流程
type Rule struct {
	TenantID uint `gorm:"index;not null;default:0" json:"tenant_id"`
	BaseModel
	ProjectID uint   `gorm:"index;not null" json:"project_id"`
	Name      string `gorm:"size:128;not null" json:"name"`
	Enabled   bool   `gorm:"default:true" json:"enabled"`
	Priority  int    `gorm:"default:0" json:"priority"` // 越大越优先
	// 条件
	Levels      string `gorm:"size:256" json:"levels"`        // fatal,error（逗号分隔，为空不限）
	Sources     string `gorm:"size:256" json:"sources"`       // 来源白名单
	Keywords    string `gorm:"size:1024" json:"keywords"`     // 命中关键字（任一）
	AllKeywords string `gorm:"size:1024" json:"all_keywords"` // 必须全部命中
	// 排除（业务拒绝、第三方故障等非代码问题）
	ExcludeKeywords string `gorm:"size:1024" json:"exclude_keywords"`
	ExcludeSources  string `gorm:"size:256" json:"exclude_sources"`
	// 正则（可选，命中才触发）
	Pattern string `gorm:"size:512" json:"pattern"`
	// 时间窗口内出现次数阈值（抑制偶发抖动）
	MinCount  int `json:"min_count"`
	WindowSec int `json:"window_sec"`
	// 冷却：同一指纹在 N 秒内不重复触发修复
	CooldownSec int `json:"cooldown_sec"`
	// 动作：fix | ignore | confirm（忽略 / 仅人工确认后修复）
	Action     string `gorm:"size:16;default:'fix'" json:"action"`
	RepoIDs    string `gorm:"size:256" json:"repo_ids"` // 限定仓库，为空则项目下所有启用仓库
	ModelID    *uint  `json:"model_id"`
	FixMode    string `gorm:"size:16" json:"fix_mode"` // 为空继承项目
	MaxRetries int    `json:"max_retries"`
	// 自定义 prompt 模板
	PromptTemplate string `gorm:"type:text" json:"prompt_template"`
	Description    string `gorm:"size:512" json:"description"`

	Project *Project `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
}

// Task 修复任务
type Task struct {
	TenantID uint `gorm:"index;not null;default:0" json:"tenant_id"`
	BaseModel
	EventID   *uint `gorm:"index" json:"event_id"`
	ProjectID uint  `gorm:"index;not null" json:"project_id"`
	RepoID    uint  `gorm:"index;not null" json:"repo_id"`
	RuleID    *uint `gorm:"index" json:"rule_id"`
	ModelID   *uint `json:"model_id"`

	Status TaskStatus `gorm:"size:16;default:'pending';index" json:"status"`
	Mode   FixMode    `gorm:"size:16;default:'semi'" json:"mode"`
	Retry  int        `json:"retry"`

	Branch     string `gorm:"size:256" json:"branch"`
	BaseCommit string `gorm:"size:64" json:"base_commit"`
	FixCommit  string `gorm:"size:64" json:"fix_commit"`
	Workspace  string `gorm:"size:512" json:"workspace"`
	PRURL      string `gorm:"size:512" json:"pr_url"`

	// 结果
	Summary      string `gorm:"type:text" json:"summary"`
	Diagnosis    string `gorm:"type:text" json:"diagnosis"` // 根因分析
	ChangedFiles JSON   `gorm:"type:text" json:"changed_files"`
	DiffStat     string `gorm:"size:512" json:"diff_stat"`
	Patch        string `gorm:"type:text" json:"patch"` // 补丁全文
	ErrorMsg     string `gorm:"type:text" json:"error_msg"`

	// dsh 执行信息
	DSHModel      string `gorm:"size:128" json:"dsh_model"`
	DSHProvider   string `gorm:"size:128" json:"dsh_provider"`
	DSHExitCode   int    `json:"dsh_exit_code"`
	DSHCmd        string `gorm:"type:text" json:"dsh_cmd"`
	InputContext  int64  `json:"input_context"`
	OutputContext int64  `json:"output_context"`

	// 人工确认
	ConfirmedBy string     `gorm:"size:64" json:"confirmed_by"`
	ConfirmedAt *time.Time `json:"confirmed_at"`
	ConfirmNote string     `gorm:"size:512" json:"confirm_note"`

	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	DurationMS int64      `json:"duration_ms"`
	// 阶段：prepare | dsh | verify | commit | push | release
	Stage string `gorm:"size:32" json:"stage"`

	Event   *Event      `gorm:"foreignKey:EventID" json:"event,omitempty"`
	Repo    *Repository `gorm:"foreignKey:RepoID" json:"repo,omitempty"`
	Rule    *Rule       `gorm:"foreignKey:RuleID" json:"rule,omitempty"`
	Model   *LLMModel   `gorm:"foreignKey:ModelID" json:"model,omitempty"`
	Project *Project    `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
}

// TaskLog 终端日志行
type TaskLog struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID    uint      `gorm:"index;not null" json:"task_id"`
	Seq       int64     `json:"seq"`
	Stream    string    `gorm:"size:8" json:"stream"` // stdout | stderr | sys
	Content   string    `gorm:"type:text" json:"content"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

func (TaskLog) TableName() string { return "task_logs" }

// Setting 键值配置
type Setting struct {
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ReviewJob 仓库上手动触发的 OCR 审查（不改代码）
type ReviewJob struct {
	TenantID uint `gorm:"index;not null;default:0" json:"tenant_id"`
	BaseModel
	ProjectID  uint         `gorm:"index;not null" json:"project_id"`
	RepoID     uint         `gorm:"index;not null" json:"repo_id"`
	Status     ReviewStatus `gorm:"size:16;default:'pending';index" json:"status"`
	Mode       string       `gorm:"size:16" json:"mode"` // review | scan
	FromRef    string       `gorm:"size:256" json:"from_ref"`
	ToRef      string       `gorm:"size:256" json:"to_ref"`
	Path       string       `gorm:"size:512" json:"path"`
	ScanAll    bool         `json:"scan_all"`
	Cmd        string       `gorm:"type:text" json:"cmd"`
	Progress   string       `gorm:"size:512" json:"progress"`
	Logs       string       `gorm:"type:text" json:"logs"`
	ErrorMsg   string       `gorm:"type:text" json:"error_msg"`
	Findings   JSON         `gorm:"type:text" json:"-"`
	FindingN   int          `json:"finding_n"`
	DurationMS int64        `json:"duration_ms"`
	StartedAt  *time.Time   `json:"started_at"`
	FinishedAt *time.Time   `json:"finished_at"`

	Repo    *Repository `gorm:"foreignKey:RepoID" json:"repo,omitempty"`
	Project *Project    `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
}

func All() []any {
	return []any{
		&Tenant{}, &User{}, &Role{}, &RolePermission{}, &UserRole{}, &AuthToken{},
		&Project{}, &Repository{}, &Credential{}, &CredentialUsage{},
		&Provider{}, &LLMModel{}, &IngestToken{}, &Event{}, &Rule{},
		&Task{}, &TaskLog{}, &Setting{}, &ReviewJob{}, &DictItem{},
	}
}

// TaskStatusList 状态字典（前端筛选用）
var TaskStatusList = []TaskStatus{
	TaskStatusPending, TaskStatusRunning, TaskStatusConfirming,
	TaskStatusSuccess, TaskStatusFailed, TaskStatusIgnored,
	TaskStatusCancelled, TaskStatusRejected,
}
