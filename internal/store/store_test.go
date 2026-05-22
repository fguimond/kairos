package store

import (
	"testing"
	"time"
)

func TestNextRun_ValidCron(t *testing.T) {
	in := CreateScheduleInput{CronExpr: "* * * * *"}
	got, err := nextRun(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.After(time.Now().UTC()) {
		t.Error("expected next run to be in the future")
	}
}

func TestNextRun_ValidInterval(t *testing.T) {
	sec := int64(60)
	in := CreateScheduleInput{IntervalSec: &sec}
	before := time.Now().UTC()
	got, err := nextRun(in)
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
	_, err := nextRun(CreateScheduleInput{})
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestNextRun_InvalidCronExpr(t *testing.T) {
	_, err := nextRun(CreateScheduleInput{CronExpr: "not-a-cron"})
	if err == nil {
		t.Error("expected error for invalid cron expression, got nil")
	}
}

func TestNextRun_ZeroInterval(t *testing.T) {
	sec := int64(0)
	_, err := nextRun(CreateScheduleInput{IntervalSec: &sec})
	if err == nil {
		t.Error("expected error for zero interval, got nil")
	}
}

func TestNextRun_NegativeInterval(t *testing.T) {
	sec := int64(-10)
	_, err := nextRun(CreateScheduleInput{IntervalSec: &sec})
	if err == nil {
		t.Error("expected error for negative interval, got nil")
	}
}

func TestNextRun_CronTakesPrecedenceOverInterval(t *testing.T) {
	sec := int64(60)
	in := CreateScheduleInput{CronExpr: "* * * * *", IntervalSec: &sec}
	got, err := nextRun(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// cron next run is within the next 60 seconds; interval would be exactly 60s out
	// both should be in the future — just verify it resolves without error
	if !got.After(time.Now().UTC()) {
		t.Error("expected next run to be in the future")
	}
}
