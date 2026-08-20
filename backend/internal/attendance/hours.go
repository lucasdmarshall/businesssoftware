package attendance

import "time"

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
