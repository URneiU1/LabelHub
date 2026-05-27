package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/auth"
	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
)

type AuthHandler struct {
	db         *gorm.DB
	auth       *auth.Service
	authRouter gin.IRouter
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
}

func NewAuthHandler(db *gorm.DB, authService *auth.Service, authRouter gin.IRouter) AuthHandler {
	return AuthHandler{
		db:         db,
		auth:       authService,
		authRouter: authRouter,
	}
}

func (h AuthHandler) Register(router gin.IRouter) {
	// 登录/刷新是无需认证、可被暴力枚举的端点,共用一个按 IP 的令牌桶:10 次突发后每 6s 补 1 次。
	authLimiter := middleware.RateLimit(6*time.Second, 10)
	router.POST("/auth/login", authLimiter, h.Login)
	router.POST("/auth/refresh", authLimiter, h.Refresh)
	h.authRouter.GET("/me", h.Me)
	h.authRouter.POST("/auth/logout", h.Logout)
}

func (h AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "username and password are required")
		return
	}

	user, ok := h.findActiveUser(c, req.Username)
	if !ok {
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid username or password")
		return
	}

	h.respondWithTokens(c, user)
}

func (h AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "refreshToken is required")
		return
	}

	claims, err := h.auth.Parse(req.RefreshToken, auth.TokenTypeRefresh)
	if err != nil {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid refresh token")
		return
	}

	// 校验该 refresh token 服务端仍有效(存在、未撤销、未过期)。
	if !h.refreshTokenActive(c, claims.ID) {
		return
	}

	user, ok := h.findActiveUser(c, claims.Username)
	if !ok {
		return
	}

	// 轮换:签发新对前先撤销旧 jti,防止 refresh token 被重放。
	if err := h.db.Model(&model.RefreshToken{}).
		Where("jti = ? AND revoked_at IS NULL", claims.ID).
		Update("revoked_at", time.Now().UTC()).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to rotate refresh token")
		return
	}
	h.respondWithTokens(c, user)
}

// refreshTokenActive 校验给定 jti 在 refresh_tokens 表中存在、未撤销且未过期。
// 校验不通过时已写入 401/500 响应,返回 false。
func (h AuthHandler) refreshTokenActive(c *gin.Context, jti string) bool {
	if jti == "" {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid refresh token")
		return false
	}
	var record model.RefreshToken
	err := h.db.Where("jti = ?", jti).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "refresh token revoked or expired")
		return false
	}
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to verify refresh token")
		return false
	}
	if record.RevokedAt != nil || record.ExpiresAt.Before(time.Now().UTC()) {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "refresh token revoked or expired")
		return false
	}
	return true
}

func (h AuthHandler) Me(c *gin.Context) {
	claims, ok := middleware.Claims(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing auth context")
		return
	}

	user, ok := h.findActiveUser(c, claims.Username)
	if !ok {
		return
	}

	httpx.OK(c, gin.H{
		"id":          user.ID,
		"username":    user.Username,
		"displayName": user.DisplayName,
		"email":       user.Email,
		"roles":       auth.RolesOf(user),
	})
}

// Logout 撤销当前用户全部未撤销的 refresh token(服务端登出),使其无法再换取新 access token。
// 注意:已签发的 access token(短 TTL)在过期前仍可用 —— 这是 JWT 的标准取舍,
// 全量 access 撤销需每请求查黑名单,代价过高,故仅撤 refresh。
func (h AuthHandler) Logout(c *gin.Context) {
	claims, ok := middleware.Claims(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing auth context")
		return
	}
	if err := h.db.Model(&model.RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", claims.UserID).
		Update("revoked_at", time.Now().UTC()).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to revoke sessions")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h AuthHandler) findActiveUser(c *gin.Context, username string) (model.User, bool) {
	var user model.User
	err := h.db.Preload("Roles").Where("username = ? AND status = ?", username, "active").First(&user).Error
	if err == nil {
		return user, true
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid username or password")
		return model.User{}, false
	}

	httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load user")
	return model.User{}, false
}

func (h AuthHandler) respondWithTokens(c *gin.Context, user model.User) {
	tokens, err := h.auth.GenerateTokenPair(user, auth.RolesOf(user))
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to issue token")
		return
	}

	// 持久化新签发的 refresh token,以支持后续撤销/轮换校验。
	if err := h.db.Create(&model.RefreshToken{
		JTI:       tokens.RefreshJTI,
		UserID:    user.ID,
		ExpiresAt: tokens.RefreshTokenExpiresAt,
	}).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to persist session")
		return
	}

	httpx.OK(c, gin.H{
		"user": gin.H{
			"id":          user.ID,
			"username":    user.Username,
			"displayName": user.DisplayName,
			"email":       user.Email,
			"roles":       auth.RolesOf(user),
		},
		"tokens": tokens,
	})
}
