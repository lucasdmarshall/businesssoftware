package attendance

import (
	"testing"
	"time"
)

func TestHoursWorked(t *testing.T) {
	in := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	out := time.Date(2026, 8, 20, 17, 30, 0, 0, time.UTC)
	if got := HoursWorked(&in, &out); got != 8.5 {
		t.Fatalf("got %v", got)
	}
	if got := HoursWorked(&in, nil); got != 0 {
		t.Fatalf("nil out = %v", got)
	}
	if got := HoursWorked(&out, &in); got != 0 {
		t.Fatalf("inverted = %v", got)
	}
}

func TestValidStatus(t *testing.T) {
	if !ValidStatus("remote") || ValidStatus("vacation") {
		t.Fatal("status validation failed")
	}
}

func TestEarlyLate(t *testing.T) {
	expected, err := ParseClock("09:00:00")
	if err != nil {
		t.Fatal(err)
	}
	early := time.Date(2026, 8, 20, 8, 45, 30, 0, time.UTC)
	e, l := EarlyLate(&early, expected)
	if e != "00:14:30" || l != "" {
		t.Fatalf("early got %q / %q", e, l)
	}
	late := time.Date(2026, 8, 20, 9, 5, 1, 0, time.UTC)
	e, l = EarlyLate(&late, expected)
	if e != "" || l != "00:05:01" {
		t.Fatalf("late got %q / %q", e, l)
	}
	onTime := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	e, l = EarlyLate(&onTime, expected)
	if e != "" || l != "" {
		t.Fatalf("on time got %q / %q", e, l)
	}
}

func TestFormatHMS(t *testing.T) {
	if got := FormatHMS(time.Hour + 2*time.Minute + 3*time.Second); got != "01:02:03" {
		t.Fatalf("got %s", got)
	}
}
