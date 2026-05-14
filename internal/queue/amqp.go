package queue

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
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
		conn.Close()
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
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("queue declare: %w", err)
	}

	log.Info().Str("queue", queueName).Msg("AMQP publisher ready")
	return &AMQPPublisher{conn: conn, ch: ch, queueName: queueName}, nil
}

func (p *AMQPPublisher) Publish(_ context.Context, msg []byte) error {
	return p.ch.Publish(
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
}

func (p *AMQPPublisher) Close() {
	p.ch.Close()
	p.conn.Close()
}
