package main

import (
	"context"
	"log"

	notificationsmtp "video-processor/internal/notification/smtp"
	"video-processor/internal/platform/config"
	"video-processor/internal/platform/metrics"
	"video-processor/internal/platform/migrate"
	platformpostgres "video-processor/internal/platform/postgres"
	platformrabbitmq "video-processor/internal/platform/rabbitmq"
	videoapplication "video-processor/internal/videoprocessing/application"
	videoffmpeg "video-processor/internal/videoprocessing/ffmpeg"
	videopostgres "video-processor/internal/videoprocessing/postgres"
	videoqueue "video-processor/internal/videoprocessing/queue"
	videostorage "video-processor/internal/videoprocessing/storage"
	videoworker "video-processor/internal/videoprocessing/worker"
	videomigrations "video-processor/migrations/video"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	if err := migrate.Run(cfg.VideoDatabaseURL, videomigrations.FS); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}

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

	queueConsumer, err := videoqueue.NewConsumer(rabbitConn)
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

	repo := videopostgres.NewRepository(pool)
	mailer := notificationsmtp.NewMailer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPFrom)
	extractor := videoffmpeg.NewExtractor()

	// No dependency on Identity: the notified email is req.UserEmail, a
	// value already carried on the ProcessingRequest since upload time
	// (see docs/adr/0007).
	processor := videoapplication.NewProcessor(repo, storageClient, extractor, mailer)
	consumer := videoworker.NewConsumer(processor)

	deliveries, err := queueConsumer.Consume(ctx)
	if err != nil {
		log.Fatalf("consume queue: %v", err)
	}

	metrics.ServeStandalone(":" + cfg.WorkerMetricsPort)

	log.Println("worker ready, waiting for messages")
	consumer.Run(ctx, deliveries)
}
