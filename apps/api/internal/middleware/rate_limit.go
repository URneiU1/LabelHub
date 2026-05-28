package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"labelhub-api/internal/httpx"
)

// RateLimit 按客户端 IP 做令牌桶限流,用于保护登录/刷新等敏感端点免遭暴力枚举。
// every:补充一个令牌的间隔(如 6s = 10 次/分钟);burst:允许的突发上限。
// 空闲 IP 由后台 goroutine 定期清理,避免 visitors map 无界增长。
func RateLimit(every time.Duration, burst int) gin.HandlerFunc {
	store := &ipLimiterStore{
		visitors: make(map[string]*ipVisitor),
		every:    rate.Every(every),
		burst:    burst,
	}
	store.startCleanup(10 * time.Minute)

	return func(c *gin.Context) {
		if !store.get(c.ClientIP()).Allow() {
			httpx.Error(c, http.StatusTooManyRequests, "RATE_LIMITED", "请求过于频繁,请稍后再试 / Too many requests, please slow down")
			c.Abort()
			return
		}
		c.Next()
	}
}

type ipVisitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type ipLimiterStore struct {
	mu       sync.Mutex
	visitors map[string]*ipVisitor
	every    rate.Limit
	burst    int
}

func (s *ipLimiterStore) get(ip string) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.visitors[ip]
	if !ok {
		v = &ipVisitor{limiter: rate.NewLimiter(s.every, s.burst)}
		s.visitors[ip] = v
	}
	v.lastSeen = time.Now()
	return v.limiter
}

func (s *ipLimiterStore) startCleanup(idle time.Duration) {
	go func() {
		ticker := time.NewTicker(idle)
		defer ticker.Stop()
		for range ticker.C {
			s.mu.Lock()
			for ip, v := range s.visitors {
				if time.Since(v.lastSeen) > idle {
					delete(s.visitors, ip)
				}
			}
			s.mu.Unlock()
		}
	}()
}
