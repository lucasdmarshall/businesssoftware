package calendar

import (
	"testing"
	"time"
)

func at(hour int) time.Time {
	return time.Date(2026, 8, 20, hour, 0, 0, 0, time.UTC)
}

func TestOverlaps(t *testing.T) {
	cases := []struct {
		name         string
		aStart, aEnd time.Time
		bStart, bEnd time.Time
		want         bool
	}{
		{"identical", at(9), at(10), at(9), at(10), true},
		{"partial", at(9), at(11), at(10), at(12), true},
		{"contained", at(9), at(12), at(10), at(11), true},
		{"adjacent-after", at(9), at(10), at(10), at(11), false},
		{"adjacent-before", at(10), at(11), at(9), at(10), false},
		{"disjoint", at(9), at(10), at(14), at(15), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Overlaps(tc.aStart, tc.aEnd, tc.bStart, tc.bEnd); got != tc.want {
				t.Fatalf("Overlaps = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOverlapsIsSymmetric(t *testing.T) {
	if Overlaps(at(9), at(11), at(10), at(12)) != Overlaps(at(10), at(12), at(9), at(11)) {
		t.Fatal("Overlaps should be symmetric in its two intervals")
	}
}
