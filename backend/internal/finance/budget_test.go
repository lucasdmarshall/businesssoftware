package finance

import "testing"

func TestRemainingAndWithinBudget(t *testing.T) {
	if got := Remaining(1000, 250); got != 750 {
		t.Fatalf("Remaining = %v, want 750", got)
	}
	if !WithinBudget(1000, 250, 700) {
		t.Fatal("700 more should fit in 750 remaining")
	}
	if WithinBudget(1000, 250, 800) {
		t.Fatal("800 more should exceed 750 remaining")
	}
	if got := Remaining(100, 150); got != -50 {
		t.Fatalf("overspend Remaining = %v, want -50", got)
	}
}
