package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

func main() {
	logger, err := newLogger()
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := logger.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "flush logger: %v\n", err)
		}
	}()
	mustAbsExportDir()
	database, err := openDB()
	if err != nil {
		logger.Fatal("connect database", zap.Error(err))
	}
	defer database.Close()

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr(), Password: os.Getenv("REDIS_PASSWORD")},
		asynq.Config{Concurrency: 2},
	)

	evaluator, err := newEvaluatorFromEnv()
	if err != nil {
		logger.Fatal("configure llm provider", zap.Error(err))
	}
	handlers := workerHandlers{logger: logger, db: database, evaluator: evaluator, circuit: newAIWorkerCircuitFromEnv(), aiActorID: lookupAIActorID(database, logger)}
	mux := asynq.NewServeMux()
	mux.HandleFunc("ai:review", handlers.handleAIReview)
	mux.HandleFunc("ai:dry-run", handlers.handleAIDryRun)
	mux.HandleFunc("export", handlers.handleExport)
	mux.HandleFunc("noop:ping", handlers.handleNoop)

	logger.Info("AI Worker started", zap.String("redis_addr", redisAddr()))
	if err := srv.Run(mux); err != nil {
		logger.Fatal("AI Worker stopped", zap.Error(err))
	}
}

// newLogger 与 api server 的同名工厂保持一致(两者是独立 module,internal 无法跨 module 共享)。
func newLogger() (*zap.Logger, error) {
	if os.Getenv("GIN_MODE") == "debug" {
		return zap.NewDevelopment()
	}
	return zap.NewProduction()
}

type workerHandlers struct {
	logger    *zap.Logger
	db        *sql.DB
	evaluator aiEvaluator
	circuit   *aiWorkerCircuit
	// aiActorID 是 seed 的 system_ai 账号 id;AI 评审写 audit_logs 时作为 actor_id,
	// 让时间线以独立 AI Agent 账户视角可追溯。nil(未 seed)时落 NULL,不阻塞评审。
	aiActorID *uint64
}

func (h workerHandlers) handleNoop(_ context.Context, t *asynq.Task) error {
	var payload map[string]any
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		h.logger.Warn("invalid noop payload", zap.Error(err), zap.ByteString("payload", t.Payload()))
		return err
	}

	h.logger.Info("received noop ping", zap.Any("payload", payload))
	return nil
}

func openDB() (*sql.DB, error) {
	database, err := sql.Open("mysql", mysqlDSN())
	if err != nil {
		return nil, err
	}
	if err := database.Ping(); err != nil {
		_ = database.Close()
		return nil, err
	}
	return database, nil
}

func mysqlDSN() string {
	// worker 不跑 migration,无需 multiStatements;DB_PASSWORD 必须显式提供,缺失即 fatal。
	password := os.Getenv("DB_PASSWORD")
	if password == "" {
		log.Fatal("DB_PASSWORD must be set")
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=UTC",
		envOrDefault("DB_USER", "labelhub"),
		password,
		envOrDefault("DB_HOST", "127.0.0.1"),
		envOrDefault("DB_PORT", "13306"),
		envOrDefault("DB_NAME", "labelhub"),
	)
}

func aiReviewTimeout() time.Duration {
	value := envOrDefault("AI_REVIEW_TIMEOUT_MS", "30000")
	var ms int
	if _, err := fmt.Sscanf(value, "%d", &ms); err != nil || ms <= 0 {
		return 30 * time.Second
	}
	return time.Duration(ms) * time.Millisecond
}

func redisAddr() string {
	host := envOrDefault("REDIS_HOST", "localhost")
	port := envOrDefault("REDIS_PORT", "6379")
	return net.JoinHostPort(host, port)
}

// mustAbsExportDir 校验 EXPORT_DIR 为绝对路径。worker 与 api 从不同工作目录启动,
// 相对路径会各自解析到不同目录,导致 worker 写入的文件 api 下载时找不到/校验失败。
// 与 api server 的同名函数保持一致(独立 module,不能跨 module 复用)。
func mustAbsExportDir() string {
	dir := os.Getenv("EXPORT_DIR")
	if dir == "" {
		log.Fatal("EXPORT_DIR must be set to an absolute path")
	}
	if !filepath.IsAbs(dir) {
		log.Fatalf("EXPORT_DIR must be an absolute path (api and worker run from different working dirs), got %q", dir)
	}
	return dir
}

// lookupAIActorID 解析 seed 的 system_ai 账号 id,作为 AI 评审审计的 actor_id。
// 账号不存在(未 seed / 精简部署)只告警不阻塞,audit_logs.actor_id 落 NULL。
func lookupAIActorID(database *sql.DB, logger *zap.Logger) *uint64 {
	var id uint64
	err := database.QueryRow(`SELECT id FROM users WHERE username = 'system_ai' LIMIT 1`).Scan(&id)
	if err != nil {
		logger.Warn("system_ai account not found; ai audit entries will have no actor_id", zap.Error(err))
		return nil
	}
	return &id
}

// envOrDefault 与 api 的 internal/envutil.Default 行为一致(internal 包不能跨 module 复用,故各留一份)。
func envOrDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
