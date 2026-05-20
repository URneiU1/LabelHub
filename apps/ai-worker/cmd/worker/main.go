package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"

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

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr()},
		asynq.Config{Concurrency: 2},
	)

	handlers := workerHandlers{logger: logger}
	mux := asynq.NewServeMux()
	mux.HandleFunc("ai:review", handlers.handleAIReview)
	mux.HandleFunc("noop:ping", handlers.handleNoop)

	logger.Info("AI Worker started", zap.String("redis_addr", redisAddr()))
	if err := srv.Run(mux); err != nil {
		logger.Fatal("AI Worker stopped", zap.Error(err))
	}
}

type workerHandlers struct {
	logger *zap.Logger
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

func (h workerHandlers) handleAIReview(_ context.Context, _ *asynq.Task) error {
	// Sprint 3 实现,当前占位
	h.logger.Info("ai review placeholder")
	return nil
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
