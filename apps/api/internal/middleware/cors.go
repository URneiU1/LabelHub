package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"labelhub-api/internal/envutil"
)

func CORS() gin.HandlerFunc {
	allowedOrigins := strings.Split(envutil.Default("API_CORS_ORIGINS", "http://localhost:5173"), ",")

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && originAllowed(origin, allowedOrigins) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-Id")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Expose-Headers", "X-Request-Id")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func originAllowed(origin string, allowed []string) bool {
	for _, item := range allowed {
		item = strings.TrimSpace(item)
		if item == origin {
			return true
		}
	}
	return false
}
