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
	platformrabbitmq "video-processor/internal/platform/rabbitmq"
	videohttp "video-processor/internal/videoprocessing/http"
	videopostgres "video-processor/internal/videoprocessing/postgres"
	videoqueue "video-processor/internal/videoprocessing/queue"
	videostorage "video-processor/internal/videoprocessing/storage"
)

func main() {
	cfg := config.Load()

	ctx := context.Background()
	pool, err := platformpostgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect to postgres: %v", err)
	}
	defer pool.Close()

	rabbitConn, err := platformrabbitmq.Connect(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("connect to rabbitmq: %v", err)
	}
	defer rabbitConn.Close()

	publisher, err := videoqueue.NewPublisher(rabbitConn)
	if err != nil {
		log.Fatalf("declare queue topology: %v", err)
	}

	storageClient, err := videostorage.NewClient(
		cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey,
		cfg.MinIOVideosBucket, cfg.MinIOResultsBucket,
	)
	if err != nil {
		log.Fatalf("connect to minio: %v", err)
	}

	identityRepo := identitypostgres.NewRepository(pool)
	issuer := identityjwt.NewIssuer(cfg.JWTSecret)
	identityHandler := identityhttp.NewHandler(identityRepo, issuer)

	videoRepo := videopostgres.NewRepository(pool)
	videoHandler := videohttp.NewHandler(videoRepo, storageClient, publisher)

	router := gin.Default()
	v1 := router.Group("/api/v1")
	identityHandler.RegisterRoutes(v1)
	videoHandler.RegisterRoutes(v1, identityHandler.RequireAuth())

	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("run server: %v", err)
	}
}
