package handler

import (
	"errors"
	"net/http"

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
	router.POST("/auth/login", h.Login)
	router.POST("/auth/refresh", h.Refresh)
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

	user, ok := h.findActiveUser(c, claims.Username)
	if !ok {
		return
	}
	h.respondWithTokens(c, user)
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

// Logout 是无服务端状态的"登出",由前端 clearToken() 负责真正失效。
// 当前 JWT 无 revocation 列表 — 已签发的 access/refresh token 在过期前仍有效。
// Sprint 5 工程质量收尾时引入 refresh token 持久化 + 黑名单后,这里改成真正撤销。
func (h AuthHandler) Logout(c *gin.Context) {
	httpx.OK(c, gin.H{
		"ok":   true,
		"note": "client must clear local tokens; server-side revocation pending (Sprint 5)",
	})
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
