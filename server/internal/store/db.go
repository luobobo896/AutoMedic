package store

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(cfg *config.Config) (*gorm.DB, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.DB.Driver))
	if driver == "" {
		driver = "postgres"
	}
	if driver != "postgres" && driver != "postgresql" && driver != "pg" {
		return nil, fmt.Errorf("unsupported db driver %q: only postgres is supported", cfg.DB.Driver)
	}
	if strings.TrimSpace(cfg.DB.DSN) == "" {
		return nil, errors.New("db.dsn is empty")
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

	db, err := gorm.Open(postgres.Open(cfg.DB.DSN), &gorm.Config{
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

// AutoMigrate 建表并补齐索引；生产环境可改用 docs/database 脚本预建后仍可幂等执行。
func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(model.All()...); err != nil {
		return err
	}
	extra := []string{
		"CREATE INDEX IF NOT EXISTS idx_tasks_status_created ON tasks(status, created_at)",
		"CREATE INDEX IF NOT EXISTS idx_tasks_project_created ON tasks(project_id, created_at)",
		"CREATE INDEX IF NOT EXISTS idx_task_logs_task_seq ON task_logs(task_id, seq)",
		"CREATE INDEX IF NOT EXISTS idx_events_project_occurred ON events(project_id, occurred_at)",
		"CREATE INDEX IF NOT EXISTS idx_events_fp_occurred ON events(fingerprint, occurred_at)",
		"CREATE INDEX IF NOT EXISTS idx_review_jobs_repo_created ON review_jobs(repo_id, created_at)",
	}
	for _, sql := range extra {
		if err := db.Exec(sql).Error; err != nil {
			slog.Debug("skip index", "sql", sql, "err", err)
		}
	}
	return nil
}

// SeedDefault 初始化默认数据：内置厂家与模型
func SeedDefault(db *gorm.DB) error {
	var count int64
	db.Model(&model.Provider{}).Count(&count)
	if count > 0 {
		return nil
	}
	providers := []model.Provider{
		{Name: "DeepSeek 官方", Key: "deepseek-official", Kind: "deepseek",
			Enabled: true, Remark: "dsh 内置 provider"},
		{Name: "OpenAI", Key: "openai", Kind: "openai"},
		{Name: "Anthropic Claude", Key: "anthropic", Kind: "anthropic"},
		{Name: "通义千问（阿里云）", Key: "qwen", Kind: "qwen"},
		{Name: "智谱 GLM", Key: "zhipu", Kind: "zhipu"},
		{Name: "月之暗面 Kimi", Key: "moonshot", Kind: "moonshot"},
		{Name: "豆包（火山引擎）", Key: "doubao", Kind: "doubao"},
		{Name: "Google Gemini", Key: "gemini", Kind: "gemini"},
		{Name: "自定义 OpenAI 兼容", Key: "openai-compatible", Kind: "custom"},
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
