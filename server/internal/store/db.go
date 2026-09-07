package store

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch strings.ToLower(cfg.DB.Driver) {
	case "mysql":
		dialector = mysql.Open(cfg.DB.DSN)
	case "sqlite", "sqlite3", "":
		if dir := filepath.Dir(cfg.DB.DSN); dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0o755)
		}
		dialector = sqlite.Open(cfg.DB.DSN)
	default:
		return nil, fmt.Errorf("unsupported db driver: %s", cfg.DB.Driver)
	}

	level := logger.Warn
	switch strings.ToLower(cfg.DB.LogLevel) {
	case "silent":
		level = logger.Silent
	case "error":
		level = logger.Error
	case "info":
		level = logger.Info
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger:          logger.Default.LogMode(level),
		TranslateError:  true,
		PrepareStmt:     false,
		CreateBatchSize: 200,
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if cfg.DB.MaxOpen > 0 {
		sqlDB.SetMaxOpenConns(cfg.DB.MaxOpen)
	}
	if cfg.DB.MaxIdle > 0 {
		sqlDB.SetMaxIdleConns(cfg.DB.MaxIdle)
	}
	if err := sqlDB.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

// AutoMigrate 建表并补齐索引；生产环境可改用 migrations/*.sql
func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(model.All()...); err != nil {
		return err
	}
	indexes := map[string][]string{
		"tasks":     {"idx_tasks_status_created", "idx_tasks_project_created", "idx_tasks_repo_status"},
		"task_logs": {"idx_task_logs_task_seq"},
		"events":    {"idx_events_project_occurred", "idx_events_fingerprint"},
	}
	_ = indexes
	// GORM 标签已覆盖主要索引，此处仅补充组合索引（SQLite/MySQL 均支持 IF NOT EXISTS 之外的幂等写法）
	extra := []string{
		"CREATE INDEX IF NOT EXISTS idx_tasks_status_created ON tasks(status, created_at)",
		"CREATE INDEX IF NOT EXISTS idx_tasks_project_created ON tasks(project_id, created_at)",
		"CREATE INDEX IF NOT EXISTS idx_task_logs_task_seq ON task_logs(task_id, seq)",
		"CREATE INDEX IF NOT EXISTS idx_events_project_occurred ON events(project_id, occurred_at)",
		"CREATE INDEX IF NOT EXISTS idx_events_fp_occurred ON events(fingerprint, occurred_at)",
	}
	for _, sql := range extra {
		if err := db.Exec(sql).Error; err != nil {
			// 索引已存在或驱动不支持时忽略
			slog.Debug("skip index", "sql", sql, "err", err)
		}
	}
	return nil
}

// SeedDefault 初始化默认数据：内置厂家与模型、示例项目
func SeedDefault(db *gorm.DB) error {
	var count int64
	db.Model(&model.Provider{}).Count(&count)
	if count > 0 {
		return nil
	}
	providers := []model.Provider{
		{Name: "DeepSeek 官方", Key: "deepseek-official", Kind: "deepseek",
			MaxInputContext: 1048576, MaxOutputContext: 131072, Enabled: true, Remark: "dsh 内置 provider"},
		{Name: "OpenAI", Key: "openai", Kind: "openai", MaxInputContext: 1000000, MaxOutputContext: 65536},
		{Name: "Anthropic Claude", Key: "anthropic", Kind: "anthropic", MaxInputContext: 1000000, MaxOutputContext: 65536},
		{Name: "通义千问（阿里云）", Key: "qwen", Kind: "qwen", MaxInputContext: 1000000, MaxOutputContext: 65536},
		{Name: "智谱 GLM", Key: "zhipu", Kind: "zhipu", MaxInputContext: 1000000, MaxOutputContext: 65536},
		{Name: "月之暗面 Kimi", Key: "moonshot", Kind: "moonshot", MaxInputContext: 1000000, MaxOutputContext: 65536},
		{Name: "豆包（火山引擎）", Key: "doubao", Kind: "doubao", MaxInputContext: 1000000, MaxOutputContext: 65536},
		{Name: "Google Gemini", Key: "gemini", Kind: "gemini", MaxInputContext: 2000000, MaxOutputContext: 65536},
		{Name: "自定义 OpenAI 兼容", Key: "openai-compatible", Kind: "custom", MaxInputContext: 1000000, MaxOutputContext: 65536},
	}
	if err := db.Create(&providers).Error; err != nil {
		return errors.Join(errors.New("seed providers failed"), err)
	}
	ds := providers[0]
	models := []model.LLMModel{
		{ProviderID: ds.ID, Name: "DeepSeek V4 Flash", Slug: "deepseek-v4-flash",
			InputContext: 1048576, OutputContext: 131072, MaxTurns: 128, Enabled: true, IsDefault: true,
			Remark: "默认模型"},
		{ProviderID: ds.ID, Name: "DeepSeek V4 Pro", Slug: "deepseek-v4-pro",
			InputContext: 1048576, OutputContext: 131072, MaxTurns: 256, Enabled: true},
	}
	if err := db.Create(&models).Error; err != nil {
		return err
	}
	slog.Info("seeded default providers and models")
	return nil
}
