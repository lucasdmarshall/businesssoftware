package analytics

import (
	"testing"
	"time"
)

func TestPeriodBounds(t *testing.T) {
	now := time.Date(2026, 8, 20, 15, 0, 0, 0, time.UTC)
	from, to := PeriodBounds(now, "30d")
	if to != "2026-08-20" || from != "2026-07-22" {
		t.Fatalf("30d = %s..%s", from, to)
	}
	from, to = PeriodBounds(now, "ytd")
	if from != "2026-01-01" || to != "2026-08-20" {
		t.Fatalf("ytd = %s..%s", from, to)
	}
}

func TestAverageHours(t *testing.T) {
	if got := AverageHours(16, 2); got != 8 {
		t.Fatalf("got %v", got)
	}
	if got := AverageHours(10, 0); got != 0 {
		t.Fatalf("zero days got %v", got)
	}
}

func TestNextRunAfter(t *testing.T) {
	from := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	if got := NextRunAfter(from, "daily"); !got.Equal(from.AddDate(0, 0, 1)) {
		t.Fatalf("daily = %v", got)
	}
	if got := NextRunAfter(from, "weekly"); !got.Equal(from.AddDate(0, 0, 7)) {
		t.Fatalf("weekly = %v", got)
	}
	if got := NextRunAfter(from, "monthly"); !got.Equal(from.AddDate(0, 1, 0)) {
		t.Fatalf("monthly = %v", got)
	}
}
