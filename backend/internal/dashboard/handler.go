package dashboard

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"name/backend/internal/auth"
	"name/backend/internal/httpapi"
)

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}

type DepartmentStat struct {
	Department string `json:"department"`
	OpenTasks  int    `json:"open_tasks"`
}

type Summary struct {
	Scope               string           `json:"scope"`
	OpenTasks           int              `json:"open_tasks"`
	InProgressTasks     int              `json:"in_progress_tasks"`
	OverdueTasks        int              `json:"overdue_tasks"`
	DoneTasks           int              `json:"done_tasks"`
	ApprovalsWaiting    int              `json:"approvals_waiting"`
	UpcomingEvents      int              `json:"upcoming_events"`
	UnreadNotifications int              `json:"unread_notifications"`
	DepartmentBreakdown []DepartmentStat `json:"department_breakdown"`
}

// scopeCondition returns the SQL predicate that limits tasks to the requested
// scope. It references $1 (organization) and $2 (user).
func scopeCondition(scope string) string {
	switch scope {
	case "own":
		return `(t.created_by=$2 OR t.assigned_to=$2)`
	case "team":
		return `EXISTS (SELECT 1 FROM user_teams cur JOIN user_teams tgt ON tgt.team_id=cur.team_id WHERE cur.user_id=$2 AND tgt.user_id=COALESCE(t.assigned_to,t.created_by))`
	case "department":
		return `EXISTS (SELECT 1 FROM user_departments cur JOIN user_departments tgt ON tgt.department_id=cur.department_id WHERE cur.user_id=$2 AND tgt.user_id=COALESCE(t.assigned_to,t.created_by))`
	default:
		// Organization scope has no per-user filter, but $2 (the user id) is
		// always bound by the caller, so the predicate must still reference it
		// or Postgres rejects the extra bind parameter. This is always true.
		return `$2::uuid IS NOT NULL`
	}
}

// Summary returns scope-aware headline metrics for the signed-in user.
func (h Handler) Summary(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope != "own" && scope != "team" && scope != "department" && scope != "organization" {
		scope = "organization"
	}
	cond := scopeCondition(scope)
	ctx := r.Context()
	summary := Summary{Scope: scope, DepartmentBreakdown: []DepartmentStat{}}

	// Task counts in a single pass over the scoped set.
	err = h.DB.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE t.status <> 'done'),
			COUNT(*) FILTER (WHERE t.status = 'in_progress'),
			COUNT(*) FILTER (WHERE t.status <> 'done' AND t.due_at IS NOT NULL AND t.due_at < NOW()),
			COUNT(*) FILTER (WHERE t.status = 'done')
		FROM tasks t WHERE t.organization_id=$1 AND `+cond,
		user.OrganizationID, user.ID).Scan(&summary.OpenTasks, &summary.InProgressTasks, &summary.OverdueTasks, &summary.DoneTasks)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load task metrics")
		return
	}

	_ = h.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM workflow_instances i
		WHERE i.organization_id=$1 AND i.status='in_review'
		  AND EXISTS (SELECT 1 FROM workflow_steps s WHERE s.definition_id=i.definition_id AND s.step_order=i.current_step_order
		    AND (s.approver_user_id=$2 OR (s.approver_role_code IS NULL AND s.approver_user_id IS NULL)
		         OR EXISTS (SELECT 1 FROM user_roles ur JOIN roles ro ON ro.id=ur.role_id WHERE ur.user_id=$2 AND ro.organization_id=$1 AND ro.code=s.approver_role_code)))
		  AND NOT EXISTS (SELECT 1 FROM workflow_actions a WHERE a.instance_id=i.id AND a.step_order=i.current_step_order AND a.actor_id=$2 AND a.action='approve')`,
		user.OrganizationID, user.ID).Scan(&summary.ApprovalsWaiting)

	_ = h.DB.QueryRow(ctx, `SELECT COUNT(*) FROM calendar_events e WHERE e.organization_id=$1 AND (e.visibility='organization' OR e.created_by=$2) AND e.starts_at BETWEEN NOW() AND NOW() + INTERVAL '7 days'`, user.OrganizationID, user.ID).Scan(&summary.UpcomingEvents)

	_ = h.DB.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND read_at IS NULL`, user.ID).Scan(&summary.UnreadNotifications)

	if scope == "organization" || scope == "department" {
		summary.DepartmentBreakdown = h.departmentBreakdown(ctx, user.OrganizationID)
	}

	httpapi.WriteJSON(w, http.StatusOK, summary)
}

// departmentBreakdown counts open tasks per department by the owner/assignee's
// department membership.
func (h Handler) departmentBreakdown(ctx context.Context, orgID string) []DepartmentStat {
	rows, err := h.DB.Query(ctx, `
		SELECT d.name, COUNT(DISTINCT t.id)
		FROM departments d
		LEFT JOIN user_departments ud ON ud.department_id=d.id
		LEFT JOIN tasks t ON t.organization_id=d.organization_id AND t.status <> 'done' AND COALESCE(t.assigned_to,t.created_by)=ud.user_id
		WHERE d.organization_id=$1
		GROUP BY d.id, d.name ORDER BY d.name`, orgID)
	if err != nil {
		return []DepartmentStat{}
	}
	defer rows.Close()
	stats := make([]DepartmentStat, 0)
	for rows.Next() {
		var stat DepartmentStat
		if err := rows.Scan(&stat.Department, &stat.OpenTasks); err != nil {
			return stats
		}
		stats = append(stats, stat)
	}
	return stats
}
