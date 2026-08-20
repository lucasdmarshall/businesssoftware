package leave

import "time"

// InclusiveLeaveDays returns calendar days in [start, end]. Invalid ranges return 0.
func InclusiveLeaveDays(startDate, endDate string) float64 {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return 0
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return 0
	}
	if end.Before(start) {
		return 0
	}
	return float64(int(end.Sub(start).Hours()/24) + 1)
}

// RequestDays returns the charged day count. A single-day half-day request is 0.5.
func RequestDays(startDate, endDate string, halfDay bool) float64 {
	days := InclusiveLeaveDays(startDate, endDate)
	if days <= 0 {
		return 0
	}
	if halfDay && days == 1 {
		return 0.5
	}
	return days
}

// RemainingDays is entitled + carried − used.
func RemainingDays(entitled, carried, used float64) float64 {
	return entitled + carried - used
}
