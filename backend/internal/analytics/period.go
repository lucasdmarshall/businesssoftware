package analytics

import (
	"math"
	"time"
)

// PeriodBounds returns inclusive UTC date strings for a named window.
func PeriodBounds(now time.Time, period string) (from, to string) {
	now = now.UTC()
	to = now.Format("2006-01-02")
	switch period {
	case "7d":
		from = now.AddDate(0, 0, -6).Format("2006-01-02")
	case "90d":
		from = now.AddDate(0, 0, -89).Format("2006-01-02")
	case "ytd":
		from = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	default: // 30d
		from = now.AddDate(0, 0, -29).Format("2006-01-02")
	}
	return from, to
}

// AverageHours returns mean worked hours rounded to two decimals.
func AverageHours(totalHours float64, days int) float64 {
	if days <= 0 {
		return 0
	}
	return math.Round((totalHours/float64(days))*100) / 100
}

// NextRunAfter advances a schedule cadence from the previous due time.
func NextRunAfter(from time.Time, cadence string) time.Time {
	from = from.UTC()
	switch cadence {
	case "weekly":
		return from.AddDate(0, 0, 7)
	case "monthly":
		return from.AddDate(0, 1, 0)
	default:
		return from.AddDate(0, 0, 1)
	}
}
