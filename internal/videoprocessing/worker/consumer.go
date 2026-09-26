package worker

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"video-processor/internal/platform/metrics"
	"video-processor/internal/videoprocessing/application"
	"video-processor/internal/videoprocessing/queue"
)

// Consumer is the worker's driving adapter: it translates RabbitMQ
// deliveries into calls to application.Processor and acks/nacks based on
// the outcome. It holds no business logic of its own.
type Consumer struct {
	processor *application.Processor
}

func NewConsumer(processor *application.Processor) *Consumer {
	return &Consumer{processor: processor}
}

// Run consumes deliveries until the channel closes (e.g. connection loss).
func (c *Consumer) Run(ctx context.Context, deliveries <-chan amqp.Delivery) {
	for delivery := range deliveries {
		var msg queue.Message
		if err := json.Unmarshal(delivery.Body, &msg); err != nil {
			log.Printf("invalid message payload: %v", err)
			_ = delivery.Nack(false, false)
			continue
		}

		start := time.Now()
		status, err := c.processor.Process(ctx, msg.RequestID)
		metrics.ProcessingDuration.Observe(time.Since(start).Seconds())

		if err != nil {
			log.Printf("processing request %s failed: %v", msg.RequestID, err)
			metrics.VideosProcessedTotal.WithLabelValues("infra_error").Inc()
			_ = delivery.Nack(false, false)
			continue
		}

		metrics.VideosProcessedTotal.WithLabelValues(strings.ToLower(string(status))).Inc()
		_ = delivery.Ack(false)
	}
}
