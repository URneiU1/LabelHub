package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders 给所有响应加上基础安全响应头。
// 这些头开销极低,却能挡住点击劫持、MIME 嗅探,并收敛 Referer 泄漏。
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}
