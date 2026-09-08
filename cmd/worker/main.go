package main

import (
	"context"
	"encoding/json"
	"log"

	"video-processor/internal/platform/config"
	platformpostgres "video-processor/internal/platform/postgres"
	platformrabbitmq "video-processor/internal/platform/rabbitmq"
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

	consumer, err := videoqueue.NewConsumer(rabbitConn)
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

	repo := videopostgres.NewRepository(pool)
	processor := videoworker.NewProcessor(repo, storageClient)

	deliveries, err := consumer.Consume(ctx)
	if err != nil {
		log.Fatalf("consume queue: %v", err)
	}

	log.Println("worker ready, waiting for messages")
	for delivery := range deliveries {
		var msg videoqueue.Message
		if err := json.Unmarshal(delivery.Body, &msg); err != nil {
			log.Printf("invalid message payload: %v", err)
			_ = delivery.Nack(false, false)
			continue
		}

		if err := processor.Process(ctx, msg.RequestID); err != nil {
			log.Printf("processing request %s failed: %v", msg.RequestID, err)
			_ = delivery.Nack(false, false)
			continue
		}

		_ = delivery.Ack(false)
	}
}
