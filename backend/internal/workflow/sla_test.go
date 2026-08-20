package workflow

import (
	"testing"
	"time"
)

func TestDueAtFromSLA(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	if DueAtFromSLA(now, nil) != nil {
		t.Fatal("nil SLA should produce nil due date")
	}
	zero := 0
	if DueAtFromSLA(now, &zero) != nil {
		t.Fatal("zero SLA should produce nil due date")
	}
	hours := 24
	due := DueAtFromSLA(now, &hours)
	if due == nil || !due.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("due = %v, want %v", due, now.Add(24*time.Hour))
	}
}

func TestReminderKind(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	upcoming := 24 * time.Hour
	every := 12 * time.Hour

	if got := ReminderKind(now, time.Time{}, nil, upcoming, every); got != "" {
		t.Fatalf("zero due = %q, want empty", got)
	}
	if got := ReminderKind(now, now.Add(48*time.Hour), nil, upcoming, every); got != "" {
		t.Fatalf("far due = %q, want empty", got)
	}
	if got := ReminderKind(now, now.Add(6*time.Hour), nil, upcoming, every); got != "upcoming" {
		t.Fatalf("near due = %q, want upcoming", got)
	}
	if got := ReminderKind(now, now.Add(-1*time.Hour), nil, upcoming, every); got != "overdue" {
		t.Fatalf("past due = %q, want overdue", got)
	}
	recent := now.Add(-1 * time.Hour)
	if got := ReminderKind(now, now.Add(-1*time.Hour), &recent, upcoming, every); got != "" {
		t.Fatalf("recently reminded = %q, want empty", got)
	}
}
