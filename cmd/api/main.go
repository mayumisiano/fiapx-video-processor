package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"

	identityhttp "video-processor/internal/identity/http"
	identityjwt "video-processor/internal/identity/jwt"
	identitypostgres "video-processor/internal/identity/postgres"
	"video-processor/internal/platform/config"
	platformpostgres "video-processor/internal/platform/postgres"
)

func main() {
	cfg := config.Load()

	ctx := context.Background()
	pool, err := platformpostgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect to postgres: %v", err)
	}
	defer pool.Close()

	repo := identitypostgres.NewRepository(pool)
	issuer := identityjwt.NewIssuer(cfg.JWTSecret)
	handler := identityhttp.NewHandler(repo, issuer)

	router := gin.Default()
	v1 := router.Group("/api/v1")
	handler.RegisterRoutes(v1)

	// TODO: wire RabbitMQ, MinIO and the remaining HTTP handlers.
	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("run server: %v", err)
	}
}
