package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/automedic/automedic/internal/api"
	"github.com/automedic/automedic/internal/auth"
	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/crypto"
	"github.com/automedic/automedic/internal/logging"
	"github.com/automedic/automedic/internal/model"
	"github.com/automedic/automedic/internal/service"
	"github.com/automedic/automedic/internal/store"
	"github.com/automedic/automedic/internal/ws"
	"gorm.io/gorm"
)

func main() {
	var cfgPath string
	flag.StringVar(&cfgPath, "config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	logging.Init(cfg.Log.Level, cfg.Log.Format)

	key, err := cfg.ResolveSecretKey()
	if err != nil {
		slog.Error("加密主密钥未配置", "err", err)
		os.Exit(1)
	}
	crypt, err := crypto.New(key)
	if err != nil {
		slog.Error("初始化加密服务失败", "err", err)
		os.Exit(1)
	}

	db, err := store.Open(cfg)
	if err != nil {
		slog.Error("数据库连接失败", "err", err, "driver", cfg.DB.Driver)
		os.Exit(1)
	}
	if err := store.AutoMigrate(db); err != nil {
		slog.Error("数据库迁移失败", "err", err)
		os.Exit(1)
	}
	if err := store.SeedDefault(db); err != nil {
		slog.Warn("初始化默认数据失败", "err", err)
	}
	if err := store.SeedDicts(db); err != nil {
		slog.Warn("初始化字典失败", "err", err)
	}

	if err := store.SeedRBAC(db, cfg); err != nil {
		slog.Error("初始化租户与权限数据失败", "err", err)
		os.Exit(1)
	}

	authSvc := auth.NewService(db, cfg)
	hub := ws.NewHub()
	hub.SetSource(wsSource{db: db})
	exec := service.NewExecutor(db, cfg, crypt, hub)
	handlers := api.NewHandlers(db, cfg, exec, crypt, hub, authSvc)

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	exec.Start(rootCtx)
	go janitor(rootCtx, cfg)

	srv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      api.NewRouter(&api.Deps{Cfg: cfg, Handlers: handlers, Auth: authSvc, WebDir: cfg.Server.WebDir}),
		ReadTimeout:  time.Duration(orDefault(cfg.Server.ReadTimeout, 60)) * time.Second,
		WriteTimeout: time.Duration(orDefault(cfg.Server.WriteTimeout, 120)) * time.Second,
	}
	go func() {
		slog.Info("AutoMedic 服务启动", "addr", cfg.Server.Addr, "db", cfg.DB.Driver, "dsh", cfg.DSH.Bin)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("服务异常退出", "err", err)
			os.Exit(1)
		}
	}()

	<-rootCtx.Done()
	slog.Info("正在关闭服务...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("服务关闭异常", "err", err)
	}
	slog.Info("已退出")
}

// wsSource 任务终端 WebSocket 的历史回放与初始状态数据源
type wsSource struct{ db *gorm.DB }

func (s wsSource) History(taskID uint, afterSeq int64, limit int) []ws.LogLine {
	if limit <= 0 {
		limit = 5000
	}
	var logs []model.TaskLog
	s.db.Where("task_id = ? AND seq > ?", taskID, afterSeq).Order("seq ASC").Limit(limit).Find(&logs)
	out := make([]ws.LogLine, 0, len(logs))
	for _, l := range logs {
		out = append(out, ws.LogLine{Seq: l.Seq, Stream: l.Stream, Content: l.Content, CreatedAt: l.CreatedAt})
	}
	return out
}

func (s wsSource) Status(taskID uint) (string, string) {
	var t model.Task
	if err := s.db.Select("status", "stage").First(&t, taskID).Error; err != nil {
		return "", ""
	}
	return string(t.Status), t.Stage
}

// janitor 定期清理过期隔离工作区
func janitor(ctx context.Context, cfg *config.Config) {
	if cfg.Git.KeepDays <= 0 {
		return
	}
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			root := cfg.Git.WorkspaceRoot
			entries, err := os.ReadDir(root)
			if err != nil {
				continue
			}
			cut := time.Now().AddDate(0, 0, -cfg.Git.KeepDays)
			for _, e := range entries {
				p := filepath.Join(root, e.Name())
				info, err := e.Info()
				if err != nil || !info.IsDir() || info.ModTime().After(cut) {
					continue
				}
				if err := os.RemoveAll(p); err == nil {
					slog.Info("清理过期工作区", "path", p)
				}
			}
		}
	}
}

func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}
