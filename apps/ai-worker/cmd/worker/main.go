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
	logger, err := zap.NewDevelopment()
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
		asynq.RedisClientOpt{Addr: redisAddr()},
		asynq.Config{Concurrency: 2},
	)

	evaluator, err := newEvaluatorFromEnv()
	if err != nil {
		logger.Fatal("configure llm provider", zap.Error(err))
	}
	handlers := workerHandlers{logger: logger, db: database, evaluator: evaluator, circuit: newAIWorkerCircuitFromEnv()}
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

type workerHandlers struct {
	logger    *zap.Logger
	db        *sql.DB
	evaluator aiEvaluator
	circuit   *aiWorkerCircuit
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

func envOrDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
