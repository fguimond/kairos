//go:build integration

package queue

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func testAMQPURL() string {
	if u := os.Getenv("KAIROS_AMQP_URL"); u != "" {
		return u
	}
	return "amqp://guest:guest@localhost:5672/"
}

func uniqueQueue() string {
	return fmt.Sprintf("test.queue.%d", time.Now().UnixNano())
}

// newPublisher creates a publisher with a unique queue, registers cleanup, and
// skips the test if the broker is unreachable.
func newPublisher(t *testing.T) (*AMQPPublisher, string) {
	t.Helper()
	queueName := uniqueQueue()
	p, err := NewAMQPPublisher(testAMQPURL(), queueName)
	if err != nil {
		t.Skipf("RabbitMQ not available: %v", err)
	}
	t.Cleanup(func() {
		p.Close()
		deleteQueue(queueName)
	})
	return p, queueName
}

// deleteQueue removes a test queue; errors are silently ignored.
func deleteQueue(queueName string) {
	conn, err := amqp.Dial(testAMQPURL())
	if err != nil {
		return
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return
	}
	defer ch.Close()
	ch.QueueDelete(queueName, false, false, false)
}

// consumeOne opens a fresh consumer connection and reads one delivery from
// queueName, failing the test if no message arrives within 2 seconds.
func consumeOne(t *testing.T, queueName string) amqp.Delivery {
	t.Helper()
	conn, err := amqp.Dial(testAMQPURL())
	if err != nil {
		t.Fatalf("consumeOne: dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("consumeOne: channel: %v", err)
	}
	t.Cleanup(func() { ch.Close() })

	msgs, err := ch.Consume(queueName, "", false, false, false, false, nil)
	if err != nil {
		t.Fatalf("consumeOne: consume: %v", err)
	}

	select {
	case d := <-msgs:
		return d
	case <-time.After(2 * time.Second):
		t.Fatal("consumeOne: timeout waiting for message")
		panic("unreachable")
	}
}

// --- NewAMQPPublisher ---

func TestNewAMQPPublisher_HappyPath(t *testing.T) {
	p, _ := newPublisher(t)
	if p == nil {
		t.Error("expected non-nil publisher")
	}
}

// TestNewAMQPPublisher_BadURL does not require the broker — it verifies that
// connection errors are wrapped and returned by NewAMQPPublisher.
func TestNewAMQPPublisher_BadURL(t *testing.T) {
	_, err := NewAMQPPublisher("amqp://localhost:1/", "test")
	if err == nil {
		t.Fatal("expected error for unreachable broker, got nil")
	}
}

func TestNewAMQPPublisher_DeclaresQueueDurable(t *testing.T) {
	_, queueName := newPublisher(t)

	// Re-declaring the same queue with durable=false must fail (406 PRECONDITION_FAILED),
	// proving the queue was originally declared durable.
	conn, err := amqp.Dial(testAMQPURL())
	if err != nil {
		t.Fatalf("verification dial: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("verification channel: %v", err)
	}
	defer ch.Close()

	_, err = ch.QueueDeclare(queueName, false, false, false, false, nil)
	if err == nil {
		t.Error("expected PRECONDITION_FAILED when re-declaring durable queue as non-durable, got nil")
	}
}

// --- Publish ---

func TestPublish_BodyArrivesUnmodified(t *testing.T) {
	p, queueName := newPublisher(t)

	body := []byte(`{"schedule_id":"s1","payload":{"key":"val"}}`)
	if err := p.Publish(context.Background(), body); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	d := consumeOne(t, queueName)
	if string(d.Body) != string(body) {
		t.Errorf("body mismatch:\n  want: %s\n  got:  %s", body, d.Body)
	}
}

func TestPublish_ContentTypeIsJSON(t *testing.T) {
	p, queueName := newPublisher(t)

	if err := p.Publish(context.Background(), []byte(`{}`)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	d := consumeOne(t, queueName)
	if d.ContentType != "application/json" {
		t.Errorf("ContentType: want %q, got %q", "application/json", d.ContentType)
	}
}

func TestPublish_DeliveryIsPersistent(t *testing.T) {
	p, queueName := newPublisher(t)

	if err := p.Publish(context.Background(), []byte(`{}`)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	d := consumeOne(t, queueName)
	if d.DeliveryMode != amqp.Persistent {
		t.Errorf("DeliveryMode: want %d (Persistent), got %d", amqp.Persistent, d.DeliveryMode)
	}
}

func TestPublish_MultipleMessagesArriveFIFO(t *testing.T) {
	p, queueName := newPublisher(t)

	messages := [][]byte{
		[]byte(`{"id":"1"}`),
		[]byte(`{"id":"2"}`),
		[]byte(`{"id":"3"}`),
	}
	for _, msg := range messages {
		if err := p.Publish(context.Background(), msg); err != nil {
			t.Fatalf("Publish %s: %v", msg, err)
		}
	}

	// Use a single consumer connection so all messages go to the same receiver
	// and FIFO order is preserved.
	conn, err := amqp.Dial(testAMQPURL())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("channel: %v", err)
	}
	defer ch.Close()

	msgs, err := ch.Consume(queueName, "", false, false, false, false, nil)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}

	for i, want := range messages {
		select {
		case d := <-msgs:
			if string(d.Body) != string(want) {
				t.Errorf("message %d: want %s, got %s", i, want, d.Body)
			}
			d.Ack(false)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for message %d", i)
		}
	}
}

func TestPublish_EmptyBody(t *testing.T) {
	p, queueName := newPublisher(t)

	if err := p.Publish(context.Background(), []byte{}); err != nil {
		t.Fatalf("Publish empty body: %v", err)
	}

	d := consumeOne(t, queueName)
	if len(d.Body) != 0 {
		t.Errorf("expected empty body, got %q", d.Body)
	}
}

func TestPublish_LargePayload(t *testing.T) {
	p, queueName := newPublisher(t)

	// 1 MiB payload — well within RabbitMQ's default 128 MiB frame limit.
	body := make([]byte, 1<<20)
	for i := range body {
		body[i] = 'x'
	}

	if err := p.Publish(context.Background(), body); err != nil {
		t.Fatalf("Publish large payload: %v", err)
	}

	d := consumeOne(t, queueName)
	if len(d.Body) != len(body) {
		t.Errorf("body length: want %d, got %d", len(body), len(d.Body))
	}
}

// --- NewAMQPPublisher edge cases ---

func TestNewAMQPPublisher_EmptyQueueName(t *testing.T) {
	// RabbitMQ permits an empty queue name — the broker assigns a server-generated
	// name. NewAMQPPublisher should succeed and be usable.
	p, err := NewAMQPPublisher(testAMQPURL(), "")
	if err != nil {
		t.Skipf("RabbitMQ not available: %v", err)
	}
	defer p.Close()

	if err := p.Publish(context.Background(), []byte(`{}`)); err != nil {
		t.Errorf("Publish on empty-named queue: %v", err)
	}
}

func TestNewAMQPPublisher_SameQueueNameTwice(t *testing.T) {
	// Declaring the same durable queue twice (idempotent declare) must succeed.
	_, queueName := newPublisher(t)

	p2, err := NewAMQPPublisher(testAMQPURL(), queueName)
	if err != nil {
		t.Fatalf("second NewAMQPPublisher with same queue name: %v", err)
	}
	defer p2.Close()
}

// --- Close ---

func TestClose_PublishFailsAfterClose(t *testing.T) {
	p, _ := newPublisher(t)

	p.Close()

	err := p.Publish(context.Background(), []byte(`{}`))
	if err == nil {
		t.Error("expected error when publishing on a closed publisher, got nil")
	}
}

func TestClose_IsIdempotent(t *testing.T) {
	p, _ := newPublisher(t)

	// Calling Close twice must not panic.
	p.Close()
	p.Close()
}
