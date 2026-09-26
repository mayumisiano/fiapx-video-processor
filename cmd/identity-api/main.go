package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	identityapplication "video-processor/internal/identity/application"
	identityhttp "video-processor/internal/identity/http"
	identityjwt "video-processor/internal/identity/jwt"
	identitypostgres "video-processor/internal/identity/postgres"
	"video-processor/internal/identity/ratelimit"
	"video-processor/internal/platform/config"
	"video-processor/internal/platform/httpcors"
	"video-processor/internal/platform/metrics"
	"video-processor/internal/platform/migrate"
	platformpostgres "video-processor/internal/platform/postgres"
	platformredis "video-processor/internal/platform/redis"
	identitymigrations "video-processor/migrations/identity"
)

// Login rate limit: 5 attempts per IP per minute (docs/adr/0010).
const (
	loginRateLimitMax    = 5
	loginRateLimitWindow = time.Minute
)

func main() {
	cfg := config.Load()

	if err := migrate.Run(cfg.IdentityDatabaseURL, identitymigrations.FS); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}

	ctx := context.Background()
	pool, err := platformpostgres.NewPool(ctx, cfg.IdentityDatabaseURL)
	if err != nil {
		log.Fatalf("connect to postgres: %v", err)
	}
	defer pool.Close()

	redisClient, err := platformredis.NewClient(cfg.RedisURL)
	if err != nil {
		log.Fatalf("connect to redis: %v", err)
	}
	defer redisClient.Close()

	limiter := ratelimit.NewLimiter(ratelimit.NewRedisStore(redisClient), loginRateLimitMax, loginRateLimitWindow)

	repo := identitypostgres.NewRepository(pool)
	issuer := identityjwt.NewIssuer(cfg.JWTSecret)
	service := identityapplication.NewService(repo, issuer)
	handler := identityhttp.NewHandler(service)

	router := gin.Default()
	router.Use(httpcors.Middleware(cfg.FrontendOrigin))
	router.Use(metrics.GinMiddleware("identity-api"))

	router.GET("/health", func(c *gin.Context) {
		if err := pool.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/metrics", gin.WrapH(metrics.Handler()))

	v1 := router.Group("/api/v1")
	handler.RegisterRoutes(v1, identityhttp.RateLimitLogin(limiter))

	if err := router.Run(":" + cfg.IdentityPort); err != nil {
		log.Fatalf("run server: %v", err)
	}
}
