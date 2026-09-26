package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"video-processor/internal/platform/config"
	"video-processor/internal/platform/httpcors"
	"video-processor/internal/platform/metrics"
	"video-processor/internal/platform/migrate"
	platformpostgres "video-processor/internal/platform/postgres"
	platformrabbitmq "video-processor/internal/platform/rabbitmq"
	videoapplication "video-processor/internal/videoprocessing/application"
	videoffmpeg "video-processor/internal/videoprocessing/ffmpeg"
	videohttp "video-processor/internal/videoprocessing/http"
	videopostgres "video-processor/internal/videoprocessing/postgres"
	videoqueue "video-processor/internal/videoprocessing/queue"
	videostorage "video-processor/internal/videoprocessing/storage"
	videomigrations "video-processor/migrations/video"
)

func main() {
	cfg := config.Load()

	if err := migrate.Run(cfg.VideoDatabaseURL, videomigrations.FS); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}

	ctx := context.Background()
	pool, err := platformpostgres.NewPool(ctx, cfg.VideoDatabaseURL)
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
		cfg.MinIOEndpoint, cfg.MinIOPublicEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey,
		cfg.MinIOVideosBucket, cfg.MinIOResultsBucket,
	)
	if err != nil {
		log.Fatalf("connect to minio: %v", err)
	}
	if err := storageClient.EnsureBuckets(ctx); err != nil {
		log.Fatalf("ensure minio buckets: %v", err)
	}

	videoRepo := videopostgres.NewRepository(pool)
	extractor := videoffmpeg.NewExtractor()
	videoService := videoapplication.NewService(videoRepo, storageClient, publisher, extractor)
	videoHandler := videohttp.NewHandler(videoService)

	router := gin.Default()
	router.Use(httpcors.Middleware(cfg.FrontendOrigin))
	router.Use(metrics.GinMiddleware("video-api"))

	// Unversioned: consumed by orchestrators (compose/k8s probes) or
	// Prometheus, not API clients, so neither is tied to the /api/v1
	// contract's lifecycle.
	router.GET("/health", func(c *gin.Context) {
		if err := pool.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/metrics", gin.WrapH(metrics.Handler()))

	v1 := router.Group("/api/v1")
	videoHandler.RegisterRoutes(v1, videohttp.RequireAuth(cfg.JWTSecret))

	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("run server: %v", err)
	}
}
