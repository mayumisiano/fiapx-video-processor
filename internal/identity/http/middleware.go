package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"video-processor/internal/identity/ratelimit"
)

// RateLimitLogin throttles login attempts per client IP (docs/adr/0010).
// It counts every attempt, not just failed ones — counting all attempts is
// simpler and still stops brute-force, since a legitimate user rarely
// calls login more than a handful of times per minute.
func RateLimitLogin(limiter *ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := "login_attempts:" + c.ClientIP()
		if !limiter.Allow(c.Request.Context(), key) {
			errorResponse(c, http.StatusTooManyRequests, "RATE_LIMITED", "Too many login attempts, try again later")
			c.Abort()
			return
		}
		c.Next()
	}
}
