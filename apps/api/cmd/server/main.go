package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"labelhub-api/internal/auth"
	"labelhub-api/internal/db"
	"labelhub-api/internal/handler"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/service/aireview"
	"labelhub-api/internal/service/outbox"
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

	database := db.Init()
	db.RunMigrations()
	authService := auth.NewServiceFromEnv()
	outboxCtx, stopOutbox := context.WithCancel(context.Background())
	defer stopOutbox()
	startOutboxPublisher(outboxCtx, database, logger)
	startAIReviewSweeper(outboxCtx, database, logger)

	r := gin.Default()
	r.Use(middleware.CORS())
	r.Use(middleware.RequestID())
	r.GET("/", healthResponse)
	r.GET("/health", healthResponse)

	api := r.Group("/api/v1")
	api.GET("", healthResponse)
	api.GET("/", healthResponse)

	authedAPI := api.Group("")
	authedAPI.Use(middleware.Auth(authService))
	handler.NewAuthHandler(database, authService, authedAPI).Register(api)

	// 按业务域分组注册;新增 handler 在此追加,不再合并进单一 god handler。
	handler.NewTaskHandler(database).Register(authedAPI)
	handler.NewLabelerHandler(database).Register(authedAPI)
	handler.NewReviewerHandler(database).Register(authedAPI)
	handler.NewUploadHandler(database).Register(authedAPI)
	handler.NewLLMHandler().Register(authedAPI)
	handler.NewExportHandler(database).Register(authedAPI)
	handler.NewTemplateHandler(database).Register(authedAPI)
	handler.NewAIPromptHandler(database).Register(authedAPI)
	handler.NewGoldenSampleHandler(database).Register(authedAPI)

	port := serverPort()
	logger.Info("API server starting", zap.String("port", port))
	log.Fatal(r.Run(port))
}

func healthResponse(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"service": "labelhub-api",
			"status":  "ok",
		},
		"request_id": c.GetString(middleware.RequestIDContextKey),
	})
}

func serverPort() string {
	port := os.Getenv("API_PORT")
	if port == "" {
		return ":8080"
	}
	if port[0] == ':' {
		return port
	}
	return ":" + port
}

func startOutboxPublisher(ctx context.Context, database *gorm.DB, logger *zap.Logger) {
	if envOrDefault("OUTBOX_PUBLISHER_ENABLED", "true") == "false" {
		logger.Info("outbox publisher disabled")
		return
	}
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr()})
	publisher := outbox.NewPublisher(database, client, logger, outboxInterval(), outboxBatch())
	go publisher.Run(ctx)
	go func() {
		<-ctx.Done()
		if err := client.Close(); err != nil {
			logger.Warn("close outbox asynq client", zap.Error(err))
		}
	}()
	logger.Info("outbox publisher started", zap.String("redis_addr", redisAddr()))
}

func redisAddr() string {
	return net.JoinHostPort(envOrDefault("REDIS_HOST", "localhost"), envOrDefault("REDIS_PORT", "6379"))
}

func envOrDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func outboxInterval() time.Duration {
	ms, err := strconv.Atoi(envOrDefault("OUTBOX_POLL_INTERVAL_MS", "1000"))
	if err != nil || ms <= 0 {
		return time.Second
	}
	return time.Duration(ms) * time.Millisecond
}

func outboxBatch() int {
	batch, err := strconv.Atoi(envOrDefault("OUTBOX_BATCH", "20"))
	if err != nil || batch <= 0 {
		return 20
	}
	return batch
}

func startAIReviewSweeper(ctx context.Context, database *gorm.DB, logger *zap.Logger) {
	if envOrDefault("AI_REVIEW_SWEEPER_ENABLED", "true") == "false" {
		logger.Info("ai review sweeper disabled")
		return
	}
	sweeper := aireview.NewSweeper(database, logger, aiReviewSweeperInterval(), aiReviewStallTimeout(), aiReviewSweeperBatch())
	go sweeper.Run(ctx)
	logger.Info("ai review sweeper started", zap.Duration("timeout", aiReviewStallTimeout()))
}

func aiReviewSweeperInterval() time.Duration {
	ms, err := strconv.Atoi(envOrDefault("AI_REVIEW_SWEEPER_INTERVAL_MS", "60000"))
	if err != nil || ms <= 0 {
		return time.Minute
	}
	return time.Duration(ms) * time.Millisecond
}

func aiReviewStallTimeout() time.Duration {
	ms, err := strconv.Atoi(envOrDefault("AI_REVIEW_STALL_TIMEOUT_MS", "300000"))
	if err != nil || ms <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(ms) * time.Millisecond
}

func aiReviewSweeperBatch() int {
	batch, err := strconv.Atoi(envOrDefault("AI_REVIEW_SWEEPER_BATCH", "20"))
	if err != nil || batch <= 0 {
		return 20
	}
	return batch
}
