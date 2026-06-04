package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type TriggerMessage struct {
	ScheduleID   string          `json:"schedule_id"`
	ScheduleName string          `json:"schedule_name"`
	FiredAt      time.Time       `json:"fired_at"`
	Payload      json.RawMessage `json:"payload"`
}

// amqpChannel is the minimal AMQP channel surface used by run.
type amqpChannel interface {
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)
	Close() error
}

// run opens the channel, declares the queue, and runs the consume loop.
// openConn is injected so tests can supply a mock without a real broker.
func run(amqpURL, queueName string, openConn func(string) (amqpChannel, func(), error), stop <-chan struct{}, w io.Writer) error {
	ch, cleanup, err := openConn(amqpURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer cleanup()

	if _, err = ch.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("queue declare: %w", err)
	}

	msgs, err := ch.Consume(queueName, "kairos-sample-consumer", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	slog.Info("consumer ready", "queue", queueName)
	consume(msgs, stop, w)
	return nil
}

// consume reads deliveries until msgs is closed or stop is closed.
func consume(msgs <-chan amqp.Delivery, stop <-chan struct{}, w io.Writer) {
	for {
		select {
		case d, ok := <-msgs:
			if !ok {
				slog.Info("channel closed, exiting")
				return
			}
			var msg TriggerMessage
			if err := json.Unmarshal(d.Body, &msg); err != nil {
				slog.Warn("failed to decode message", "err", err, "body", string(d.Body))
				if err := d.Nack(false, false); err != nil {
					slog.Error("nack failed", "error", err)
				}
				continue
			}
			if _, err := fmt.Fprintf(w, "--- trigger fired ---\n  schedule_id:   %s\n  schedule_name: %s\n  fired_at:      %s\n  payload:       %s\n",
				msg.ScheduleID,
				msg.ScheduleName,
				msg.FiredAt.Format(time.RFC3339),
				string(msg.Payload),
			); err != nil {
				slog.Error("write failed", "error", err)
			}
			if err := d.Ack(false); err != nil {
				slog.Error("ack failed", "error", err)
			}
		case <-stop:
			slog.Info("shutting down consumer")
			return
		}
	}
}

func realOpenConn(url string) (amqpChannel, func(), error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, nil, fmt.Errorf("amqp dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("amqp channel: %w", err)
	}
	return ch, func() {
		_ = ch.Close()
		_ = conn.Close()
	}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	amqpURL := getEnv("KAIROS_AMQP_URL", "amqp://guest:guest@localhost:5672/")
	queueName := getEnv("KAIROS_QUEUE_NAME", "kairos.triggers")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	stop := make(chan struct{})
	go func() {
		<-sig
		close(stop)
	}()

	if err := run(amqpURL, queueName, realOpenConn, stop, os.Stdout); err != nil {
		slog.Error("consumer error", "err", err)
		os.Exit(1)
	}
}
