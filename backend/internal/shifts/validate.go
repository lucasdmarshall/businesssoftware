package shifts

import (
	"fmt"
	"strings"
	"time"
)

func ValidStatus(status string) bool {
	switch status {
	case "scheduled", "confirmed", "cancelled", "completed":
		return true
	default:
		return false
	}
}

// ParseClock parses HH:MM or HH:MM:SS.
func ParseClock(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"15:04:05", "15:04"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time")
}

// NormalizeClock returns HH:MM:SS for storage/display.
func NormalizeClock(value string) (string, error) {
	t, err := ParseClock(value)
	if err != nil {
		return "", err
	}
	return t.Format("15:04:05"), nil
}

// DurationHMS returns hh:mm:ss between start and end clocks on the same day.
func DurationHMS(startsAt, endsAt string) string {
	start, err1 := ParseClock(startsAt)
	end, err2 := ParseClock(endsAt)
	if err1 != nil || err2 != nil || !end.After(start) {
		return ""
	}
	total := int64(end.Sub(start).Seconds())
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// ClocksOverlap reports whether [aStart,aEnd) overlaps [bStart,bEnd).
func ClocksOverlap(aStart, aEnd, bStart, bEnd string) bool {
	as, err1 := ParseClock(aStart)
	ae, err2 := ParseClock(aEnd)
	bs, err3 := ParseClock(bStart)
	be, err4 := ParseClock(bEnd)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return false
	}
	return as.Before(be) && bs.Before(ae)
}
