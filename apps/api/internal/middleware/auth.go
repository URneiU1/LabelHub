package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"labelhub-api/internal/auth"
	"labelhub-api/internal/httpx"
)

const ClaimsContextKey = "authClaims"

func Auth(service *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token")
			c.Abort()
			return
		}

		claims, err := service.Parse(strings.TrimPrefix(header, "Bearer "), auth.TokenTypeAccess)
		if err != nil {
			httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid bearer token")
			c.Abort()
			return
		}

		c.Set(ClaimsContextKey, claims)
		c.Next()
	}
}

func RequireRoles(allowedRoles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedRoles))
	for _, role := range allowedRoles {
		allowed[role] = struct{}{}
	}

	return func(c *gin.Context) {
		claims, ok := Claims(c)
		if !ok {
			httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing auth context")
			c.Abort()
			return
		}

		for _, role := range claims.Roles {
			if _, ok := allowed[role]; ok {
				c.Next()
				return
			}
		}

		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "role is not allowed")
		c.Abort()
	}
}

func Claims(c *gin.Context) (*auth.Claims, bool) {
	value, exists := c.Get(ClaimsContextKey)
	if !exists {
		return nil, false
	}

	claims, ok := value.(*auth.Claims)
	return claims, ok
}
