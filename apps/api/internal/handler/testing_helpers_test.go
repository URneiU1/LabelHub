package handler

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// registerAllHandlers:在测试里一次性注册 6 个业务 handler。
// 生产 main.go 显式列出每个 NewXHandler 的 Register 调用,故此 helper 只在 _test.go 编译。
func registerAllHandlers(r gin.IRouter, db *gorm.DB) {
	NewTaskHandler(db).Register(r)
	NewLabelerHandler(db).Register(r)
	NewReviewerHandler(db).Register(r)
	NewUploadHandler(db).Register(r)
	NewLLMHandler().Register(r)
	NewExportHandler(db).Register(r)
	NewTemplateHandler(db).Register(r)
	NewAIPromptHandler(db).Register(r)
	NewGoldenSampleHandler(db).Register(r)
	NewAIDryRunHandler(db).Register(r)
}
