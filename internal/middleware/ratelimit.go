package middleware

import (
	"net/http"
	"sync"
	"time"

	"api-v2/internal/models"

	"github.com/gin-gonic/gin"
)

type clientInfo struct {
	count    int
	lastSeen time.Time
}

// RateLimiter implementa rate limiting por IP usando janela fixa
type RateLimiter struct {
	clients sync.Map
	limit   int
	window  time.Duration
}

// NewRateLimiter cria um novo rate limiter
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{limit: limit, window: window}

	// Goroutine para limpeza periódica de entradas expiradas
	go func() {
		ticker := time.NewTicker(window)
		defer ticker.Stop()
		for range ticker.C {
			rl.clients.Range(func(key, value interface{}) bool {
				info := value.(*clientInfo)
				if time.Since(info.lastSeen) > window {
					rl.clients.Delete(key)
				}
				return true
			})
		}
	}()

	return rl
}

// Middleware retorna o gin handler de rate limiting
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()

		val, _ := rl.clients.LoadOrStore(ip, &clientInfo{count: 0, lastSeen: now})
		info := val.(*clientInfo)

		// Resetar janela se expirou
		if now.Sub(info.lastSeen) > rl.window {
			info.count = 0
			info.lastSeen = now
		}

		info.count++
		if info.count > rl.limit {
			c.JSON(http.StatusTooManyRequests, models.NewErrorResponse("Rate limit excedido"))
			c.Abort()
			return
		}

		c.Next()
	}
}
