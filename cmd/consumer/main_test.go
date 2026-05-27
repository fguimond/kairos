package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// --- helpers ---

type mockAck struct {
	acked  bool
	nacked bool
}

func (m *mockAck) Ack(_ uint64, _ bool) error     { m.acked = true; return nil }
func (m *mockAck) Nack(_ uint64, _, _ bool) error { m.nacked = true; return nil }
func (m *mockAck) Reject(_ uint64, _ bool) error  { return nil }

func delivery(ack *mockAck, body []byte) amqp.Delivery {
	return amqp.Delivery{Acknowledger: ack, Body: body}
}

func triggerJSON(t *testing.T, id, name, payload string) []byte {
	t.Helper()
	msg := TriggerMessage{
		ScheduleID:   id,
		ScheduleName: name,
		FiredAt:      time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC),
		Payload:      json.RawMessage(payload),
	}
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal trigger: %v", err)
	}
	return b
}

// mockChannel implements amqpChannel for tests.
type mockChannel struct {
	queueDeclareErr error
	consumeErr      error
	consumeMsgs     chan amqp.Delivery
	closed          bool
}

func (m *mockChannel) QueueDeclare(_ string, _, _, _, _ bool, _ amqp.Table) (amqp.Queue, error) {
	return amqp.Queue{}, m.queueDeclareErr
}

func (m *mockChannel) Consume(_, _ string, _, _, _, _ bool, _ amqp.Table) (<-chan amqp.Delivery, error) {
	if m.consumeErr != nil {
		return nil, m.consumeErr
	}
	return m.consumeMsgs, nil
}

func (m *mockChannel) Close() error { m.closed = true; return nil }

func openConnWith(ch *mockChannel, err error) func(string) (amqpChannel, func(), error) {
	return func(_ string) (amqpChannel, func(), error) {
		if err != nil {
			return nil, nil, err
		}
		return ch, func() { _ = ch.Close() }, nil
	}
}

func closedMsgs() chan amqp.Delivery {
	ch := make(chan amqp.Delivery)
	close(ch)
	return ch
}

// --- run() tests ---

func TestRun_ConnectError(t *testing.T) {
	err := run("amqp://bad", "q", openConnWith(nil, errors.New("dial failed")), make(chan struct{}), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "connect") {
		t.Errorf("expected connect error, got %v", err)
	}
}

func TestRun_QueueDeclareError(t *testing.T) {
	ch := &mockChannel{queueDeclareErr: errors.New("declare failed"), consumeMsgs: closedMsgs()}
	err := run("url", "q", openConnWith(ch, nil), make(chan struct{}), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "queue declare") {
		t.Errorf("expected queue declare error, got %v", err)
	}
	if !ch.closed {
		t.Error("expected channel to be closed after QueueDeclare error")
	}
}

func TestRun_ConsumeError(t *testing.T) {
	ch := &mockChannel{consumeErr: errors.New("consume failed")}
	err := run("url", "q", openConnWith(ch, nil), make(chan struct{}), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "consume") {
		t.Errorf("expected consume error, got %v", err)
	}
	if !ch.closed {
		t.Error("expected channel to be closed after Consume error")
	}
}

func TestRun_HappyPath_ProcessesMessage(t *testing.T) {
	ack := &mockAck{}
	msgs := make(chan amqp.Delivery, 1)
	msgs <- delivery(ack, triggerJSON(t, "id-1", "my-job", `{"k":"v"}`))
	close(msgs)

	ch := &mockChannel{consumeMsgs: msgs}
	var buf bytes.Buffer
	err := run("url", "q", openConnWith(ch, nil), make(chan struct{}), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ack.acked {
		t.Error("expected message to be acked")
	}
	if !strings.Contains(buf.String(), "id-1") {
		t.Errorf("expected output to contain id-1, got: %s", buf.String())
	}
}

func TestRun_CleanupCalledOnSuccess(t *testing.T) {
	ch := &mockChannel{consumeMsgs: closedMsgs()}
	err := run("url", "q", openConnWith(ch, nil), make(chan struct{}), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ch.closed {
		t.Error("expected channel to be closed after run returns")
	}
}

// --- realOpenConn() tests ---

func TestRealOpenConn_BadURL(t *testing.T) {
	_, _, err := realOpenConn("not-a-url")
	if err == nil {
		t.Error("expected error for malformed URL, got nil")
	}
}

func TestRealOpenConn_HappyPath(t *testing.T) {
	ch, cleanup, err := realOpenConn("amqp://guest:guest@localhost:5672/")
	if err != nil {
		t.Skipf("RabbitMQ not available: %v", err)
	}
	defer cleanup()

	_, err = ch.QueueDeclare("test-realOpenConn", false, true, false, false, nil)
	if err != nil {
		t.Errorf("QueueDeclare on open channel failed: %v", err)
	}
}

func TestRealOpenConn_CleanupCloses(t *testing.T) {
	ch, cleanup, err := realOpenConn("amqp://guest:guest@localhost:5672/")
	if err != nil {
		t.Skipf("RabbitMQ not available: %v", err)
	}

	cleanup()

	_, err = ch.QueueDeclare("test-cleanup", false, true, false, false, nil)
	if err == nil {
		t.Error("expected error when using channel after cleanup, got nil")
	}
}

// --- consume() tests ---

func runConsume(msgs chan amqp.Delivery, stop chan struct{}) *bytes.Buffer {
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		consume(msgs, stop, &buf)
		close(done)
	}()
	<-done
	return &buf
}

func TestConsume_ValidMessage_PrintsAndAcks(t *testing.T) {
	ack := &mockAck{}
	msgs := make(chan amqp.Delivery, 1)
	msgs <- delivery(ack, triggerJSON(t, "id-1", "my-job", `{"k":"v"}`))
	close(msgs)

	out := runConsume(msgs, make(chan struct{}))

	if !ack.acked {
		t.Error("expected Ack to be called")
	}
	if ack.nacked {
		t.Error("expected Nack not to be called")
	}
	for _, want := range []string{"id-1", "my-job", `{"k":"v"}`, "2026-05-27T12:00:00Z"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q; got:\n%s", want, out.String())
		}
	}
}

func TestConsume_InvalidJSON_Nacks(t *testing.T) {
	ack := &mockAck{}
	msgs := make(chan amqp.Delivery, 1)
	msgs <- delivery(ack, []byte("not json"))
	close(msgs)

	runConsume(msgs, make(chan struct{}))

	if !ack.nacked {
		t.Error("expected Nack to be called for invalid JSON")
	}
	if ack.acked {
		t.Error("expected Ack not to be called for invalid JSON")
	}
}

func TestConsume_ChannelClosed_Returns(t *testing.T) {
	msgs := make(chan amqp.Delivery)
	close(msgs)
	runConsume(msgs, make(chan struct{}))
}

func TestConsume_StopSignal_Returns(t *testing.T) {
	stop := make(chan struct{})
	close(stop)
	runConsume(make(chan amqp.Delivery), stop)
}

func TestConsume_MultipleMessages_AllProcessed(t *testing.T) {
	acks := []*mockAck{{}, {}, {}}
	msgs := make(chan amqp.Delivery, 3)
	for i, ack := range acks {
		id := string(rune('a' + i))
		msgs <- delivery(ack, triggerJSON(t, id, "job-"+id, `{}`))
	}
	close(msgs)

	out := runConsume(msgs, make(chan struct{}))

	for i, ack := range acks {
		if !ack.acked {
			t.Errorf("delivery %d: expected Ack", i)
		}
	}
	if count := strings.Count(out.String(), "--- trigger fired ---"); count != 3 {
		t.Errorf("expected 3 trigger blocks in output, got %d", count)
	}
}
