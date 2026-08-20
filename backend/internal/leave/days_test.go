package leave

import "testing"

func TestInclusiveLeaveDays(t *testing.T) {
	if got := InclusiveLeaveDays("2026-08-20", "2026-08-22"); got != 3 {
		t.Fatalf("InclusiveLeaveDays = %v, want 3", got)
	}
	if got := InclusiveLeaveDays("2026-08-20", "2026-08-20"); got != 1 {
		t.Fatalf("same-day leave = %v, want 1", got)
	}
	if got := InclusiveLeaveDays("2026-08-22", "2026-08-20"); got != 0 {
		t.Fatalf("inverted range = %v, want 0", got)
	}
}
