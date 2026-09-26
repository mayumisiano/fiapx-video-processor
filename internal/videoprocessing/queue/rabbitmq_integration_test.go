//go:build integration

package queue_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	platformrabbitmq "video-processor/internal/platform/rabbitmq"
	"video-processor/internal/videoprocessing/queue"
)

func TestPublishAndConsume_RoundTrip(t *testing.T) {
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		t.Skip("RABBITMQ_URL not set, skipping integration test")
	}

	conn, err := platformrabbitmq.Connect(url)
	if err != nil {
		t.Fatalf("connect to rabbitmq: %v", err)
	}
	defer conn.Close()

	publisher, err := queue.NewPublisher(conn)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}

	consumer, err := queue.NewConsumer(conn)
	if err != nil {
		t.Fatalf("new consumer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	deliveries, err := consumer.Consume(ctx)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}

	const requestID = "test-request-id-for-roundtrip"
	if err := publisher.Publish(ctx, requestID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case delivery := <-deliveries:
		var msg queue.Message
		if err := json.Unmarshal(delivery.Body, &msg); err != nil {
			t.Fatalf("unmarshal delivery body: %v", err)
		}
		if msg.RequestID != requestID {
			t.Fatalf("delivered requestID = %q, want %q", msg.RequestID, requestID)
		}
		if err := delivery.Ack(false); err != nil {
			t.Fatalf("ack: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for the published message to be delivered")
	}
}
