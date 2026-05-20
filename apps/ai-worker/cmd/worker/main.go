package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: "localhost:6379"},
		asynq.Config{Concurrency: 2},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc("ai:review", handleAIReview)
	mux.HandleFunc("noop:ping", handleNoop)

	logger.Info("AI Worker started")
	if err := srv.Run(mux); err != nil {
		log.Fatal(err)
	}
}

func handleNoop(ctx context.Context, t *asynq.Task) error {
	var payload map[string]any
	json.Unmarshal(t.Payload(), &payload)
	log.Printf("[noop] received ping: %v", payload)
	return nil
}

func handleAIReview(ctx context.Context, t *asynq.Task) error {
	// Sprint 3 实现,当前占位
	log.Println("[ai:review] placeholder — real impl in Sprint 3")
	return nil
}
