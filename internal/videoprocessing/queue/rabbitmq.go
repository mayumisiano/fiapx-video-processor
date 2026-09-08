package queue

import (
	"context"
	"encoding/json"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	QueueName   = "video_processing"
	dlxName     = "video_processing.dlx"
	deadLetterQ = "video_processing.dlq"
)

type Message struct {
	RequestID string `json:"requestId"`
}

func declareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(dlxName, "fanout", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(deadLetterQ, true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.QueueBind(deadLetterQ, "", dlxName, false, nil); err != nil {
		return err
	}
	_, err := ch.QueueDeclare(QueueName, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": dlxName,
	})
	return err
}

type Publisher struct {
	channel *amqp.Channel
}

func NewPublisher(conn *amqp.Connection) (*Publisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	if err := declareTopology(ch); err != nil {
		return nil, err
	}
	return &Publisher{channel: ch}, nil
}

func (p *Publisher) Publish(ctx context.Context, requestID string) error {
	body, err := json.Marshal(Message{RequestID: requestID})
	if err != nil {
		return err
	}
	return p.channel.PublishWithContext(ctx, "", QueueName, false, false, amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
	})
}

type Consumer struct {
	channel *amqp.Channel
}

func NewConsumer(conn *amqp.Connection) (*Consumer, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	if err := declareTopology(ch); err != nil {
		return nil, err
	}
	if err := ch.Qos(1, 0, false); err != nil {
		return nil, err
	}
	return &Consumer{channel: ch}, nil
}

func (c *Consumer) Consume(ctx context.Context) (<-chan amqp.Delivery, error) {
	return c.channel.ConsumeWithContext(ctx, QueueName, "", false, false, false, false, nil)
}
