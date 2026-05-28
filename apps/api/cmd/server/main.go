package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"labelhub-api/internal/auth"
	"labelhub-api/internal/db"
	"labelhub-api/internal/handler"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/service/aireview"
	"labelhub-api/internal/service/outbox"
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
	database := db.Init()
	db.RunMigrations()
	authService := auth.NewServiceFromEnv()
	outboxCtx, stopOutbox := context.WithCancel(context.Background())
	defer stopOutbox()
	startOutboxPublisher(outboxCtx, database, logger)
	startAIReviewSweeper(outboxCtx, database, logger)
	startOrphanTempFileCleaner(outboxCtx, database, logger)

	// 默认 release 模式(不打印调试路由/警告);本地调试可显式设 GIN_MODE=debug。
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(os.Getenv("GIN_MODE"))
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.SecurityHeaders())
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
	exportHandler := handler.NewExportHandler(database, exportDownloadSecret(), exportDownloadTTL())
	exportHandler.Register(authedAPI)
	exportHandler.RegisterPublic(api) // 公开下载路由, 签名 token 即鉴权
	handler.NewStatsHandler(database).Register(authedAPI)
	handler.NewTemplateHandler(database).Register(authedAPI)
	handler.NewAIPromptHandler(database).Register(authedAPI)
	handler.NewGoldenSampleHandler(database).Register(authedAPI)
	handler.NewAIDryRunHandler(database).Register(authedAPI)

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

func newLogger() (*zap.Logger, error) {
	if os.Getenv("GIN_MODE") == "debug" {
		return zap.NewDevelopment()
	}
	return zap.NewProduction()
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

// mustAbsExportDir 校验 EXPORT_DIR 为绝对路径并返回它。
// api 与 worker 从不同工作目录启动(见 Makefile),相对路径会各自解析到不同目录,
// 导致 worker 写入的文件 api 下载时 base 不匹配,safeExportPath 永远 403。
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

func exportDownloadSecret() string { return os.Getenv("EXPORT_DOWNLOAD_SECRET") }

func exportDownloadTTL() time.Duration {
	seconds, err := strconv.Atoi(os.Getenv("EXPORT_DOWNLOAD_TTL"))
	if err != nil || seconds <= 0 {
		seconds = 600
	}
	return time.Duration(seconds) * time.Second
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

func startOrphanTempFileCleaner(ctx context.Context, database *gorm.DB, logger *zap.Logger) {
	if envOrDefault("ORPHAN_TEMP_FILE_CLEANER_ENABLED", "true") == "false" {
		logger.Info("orphan temp file cleaner disabled")
		return
	}
	interval := orphanTempFileCleanerInterval()
	ticker := time.NewTicker(interval)
	logger.Info("orphan temp file cleaner started", zap.Duration("interval", interval))
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().Add(-24 * time.Hour)
				orphans, err := claimOrphanTempFiles(ctx, database, cutoff, orphanTempFileCleanerBatch())
				if err != nil {
					logger.Warn("failed to query orphan temp files", zap.Error(err))
					continue
				}
				uploadDir := envOrDefault("UPLOAD_DIR", "./data/uploads")
				for _, f := range orphans {
					dest := filepath.Join(uploadDir, strconv.FormatUint(f.TaskID, 10), f.StorageKey)
					if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
						logger.Warn("failed to remove orphan temp file", zap.Uint64("id", f.ID), zap.String("path", dest), zap.Error(err))
						continue
					}
					logger.Info("cleaned up orphan temp file", zap.Uint64("id", f.ID), zap.String("storage_key", f.StorageKey))
				}
			}
		}
	}()
}

func claimOrphanTempFiles(ctx context.Context, database *gorm.DB, cutoff time.Time, batch int) ([]model.UploadedFile, error) {
	var orphans []model.UploadedFile
	err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND created_at < ?", "temp", cutoff).
			Order("id ASC").
			Limit(batch).
			Find(&orphans).Error; err != nil {
			return err
		}
		if len(orphans) == 0 {
			return nil
		}
		ids := make([]uint64, 0, len(orphans))
		for _, f := range orphans {
			ids = append(ids, f.ID)
		}
		res := tx.Model(&model.UploadedFile{}).
			Where("id IN ? AND status = ?", ids, "temp").
			Update("status", "deleted")
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != int64(len(orphans)) {
			return fmt.Errorf("orphan temp file claim lost update race")
		}
		return nil
	})
	return orphans, err
}

func orphanTempFileCleanerInterval() time.Duration {
	ms, err := strconv.Atoi(envOrDefault("ORPHAN_TEMP_FILE_CLEANER_INTERVAL_MS", "3600000"))
	if err != nil || ms <= 0 {
		return time.Hour
	}
	return time.Duration(ms) * time.Millisecond
}

func orphanTempFileCleanerBatch() int {
	batch, err := strconv.Atoi(envOrDefault("ORPHAN_TEMP_FILE_CLEANER_BATCH", "100"))
	if err != nil || batch <= 0 {
		return 100
	}
	return batch
}
