package workspace

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"name/backend/internal/auth"
)

// CanEnterDepartment reports whether user may view data scoped to deptID.
// Empty deptID means no department filter (company-wide lists stay allowed).
func CanEnterDepartment(ctx context.Context, db *pgxpool.Pool, authHandler auth.Handler, user auth.SessionUser, deptID string) bool {
	deptID = strings.TrimSpace(deptID)
	if deptID == "" || db == nil {
		return deptID == ""
	}
	var ok bool
	_ = db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM departments WHERE id=$1 AND organization_id=$2
		)`, deptID, user.OrganizationID).Scan(&ok)
	if !ok {
		return false
	}
	if authHandler.HasPermission(ctx, user, "company.departments.access") ||
		authHandler.HasPermission(ctx, user, "organization.manage") {
		return true
	}
	_ = db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_departments WHERE department_id=$1 AND user_id=$2
		)`, deptID, user.ID).Scan(&ok)
	return ok
}

// MemberExistsSQL returns an EXISTS clause binding subjectUserExpr to department_id at argN.
// Example subjectUserExpr: "l.requested_by", "a.user_id", "COALESCE(t.assigned_to,t.created_by)".
func MemberExistsSQL(subjectUserExpr string, argN int) string {
	return `EXISTS (
		SELECT 1 FROM user_departments ud
		WHERE ud.department_id=$` + itoa(argN) + ` AND ud.user_id=` + subjectUserExpr + `
	)`
}

func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
