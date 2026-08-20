package payroll

import (
	"testing"
	"time"
)

func TestWorkedHours(t *testing.T) {
	in := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	out := time.Date(2026, 8, 20, 17, 30, 0, 0, time.UTC)
	if got := WorkedHours(&in, &out); got != 8.5 {
		t.Fatalf("WorkedHours = %v, want 8.5", got)
	}
	if got := WorkedHours(&in, nil); got != 0 {
		t.Fatalf("incomplete checkout should be 0, got %v", got)
	}
	if got := WorkedHours(&out, &in); got != 0 {
		t.Fatalf("reversed stamps should be 0, got %v", got)
	}
}

func TestSplitDayHoursWeekdayOvertime(t *testing.T) {
	rule := DefaultRule()
	day := SplitDayHours("2026-08-20", 10, rule) // Thursday
	if day.IsWeekend {
		t.Fatal("Thursday should not be weekend")
	}
	if day.RegularHours != 8 || day.OvertimeHours != 2 {
		t.Fatalf("regular/overtime = %v/%v, want 8/2", day.RegularHours, day.OvertimeHours)
	}
	if day.Multiplier != 1.5 {
		t.Fatalf("multiplier = %v, want 1.5", day.Multiplier)
	}
}

func TestSplitDayHoursWeekend(t *testing.T) {
	rule := DefaultRule()
	day := SplitDayHours("2026-08-22", 6, rule) // Saturday
	if !day.IsWeekend {
		t.Fatal("Saturday should be weekend")
	}
	if day.RegularHours != 6 || day.OvertimeHours != 0 {
		t.Fatalf("regular/overtime = %v/%v, want 6/0", day.RegularHours, day.OvertimeHours)
	}
	if day.Multiplier != 2.0 {
		t.Fatalf("multiplier = %v, want 2.0", day.Multiplier)
	}
}
