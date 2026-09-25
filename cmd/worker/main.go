package main

import (
	"context"
	"log"

	identitypostgres "video-processor/internal/identity/postgres"
	notificationsmtp "video-processor/internal/notification/smtp"
	"video-processor/internal/platform/config"
	platformpostgres "video-processor/internal/platform/postgres"
	platformrabbitmq "video-processor/internal/platform/rabbitmq"
	videoapplication "video-processor/internal/videoprocessing/application"
	videoffmpeg "video-processor/internal/videoprocessing/ffmpeg"
	videopostgres "video-processor/internal/videoprocessing/postgres"
	videoqueue "video-processor/internal/videoprocessing/queue"
	videostorage "video-processor/internal/videoprocessing/storage"
	videoworker "video-processor/internal/videoprocessing/worker"
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

	repo := videopostgres.NewRepository(pool)
	userRepo := identitypostgres.NewRepository(pool)
	mailer := notificationsmtp.NewMailer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPFrom)
	extractor := videoffmpeg.NewExtractor()

	processor := videoapplication.NewProcessor(repo, storageClient, extractor, userRepo, mailer)
	consumer := videoworker.NewConsumer(processor)

	deliveries, err := queueConsumer.Consume(ctx)
	if err != nil {
		log.Fatalf("consume queue: %v", err)
	}

	log.Println("worker ready, waiting for messages")
	consumer.Run(ctx, deliveries)
}
