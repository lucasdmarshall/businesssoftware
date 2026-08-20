package leave

import "time"

// InclusiveLeaveDays returns the number of calendar days in [start, end].
// Invalid or inverted ranges return 0.
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
