package service

import (
	"testing"
	"time"
)

func TestNextRun_ValidCron(t *testing.T) {
	got, err := nextRun("* * * * *", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.After(time.Now().UTC()) {
		t.Error("expected next run to be in the future")
	}
}

func TestNextRun_ValidInterval(t *testing.T) {
	sec := int64(60)
	before := time.Now().UTC()
	got, err := nextRun("", &sec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lower := before.Add(59 * time.Second)
	upper := before.Add(61 * time.Second)
	if got.Before(lower) || got.After(upper) {
		t.Errorf("next run %v outside expected range [%v, %v]", got, lower, upper)
	}
}

func TestNextRun_NoCronNoInterval(t *testing.T) {
	_, err := nextRun("", nil)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestNextRun_InvalidCronExpr(t *testing.T) {
	_, err := nextRun("not-a-cron", nil)
	if err == nil {
		t.Error("expected error for invalid cron expression, got nil")
	}
}

func TestNextRun_ZeroInterval(t *testing.T) {
	sec := int64(0)
	_, err := nextRun("", &sec)
	if err == nil {
		t.Error("expected error for zero interval, got nil")
	}
}

func TestNextRun_NegativeInterval(t *testing.T) {
	sec := int64(-10)
	_, err := nextRun("", &sec)
	if err == nil {
		t.Error("expected error for negative interval, got nil")
	}
}

func TestNextRun_CronTakesPrecedenceOverInterval(t *testing.T) {
	sec := int64(60)
	got, err := nextRun("* * * * *", &sec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.After(time.Now().UTC()) {
		t.Error("expected next run to be in the future")
	}
}
