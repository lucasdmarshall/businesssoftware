package shifts

import "testing"

func TestDurationHMS(t *testing.T) {
	if got := DurationHMS("09:00", "17:30:00"); got != "08:30:00" {
		t.Fatalf("got %s", got)
	}
	if got := DurationHMS("17:00", "09:00"); got != "" {
		t.Fatalf("inverted got %s", got)
	}
}

func TestClocksOverlap(t *testing.T) {
	if !ClocksOverlap("09:00", "12:00", "11:00", "13:00") {
		t.Fatal("expected overlap")
	}
	if ClocksOverlap("09:00", "12:00", "12:00", "14:00") {
		t.Fatal("touching ends should not overlap")
	}
	if !ValidStatus("confirmed") || ValidStatus("draft") {
		t.Fatal("status validation")
	}
}
