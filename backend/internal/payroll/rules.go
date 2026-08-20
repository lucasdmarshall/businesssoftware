// Package payroll holds organization payroll rules and hour-summary helpers.
// Payroll confirmation stays server-authoritative; this package only stores
// rules and derives worked/overtime hours from attendance records.
package payroll

import (
	"math"
	"time"
)

// Rule is an organization payroll configuration.
type Rule struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	StandardHoursPerDay float64 `json:"standard_hours_per_day"`
	OvertimeAfterHours  float64 `json:"overtime_after_hours"`
	OvertimeMultiplier  float64 `json:"overtime_multiplier"`
	WeekendMultiplier   float64 `json:"weekend_multiplier"`
	Currency            string  `json:"currency"`
	Active              bool    `json:"active"`
}

// DayHours is one day's computed hours under a rule.
type DayHours struct {
	WorkDate      string  `json:"work_date"`
	WorkedHours   float64 `json:"worked_hours"`
	RegularHours  float64 `json:"regular_hours"`
	OvertimeHours float64 `json:"overtime_hours"`
	Multiplier    float64 `json:"multiplier"`
	IsWeekend     bool    `json:"is_weekend"`
}

// DefaultRule returns the baseline rule used when an organization has none yet.
func DefaultRule() Rule {
	return Rule{
		Name:                "Default",
		StandardHoursPerDay: 8,
		OvertimeAfterHours:  8,
		OvertimeMultiplier:  1.5,
		WeekendMultiplier:   2.0,
		Currency:            "USD",
		Active:              true,
	}
}

// WorkedHours returns the duration between check-in and check-out in hours.
// Incomplete records (missing either stamp) contribute zero hours.
func WorkedHours(checkIn, checkOut *time.Time) float64 {
	if checkIn == nil || checkOut == nil {
		return 0
	}
	if !checkOut.After(*checkIn) {
		return 0
	}
	hours := checkOut.Sub(*checkIn).Hours()
	return round2(hours)
}

// SplitDayHours applies overtime and weekend multipliers to a single day.
func SplitDayHours(workDate string, worked float64, rule Rule) DayHours {
	day := DayHours{WorkDate: workDate, WorkedHours: round2(worked), Multiplier: 1}
	if t, err := time.Parse("2006-01-02", workDate); err == nil {
		weekday := t.Weekday()
		day.IsWeekend = weekday == time.Saturday || weekday == time.Sunday
		if day.IsWeekend {
			day.Multiplier = rule.WeekendMultiplier
		}
	}
	threshold := rule.OvertimeAfterHours
	if threshold <= 0 {
		threshold = rule.StandardHoursPerDay
	}
	if worked <= threshold {
		day.RegularHours = round2(worked)
		return day
	}
	day.RegularHours = round2(threshold)
	day.OvertimeHours = round2(worked - threshold)
	if !day.IsWeekend {
		day.Multiplier = rule.OvertimeMultiplier
	}
	return day
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
