package workflow

import "time"

// DueAtFromSLA returns now+slaHours, or nil when no SLA is configured.
func DueAtFromSLA(now time.Time, slaHours *int) *time.Time {
	if slaHours == nil || *slaHours <= 0 {
		return nil
	}
	due := now.UTC().Add(time.Duration(*slaHours) * time.Hour)
	return &due
}

// ReminderKind classifies a deadline relative to now.
// Returns "" when no reminder should be sent.
func ReminderKind(now, dueAt time.Time, lastReminded *time.Time, upcomingWindow, remindEvery time.Duration) string {
	if dueAt.IsZero() {
		return ""
	}
	if lastReminded != nil && now.Sub(*lastReminded) < remindEvery {
		return ""
	}
	if !now.Before(dueAt) {
		return "overdue"
	}
	if dueAt.Sub(now) <= upcomingWindow {
		return "upcoming"
	}
	return ""
}
