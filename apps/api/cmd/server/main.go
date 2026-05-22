package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"labelhub-api/internal/auth"
	"labelhub-api/internal/db"
	"labelhub-api/internal/handler"
	"labelhub-api/internal/middleware"
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
