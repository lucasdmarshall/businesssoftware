package leave

import "testing"

func TestInclusiveLeaveDays(t *testing.T) {
	if got := InclusiveLeaveDays("2026-08-20", "2026-08-20"); got != 1 {
		t.Fatalf("same day = %v", got)
	}
	if got := InclusiveLeaveDays("2026-08-20", "2026-08-22"); got != 3 {
		t.Fatalf("3 days = %v", got)
	}
	if got := InclusiveLeaveDays("2026-08-22", "2026-08-20"); got != 0 {
		t.Fatalf("inverted = %v", got)
	}
}

func TestRequestDays(t *testing.T) {
	if got := RequestDays("2026-08-20", "2026-08-20", true); got != 0.5 {
		t.Fatalf("half day = %v", got)
	}
	if got := RequestDays("2026-08-20", "2026-08-21", true); got != 2 {
		t.Fatalf("multi-day ignores half = %v", got)
	}
}

func TestRemainingDays(t *testing.T) {
	if got := RemainingDays(20, 2, 5); got != 17 {
		t.Fatalf("got %v", got)
	}
}
