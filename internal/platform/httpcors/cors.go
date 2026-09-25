// Package httpcors provides a minimal CORS middleware for Gin, scoped to a
// single allowed origin (the frontend). A hand-rolled middleware avoids
// pulling in gin-contrib/cors, whose latest release forces a Gin upgrade
// that drags in an unrelated dependency tree (quic-go, mongo-driver).
package httpcors

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func Middleware(allowedOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", allowedOrigin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
