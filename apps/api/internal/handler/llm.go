package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
)

// LLMHandler 封装 /llm/* 端点。Sprint 1 阶段只有 mock /llm/inline;
// Sprint 3 接真实豆包后,这里会扩成"前端在线辅助"(inline)与"worker 异步审核"两条线。
type LLMHandler struct{}

func NewLLMHandler() LLMHandler {
	return LLMHandler{}
}

func (h LLMHandler) Register(api gin.IRouter) {
	api.POST("/llm/inline", middleware.RequireRoles("labeler", "owner", "reviewer", "admin"), h.InlineLLM)
}

type inlineLLMRequest struct {
	Prompt string         `json:"prompt"`
	Input  map[string]any `json:"input"`
}

// InlineLLM:Sprint 1 mock 实现。返回固定字符串 + 模拟 800ms 延迟;Sprint 3 替换为豆包 Function Calling。
func (h LLMHandler) InlineLLM(c *gin.Context) {
	var req inlineLLMRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}
	time.Sleep(800 * time.Millisecond)
	text := fmt.Sprintf("AI 预评分:建议重点检查相关性、准确性、格式合规与安全性。输入长度=%d。", len(req.Prompt)+len(fmt.Sprint(req.Input)))
	httpx.OK(c, gin.H{"text": text, "provider": "mock"})
}
