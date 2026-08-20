package attendance

import (
	"fmt"
	"time"
)

// HoursWorked returns hours between check-in and check-out, rounded to 2 decimals.
// Missing or inverted times return 0.
func HoursWorked(checkIn, checkOut *time.Time) float64 {
	if checkIn == nil || checkOut == nil {
		return 0
	}
	if !checkOut.After(*checkIn) {
		return 0
	}
	hours := checkOut.Sub(*checkIn).Hours()
	return float64(int(hours*100+0.5)) / 100
}

// ValidStatus reports whether a status is allowed on attendance records.
func ValidStatus(status string) bool {
	switch status {
	case "present", "absent", "leave", "remote":
		return true
	default:
		return false
	}
}

// FormatHMS formats a non-negative duration as hh:mm:ss.
func FormatHMS(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	total := int64(d.Seconds())
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// EarlyLate compares actual check-in against the company expected check-in time
// on the same calendar day (using the check-in's location / UTC wall clock).
// Returns earlyBy and lateBy as hh:mm:ss (the unused side is empty).
func EarlyLate(checkIn *time.Time, expected time.Time) (earlyBy, lateBy string) {
	if checkIn == nil {
		return "", ""
	}
	loc := checkIn.Location()
	y, m, d := checkIn.In(loc).Date()
	eh, em, es := expected.Clock()
	target := time.Date(y, m, d, eh, em, es, 0, loc)
	delta := checkIn.In(loc).Sub(target)
	if delta == 0 {
		return "", ""
	}
	if delta < 0 {
		return FormatHMS(-delta), ""
	}
	return "", FormatHMS(delta)
}

// ParseClock parses "HH:MM" or "HH:MM:SS" into a time-of-day (date ignored).
func ParseClock(value string) (time.Time, error) {
	value = trimSpace(value)
	layouts := []string{"15:04:05", "15:04"}
	var last error
	for _, layout := range layouts {
		t, err := time.Parse(layout, value)
		if err == nil {
			return t, nil
		}
		last = err
	}
	return time.Time{}, last
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
