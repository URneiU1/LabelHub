package auth

import (
	"errors"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"labelhub-api/internal/model"
)

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

var ErrInvalidToken = errors.New("invalid token")

type Claims struct {
	UserID    uint64   `json:"userId"`
	Username  string   `json:"username"`
	Roles     []string `json:"roles"`
	TokenType string   `json:"tokenType"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken           string    `json:"accessToken"`
	RefreshToken          string    `json:"refreshToken"`
	AccessTokenExpiresAt  time.Time `json:"accessTokenExpiresAt"`
	RefreshTokenExpiresAt time.Time `json:"refreshTokenExpiresAt"`
}

type Service struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewServiceFromEnv() *Service {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("JWT_SECRET 必须设置(.env 复制自 .env.example 后再启动 API)")
	}
	if len(secret) < 32 {
		log.Fatal("JWT_SECRET 长度必须 >= 32 字符,避免 HS256 暴力枚举")
	}
	return &Service{
		secret:     []byte(secret),
		accessTTL:  durationEnv("JWT_ACCESS_EXPIRY", 2*time.Hour),
		refreshTTL: durationEnv("JWT_REFRESH_EXPIRY", 14*24*time.Hour),
	}
}

// NewServiceForTest 仅供单元测试使用,绕过 env 校验
func NewServiceForTest(secret string, accessTTL, refreshTTL time.Duration) *Service {
	return &Service{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func (s *Service) GenerateTokenPair(user model.User, roles []string) (TokenPair, error) {
	now := time.Now().UTC()
	accessExpiresAt := now.Add(s.accessTTL)
	refreshExpiresAt := now.Add(s.refreshTTL)

	accessToken, err := s.sign(user, roles, TokenTypeAccess, now, accessExpiresAt)
	if err != nil {
		return TokenPair{}, err
	}

	refreshToken, err := s.sign(user, roles, TokenTypeRefresh, now, refreshExpiresAt)
	if err != nil {
		return TokenPair{}, err
	}

	return TokenPair{
		AccessToken:           accessToken,
		RefreshToken:          refreshToken,
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshTokenExpiresAt: refreshExpiresAt,
	}, nil
}

func (s *Service) Parse(tokenText string, expectedType string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenText, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	if claims.TokenType != expectedType {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func (s *Service) sign(user model.User, roles []string, tokenType string, issuedAt time.Time, expiresAt time.Time) (string, error) {
	claims := Claims{
		UserID:    user.ID,
		Username:  user.Username,
		Roles:     roles,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(user.ID, 10),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hashed), err
}

func CheckPassword(passwordHash string, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) == nil
}

func RolesOf(user model.User) []string {
	roles := make([]string, 0, len(user.Roles))
	for _, role := range user.Roles {
		roles = append(roles, role.Role)
	}
	return roles
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return duration
}
