package sync

import "fmt"

// OfflineCapable reports whether a device may queue this entity/action while
// unreachable. Keep aligned with docs/OFFLINE_RULES.md and the frontend harness.
func OfflineCapable(entity, action string) bool {
	switch entity {
	case "task":
		return action == "create" || action == "update" || action == "delete"
	case "attendance":
		return action == "check_in" || action == "check_out"
	case "leave", "shift", "calendar":
		return action == "create"
	default:
		return false
	}
}

// ServerAuthoritative reports actions that must never be applied from the outbox.
func ServerAuthoritative(entity, action string) bool {
	switch entity {
	case "leave":
		return action == "approve" || action == "reject"
	case "workflow":
		return action == "approve" || action == "reject" || action == "delegate"
	case "finance":
		return action == "approve" || action == "pay" || action == "confirm_payroll"
	case "rbac", "permission", "role", "auth", "mfa", "password":
		return true
	case "user":
		return action == "offboard" || action == "delete"
	default:
		return !OfflineCapable(entity, action) && action != "read"
	}
}

// AttendanceColumn returns the attendance_records column for a check-in/out op.
func AttendanceColumn(action string) (string, error) {
	switch action {
	case "check_in":
		return "check_in_at", nil
	case "check_out":
		return "check_out_at", nil
	default:
		return "", fmt.Errorf("unsupported attendance operation")
	}
}

// TaskCreateConflict is true when an INSERT ... ON CONFLICT DO NOTHING touched
// zero rows — the id already exists on the server and needs human review.
func TaskCreateConflict(rowsAffected int64) bool {
	return rowsAffected == 0
}

// ClassifyPushOutcome maps a single operation's processing result onto the
// push response buckets the client understands.
func ClassifyPushOutcome(duplicate, conflict, rejected bool) string {
	switch {
	case conflict:
		return "conflict"
	case duplicate:
		return "duplicate"
	case rejected:
		return "rejected"
	default:
		return "accepted"
	}
}
