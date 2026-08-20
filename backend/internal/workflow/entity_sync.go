package workflow

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// syncLinkedEntity updates module records when a workflow reaches a terminal
// approval state. Leave gets full production side effects (balance, attendance,
// calendar). Expenses stay submitted until paid via the Finance module.
func syncLinkedEntity(ctx context.Context, tx pgx.Tx, instanceID, status, actorID string) error {
	var orgID, entityType string
	var entityID *string
	if err := tx.QueryRow(ctx, `SELECT organization_id, entity_type, entity_id::text FROM workflow_instances WHERE id=$1`, instanceID).Scan(&orgID, &entityType, &entityID); err != nil {
		return err
	}
	if entityID == nil || *entityID == "" {
		return nil
	}
	switch entityType {
	case "leave":
		if status != "approved" && status != "rejected" {
			return nil
		}
		var leaveType, startDate, endDate, requestedBy string
		var totalDays float64
		err := tx.QueryRow(ctx, `
			UPDATE leave_requests SET status=$1, reviewed_by=$2, updated_at=NOW()
			WHERE id=$3 AND organization_id=$4 AND status='pending'
			RETURNING leave_type, start_date::text, end_date::text, requested_by, total_days`,
			status, actorID, *entityID, orgID,
		).Scan(&leaveType, &startDate, &endDate, &requestedBy, &totalDays)
		if err != nil {
			// Already decided or missing — ignore so workflow can complete.
			return nil
		}
		if status == "approved" {
			year := yearOf(startDate)
			_, _ = tx.Exec(ctx, `
				INSERT INTO leave_balances (organization_id, user_id, leave_type, year, entitled_days)
				SELECT $1,$2,$3,$4,COALESCE((SELECT entitled_days FROM leave_policies WHERE organization_id=$1 AND leave_type=$3 AND active=TRUE), 0)
				ON CONFLICT (organization_id, user_id, leave_type, year) DO NOTHING`,
				orgID, requestedBy, leaveType, year)
			_, _ = tx.Exec(ctx, `
				UPDATE leave_balances SET used_days = used_days + $1, updated_at=NOW()
				WHERE organization_id=$2 AND user_id=$3 AND leave_type=$4 AND year=$5`,
				totalDays, orgID, requestedBy, leaveType, year)
			markAttendanceLeaveDays(ctx, tx, orgID, requestedBy, startDate, endDate)
			createApprovedLeaveEvent(ctx, tx, orgID, requestedBy, leaveType, startDate, endDate)
		}
		return nil
	default:
		return nil
	}
}

func yearOf(date string) int {
	if t, err := time.Parse("2006-01-02", date); err == nil {
		return t.Year()
	}
	return time.Now().UTC().Year()
}

func markAttendanceLeaveDays(ctx context.Context, tx pgx.Tx, orgID, userID, startDate, endDate string) {
	start, err1 := time.Parse("2006-01-02", startDate)
	end, err2 := time.Parse("2006-01-02", endDate)
	if err1 != nil || err2 != nil {
		return
	}
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		_, _ = tx.Exec(ctx, `
			INSERT INTO attendance_records (organization_id, user_id, work_date, status, note)
			VALUES ($1,$2,$3,'leave','Approved leave')
			ON CONFLICT (organization_id, user_id, work_date) DO UPDATE SET
				status='leave',
				note=CASE WHEN attendance_records.note='' THEN 'Approved leave' ELSE attendance_records.note END,
				updated_at=NOW()`,
			orgID, userID, d.Format("2006-01-02"))
	}
}

func createApprovedLeaveEvent(ctx context.Context, tx pgx.Tx, orgID, userID, leaveType, startDate, endDate string) {
	start, err1 := time.Parse("2006-01-02", startDate)
	end, err2 := time.Parse("2006-01-02", endDate)
	if err1 != nil || err2 != nil {
		return
	}
	startsAt := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	endsAt := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	_, _ = tx.Exec(ctx, `
		INSERT INTO calendar_events (organization_id, created_by, title, description, starts_at, ends_at, all_day, visibility)
		VALUES ($1,$2,$3,$4,$5,$6,TRUE,'organization')`,
		orgID, userID, "Leave · "+leaveType, "Approved leave", startsAt, endsAt)
}
