package queue

import (
	"context"
	"fmt"

	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
)

// AMQPPublisher publishes trigger messages to a RabbitMQ queue.
type AMQPPublisher struct {
	conn      *amqp.Connection
	ch        *amqp.Channel
	queueName string
}

func NewAMQPPublisher(url, queueName string) (*AMQPPublisher, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("amqp.Dial: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("amqp channel: %w", err)
	}

	// Declare queue as durable so messages survive broker restarts.
	_, err = ch.QueueDeclare(
		queueName,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,
	)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("queue declare: %w", err)
	}

	slog.Info("AMQP publisher ready", "queue", queueName)
	return &AMQPPublisher{conn: conn, ch: ch, queueName: queueName}, nil
}

func (p *AMQPPublisher) Publish(ctx context.Context, msg []byte) error {
	err := p.ch.PublishWithContext(
		ctx,
		"",          // default exchange
		p.queueName, // routing key = queue name
		false,       // mandatory
		false,       // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent, // survive broker restart
			Body:         msg,
		},
	)
	if err != nil {
		return fmt.Errorf("amqp publish: %w", err)
	}
	return nil
}

func (p *AMQPPublisher) Close() {
	if err := p.ch.Close(); err != nil {
		slog.Error("amqp channel close", "error", err)
	}
	if err := p.conn.Close(); err != nil {
		slog.Error("amqp connection close", "error", err)
	}
}
