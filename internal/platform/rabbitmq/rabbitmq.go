package rabbitmq

import (
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Connect retries the initial dial for up to ~30s. A Docker Compose/
// Kubernetes readiness probe on RabbitMQ can report healthy an instant
// before its AMQP listener actually accepts connections (the broker's
// CLI-facing Erlang node and its AMQP protocol listener start up
// somewhat independently) — a bare one-shot amqp.Dial would crash the
// caller's startup on that race, with no supervisor to bring it back.
func Connect(url string) (*amqp.Connection, error) {
	const (
		attempts = 10
		delay    = 3 * time.Second
	)

	var lastErr error
	for i := 0; i < attempts; i++ {
		conn, err := amqp.Dial(url)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		time.Sleep(delay)
	}
	return nil, fmt.Errorf("dial rabbitmq after %d attempts: %w", attempts, lastErr)
}
