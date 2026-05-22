package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
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
	handlers := workerHandlers{logger: logger, db: database, evaluator: evaluator}
	mux := asynq.NewServeMux()
	mux.HandleFunc("ai:review", handlers.handleAIReview)
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
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=UTC&multiStatements=true",
		envOrDefault("DB_USER", "labelhub"),
		envOrDefault("DB_PASSWORD", "labelhub_dev"),
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

func envOrDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
