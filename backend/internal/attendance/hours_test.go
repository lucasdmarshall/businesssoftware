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
