package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	platformjwt "video-processor/internal/platform/jwt"
)

// RequireAuth verifies the bearer token locally against the shared JWT
// secret — no call to identity-api. Video Processing and Identity share a
// signing secret, not a database (see docs/adr/0007).
func RequireAuth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		const prefix = "Bearer "
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, prefix) {
			errorResponse(c, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid Authorization header")
			c.Abort()
			return
		}

		claims, err := platformjwt.Verify(jwtSecret, strings.TrimPrefix(header, prefix))
		if err != nil {
			errorResponse(c, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or expired token")
			c.Abort()
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("userEmail", claims.Email)
		c.Next()
	}
}
