package leave

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"name/backend/internal/auth"
	"name/backend/internal/httpapi"
	"name/backend/internal/notify"
	"name/backend/internal/workflow"
)

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}

type Request struct {
	ID          string    `json:"id"`
	RequestedBy string    `json:"requested_by"`
	DisplayName string    `json:"display_name,omitempty"`
	LeaveType   string    `json:"leave_type"`
	StartDate   string    `json:"start_date"`
	EndDate     string    `json:"end_date"`
	TotalDays   float64   `json:"total_days"`
	HalfDay     bool      `json:"half_day"`
	Reason      string    `json:"reason"`
	Status      string    `json:"status"`
	WorkflowID  string    `json:"workflow_instance_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type Balance struct {
	ID              string  `json:"id"`
	UserID          string  `json:"user_id"`
	DisplayName     string  `json:"display_name,omitempty"`
	LeaveType       string  `json:"leave_type"`
	Year            int     `json:"year"`
	EntitledDays    float64 `json:"entitled_days"`
	UsedDays        float64 `json:"used_days"`
	CarriedOverDays float64 `json:"carried_over_days"`
	PendingDays     float64 `json:"pending_days"`
	RemainingDays   float64 `json:"remaining_days"`
}

type Policy struct {
	ID              string  `json:"id"`
	LeaveType       string  `json:"leave_type"`
	EntitledDays    float64 `json:"entitled_days"`
	AllowHalfDay    bool    `json:"allow_half_day"`
	RequiresBalance bool    `json:"requires_balance"`
	Active          bool    `json:"active"`
}

type createRequest struct {
	LeaveType string `json:"leave_type"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Reason    string `json:"reason"`
	HalfDay   bool   `json:"half_day"`
}

type decisionRequest struct {
	ID string `json:"id"`
}

func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	filter := r.URL.Query().Get("filter")
	query := `
		SELECT l.id,l.requested_by,u.display_name,l.leave_type,l.start_date::text,l.end_date::text,
		       l.total_days,l.half_day,l.reason,l.status,COALESCE(l.workflow_instance_id::text,''),l.created_at
		FROM leave_requests l JOIN users u ON u.id=l.requested_by
		WHERE l.organization_id=$1`
	args := []any{user.OrganizationID}
	switch filter {
	case "mine":
		query += ` AND l.requested_by=$2`
		args = append(args, user.ID)
	case "pending":
		query += ` AND l.status='pending'`
	}
	query += ` ORDER BY l.created_at DESC LIMIT 500`

	rows, err := h.DB.Query(r.Context(), query, args...)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load leave requests")
		return
	}
	defer rows.Close()
	items := make([]Request, 0)
	for rows.Next() {
		var item Request
		if err := rows.Scan(&item.ID, &item.RequestedBy, &item.DisplayName, &item.LeaveType, &item.StartDate, &item.EndDate,
			&item.TotalDays, &item.HalfDay, &item.Reason, &item.Status, &item.WorkflowID, &item.CreatedAt); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read leave requests")
			return
		}
		items = append(items, item)
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

func (h Handler) Create(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var input createRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.StartDate) == "" || strings.TrimSpace(input.EndDate) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "start_date and end_date are required")
		return
	}
	if input.LeaveType == "" {
		input.LeaveType = "annual"
	}
	days := RequestDays(input.StartDate, input.EndDate, input.HalfDay)
	if days <= 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_dates", "end_date must be on or after start_date")
		return
	}
	if input.HalfDay && days != 0.5 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_half_day", "half-day leave is only allowed for a single calendar day")
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "tx_failed", "could not start leave request")
		return
	}
	defer tx.Rollback(r.Context())

	if err := assertNoOverlap(r.Context(), tx, user.OrganizationID, user.ID, input.StartDate, input.EndDate, ""); err != nil {
		httpapi.WriteError(w, http.StatusConflict, "overlap", err.Error())
		return
	}
	if err := assertBalanceAvailable(r.Context(), tx, user.OrganizationID, user.ID, input.LeaveType, input.StartDate, days); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "insufficient_balance", err.Error())
		return
	}

	var item Request
	err = tx.QueryRow(r.Context(), `
		INSERT INTO leave_requests (organization_id,requested_by,leave_type,start_date,end_date,total_days,half_day,reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id,requested_by,leave_type,start_date::text,end_date::text,total_days,half_day,reason,status,COALESCE(workflow_instance_id::text,''),created_at`,
		user.OrganizationID, user.ID, input.LeaveType, input.StartDate, input.EndDate, days, input.HalfDay, input.Reason,
	).Scan(&item.ID, &item.RequestedBy, &item.LeaveType, &item.StartDate, &item.EndDate, &item.TotalDays, &item.HalfDay, &item.Reason, &item.Status, &item.WorkflowID, &item.CreatedAt)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create leave request")
		return
	}

	_, _ = tx.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'leave.created','leave_request',$3,$4)`,
		user.OrganizationID, user.ID, item.ID, map[string]any{"leave_type": item.LeaveType, "total_days": item.TotalDays})
	if err := tx.Commit(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not save leave request")
		return
	}

	if defID := workflow.FindDefinitionByEntity(r.Context(), h.DB, user.OrganizationID, "leave"); defID != "" {
		title := fmt.Sprintf("Leave: %s (%s → %s)", input.LeaveType, input.StartDate, input.EndDate)
		if instanceID, err := workflow.Start(r.Context(), h.DB, user.OrganizationID, defID, title, "leave", item.ID, nil, user.ID); err == nil {
			_, _ = h.DB.Exec(r.Context(), `UPDATE leave_requests SET workflow_instance_id=$1 WHERE id=$2`, instanceID, item.ID)
			item.WorkflowID = instanceID
		}
	}
	httpapi.WriteJSON(w, http.StatusCreated, item)
}

func (h Handler) Decide(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var input decisionRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.ID == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}
	status := "approved"
	if strings.HasSuffix(r.URL.Path, "/reject") {
		status = "rejected"
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "tx_failed", "could not start leave decision")
		return
	}
	defer tx.Rollback(r.Context())

	var leaveType, startDate, endDate, requestedBy string
	var totalDays float64
	err = tx.QueryRow(r.Context(), `
		UPDATE leave_requests SET status=$1, reviewed_by=$2, updated_at=NOW()
		WHERE id=$3 AND organization_id=$4 AND status='pending'
		RETURNING leave_type, start_date::text, end_date::text, requested_by, total_days`,
		status, user.ID, input.ID, user.OrganizationID,
	).Scan(&leaveType, &startDate, &endDate, &requestedBy, &totalDays)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "update_failed", "leave request could not be updated")
		return
	}

	if status == "approved" {
		if err := applyApprovedBalance(r.Context(), tx, user.OrganizationID, requestedBy, leaveType, startDate, totalDays); err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "insufficient_balance", err.Error())
			return
		}
		markAttendanceLeave(r.Context(), tx, user.OrganizationID, requestedBy, startDate, endDate)
		createLeaveCalendarEvent(r.Context(), tx, user.OrganizationID, requestedBy, leaveType, startDate, endDate)
	}

	_, _ = tx.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,$3,'leave_request',$4,$5)`,
		user.OrganizationID, user.ID, "leave."+status, input.ID, map[string]any{"status": status, "total_days": totalDays})
	if err := tx.Commit(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not save leave decision")
		return
	}

	title := "Leave approved"
	body := fmt.Sprintf("Your %s leave (%s → %s) was approved.", leaveType, startDate, endDate)
	if status == "rejected" {
		title = "Leave rejected"
		body = fmt.Sprintf("Your %s leave (%s → %s) was rejected.", leaveType, startDate, endDate)
	}
	_ = notify.EmitUnlessActor(r.Context(), h.DB, user.OrganizationID, user.ID, requestedBy, "leave."+status, title, body, "leave_request", input.ID)
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"status": status})
}

// Cancel lets the requester cancel a pending request, or a manager cancel pending/approved.
func (h Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var input decisionRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.ID == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}
	canManage := h.Auth.HasPermission(r.Context(), user, "leave.manage")

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "tx_failed", "could not cancel leave")
		return
	}
	defer tx.Rollback(r.Context())

	var leaveType, startDate, endDate, requestedBy, status string
	var totalDays float64
	err = tx.QueryRow(r.Context(), `
		SELECT leave_type, start_date::text, end_date::text, requested_by, status, total_days
		FROM leave_requests WHERE id=$1 AND organization_id=$2 FOR UPDATE`,
		input.ID, user.OrganizationID,
	).Scan(&leaveType, &startDate, &endDate, &requestedBy, &status, &totalDays)
	if err != nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "leave request not found")
		return
	}
	if status != "pending" && status != "approved" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_state", "only pending or approved leave can be cancelled")
		return
	}
	if requestedBy != user.ID && !canManage {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "you can only cancel your own leave")
		return
	}
	if status == "approved" && !canManage {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "approved leave can only be cancelled by a leave manager")
		return
	}

	if _, err := tx.Exec(r.Context(), `UPDATE leave_requests SET status='cancelled', reviewed_by=$1, updated_at=NOW() WHERE id=$2`, user.ID, input.ID); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "update_failed", "could not cancel leave")
		return
	}
	if status == "approved" && requiresBalance(r.Context(), tx, user.OrganizationID, leaveType) {
		year := yearOf(startDate)
		_, _ = tx.Exec(r.Context(), `
			UPDATE leave_balances SET used_days = GREATEST(0, used_days - $1), updated_at=NOW()
			WHERE organization_id=$2 AND user_id=$3 AND leave_type=$4 AND year=$5`,
			totalDays, user.OrganizationID, requestedBy, leaveType, year)
	}
	_, _ = tx.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'leave.cancelled','leave_request',$3,$4)`,
		user.OrganizationID, user.ID, input.ID, map[string]any{"previous_status": status, "total_days": totalDays})
	if err := tx.Commit(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not cancel leave")
		return
	}
	if requestedBy != user.ID {
		_ = notify.Emit(r.Context(), h.DB, user.OrganizationID, requestedBy, "leave.cancelled", "Leave cancelled",
			fmt.Sprintf("Your %s leave (%s → %s) was cancelled.", leaveType, startDate, endDate), "leave_request", input.ID)
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// Balances lists or upserts leave balances.
func (h Handler) Balances(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		year := time.Now().UTC().Year()
		if y := r.URL.Query().Get("year"); y != "" {
			if parsed, err := strconv.Atoi(y); err == nil && parsed >= 2000 && parsed <= 2100 {
				year = parsed
			}
		}
		mine := r.URL.Query().Get("mine") == "1" || !h.Auth.HasPermission(r.Context(), user, "leave.manage")
		query := `
			SELECT b.id, b.user_id, u.display_name, b.leave_type, b.year, b.entitled_days, b.used_days, b.carried_over_days,
			       COALESCE((SELECT SUM(lr.total_days) FROM leave_requests lr
			                 WHERE lr.organization_id=b.organization_id AND lr.requested_by=b.user_id
			                   AND lr.leave_type=b.leave_type AND lr.status='pending'
			                   AND EXTRACT(YEAR FROM lr.start_date)=b.year), 0)
			FROM leave_balances b JOIN users u ON u.id=b.user_id
			WHERE b.organization_id=$1 AND b.year=$2`
		args := []any{user.OrganizationID, year}
		if mine {
			query += ` AND b.user_id=$3`
			args = append(args, user.ID)
		}
		query += ` ORDER BY u.display_name, b.leave_type`
		rows, err := h.DB.Query(r.Context(), query, args...)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load leave balances")
			return
		}
		defer rows.Close()
		items := make([]Balance, 0)
		for rows.Next() {
			var item Balance
			if err := rows.Scan(&item.ID, &item.UserID, &item.DisplayName, &item.LeaveType, &item.Year, &item.EntitledDays, &item.UsedDays, &item.CarriedOverDays, &item.PendingDays); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read leave balances")
				return
			}
			item.RemainingDays = RemainingDays(item.EntitledDays, item.CarriedOverDays, item.UsedDays) - item.PendingDays
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}

	if !h.Auth.HasPermission(r.Context(), user, "leave.manage") {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "leave.manage required to set entitlements")
		return
	}
	var input struct {
		UserID          string  `json:"user_id"`
		LeaveType       string  `json:"leave_type"`
		Year            int     `json:"year"`
		EntitledDays    float64 `json:"entitled_days"`
		CarriedOverDays float64 `json:"carried_over_days"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.LeaveType) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "user_id and leave_type are required")
		return
	}
	if input.Year == 0 {
		input.Year = time.Now().UTC().Year()
	}
	var item Balance
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO leave_balances (organization_id, user_id, leave_type, year, entitled_days, carried_over_days)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (organization_id, user_id, leave_type, year) DO UPDATE SET
			entitled_days=EXCLUDED.entitled_days,
			carried_over_days=EXCLUDED.carried_over_days,
			updated_at=NOW()
		RETURNING id, user_id, leave_type, year, entitled_days, used_days, carried_over_days`,
		user.OrganizationID, input.UserID, input.LeaveType, input.Year, input.EntitledDays, input.CarriedOverDays,
	).Scan(&item.ID, &item.UserID, &item.LeaveType, &item.Year, &item.EntitledDays, &item.UsedDays, &item.CarriedOverDays)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "save_failed", "could not save leave balance")
		return
	}
	item.RemainingDays = RemainingDays(item.EntitledDays, item.CarriedOverDays, item.UsedDays)
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'leave.balance_set','leave_balance',$3,$4)`,
		user.OrganizationID, user.ID, item.ID, map[string]any{"user_id": item.UserID, "leave_type": item.LeaveType, "year": item.Year, "entitled_days": item.EntitledDays})
	httpapi.WriteJSON(w, http.StatusOK, item)
}

// Policies lists or upserts org leave policies (defaults for new entitlements).
func (h Handler) Policies(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT id, leave_type, entitled_days, allow_half_day, requires_balance, active
			FROM leave_policies WHERE organization_id=$1 ORDER BY leave_type`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load leave policies")
			return
		}
		defer rows.Close()
		items := make([]Policy, 0)
		for rows.Next() {
			var item Policy
			if err := rows.Scan(&item.ID, &item.LeaveType, &item.EntitledDays, &item.AllowHalfDay, &item.RequiresBalance, &item.Active); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read leave policies")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	if !h.Auth.HasPermission(r.Context(), user, "leave.manage") {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "leave.manage required")
		return
	}
	var input Policy
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.LeaveType) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "leave_type is required")
		return
	}
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO leave_policies (organization_id, leave_type, entitled_days, allow_half_day, requires_balance, active)
		VALUES ($1,$2,$3,$4,$5,TRUE)
		ON CONFLICT (organization_id, leave_type) DO UPDATE SET
			entitled_days=EXCLUDED.entitled_days,
			allow_half_day=EXCLUDED.allow_half_day,
			requires_balance=EXCLUDED.requires_balance,
			active=TRUE,
			updated_at=NOW()
		RETURNING id, leave_type, entitled_days, allow_half_day, requires_balance, active`,
		user.OrganizationID, input.LeaveType, input.EntitledDays, input.AllowHalfDay, input.RequiresBalance,
	).Scan(&input.ID, &input.LeaveType, &input.EntitledDays, &input.AllowHalfDay, &input.RequiresBalance, &input.Active)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "save_failed", "could not save leave policy")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'leave.policy_set','leave_policy',$3,$4)`,
		user.OrganizationID, user.ID, input.ID, map[string]any{"leave_type": input.LeaveType, "entitled_days": input.EntitledDays})
	httpapi.WriteJSON(w, http.StatusOK, input)
}

// EnsureYearBalances seeds missing balances for a user from org policies.
func (h Handler) EnsureYearBalances(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if !h.Auth.HasPermission(r.Context(), user, "leave.manage") {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "leave.manage required")
		return
	}
	var input struct {
		UserID string `json:"user_id"`
		Year   int    `json:"year"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.UserID == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "user_id is required")
		return
	}
	if input.Year == 0 {
		input.Year = time.Now().UTC().Year()
	}
	tag, err := h.DB.Exec(r.Context(), `
		INSERT INTO leave_balances (organization_id, user_id, leave_type, year, entitled_days)
		SELECT p.organization_id, $2, p.leave_type, $3, p.entitled_days
		FROM leave_policies p
		WHERE p.organization_id=$1 AND p.active=TRUE AND p.requires_balance=TRUE
		ON CONFLICT (organization_id, user_id, leave_type, year) DO NOTHING`,
		user.OrganizationID, input.UserID, input.Year)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "seed_failed", "could not seed leave balances")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"seeded": tag.RowsAffected(), "year": input.Year})
}

func assertNoOverlap(ctx context.Context, tx pgx.Tx, orgID, userID, start, end, excludeID string) error {
	var conflict int
	err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM leave_requests
		WHERE organization_id=$1 AND requested_by=$2 AND status IN ('pending','approved')
		  AND start_date <= $4::date AND end_date >= $3::date
		  AND ($5 = '' OR id <> $5::uuid)`,
		orgID, userID, start, end, excludeID).Scan(&conflict)
	if err != nil {
		return fmt.Errorf("could not check overlapping leave")
	}
	if conflict > 0 {
		return fmt.Errorf("you already have leave covering these dates")
	}
	return nil
}

func assertBalanceAvailable(ctx context.Context, tx pgx.Tx, orgID, userID, leaveType, startDate string, days float64) error {
	if !requiresBalance(ctx, tx, orgID, leaveType) {
		return nil
	}
	year := yearOf(startDate)
	_ = ensureBalanceRow(ctx, tx, orgID, userID, leaveType, year)

	var entitled, used, carried float64
	if err := tx.QueryRow(ctx, `
		SELECT entitled_days, used_days, carried_over_days FROM leave_balances
		WHERE organization_id=$1 AND user_id=$2 AND leave_type=$3 AND year=$4`,
		orgID, userID, leaveType, year).Scan(&entitled, &used, &carried); err != nil {
		return fmt.Errorf("no leave balance configured for %s in %d", leaveType, year)
	}
	var pending float64
	_ = tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(total_days),0) FROM leave_requests
		WHERE organization_id=$1 AND requested_by=$2 AND leave_type=$3 AND status='pending'
		  AND EXTRACT(YEAR FROM start_date)=$4`, orgID, userID, leaveType, year).Scan(&pending)
	available := RemainingDays(entitled, carried, used) - pending
	if days > available {
		return fmt.Errorf("insufficient %s balance: need %.1f days, %.1f available", leaveType, days, available)
	}
	return nil
}

func applyApprovedBalance(ctx context.Context, tx pgx.Tx, orgID, userID, leaveType, startDate string, days float64) error {
	if !requiresBalance(ctx, tx, orgID, leaveType) {
		return nil
	}
	year := yearOf(startDate)
	_ = ensureBalanceRow(ctx, tx, orgID, userID, leaveType, year)
	var entitled, used, carried float64
	if err := tx.QueryRow(ctx, `
		SELECT entitled_days, used_days, carried_over_days FROM leave_balances
		WHERE organization_id=$1 AND user_id=$2 AND leave_type=$3 AND year=$4 FOR UPDATE`,
		orgID, userID, leaveType, year).Scan(&entitled, &used, &carried); err != nil {
		return fmt.Errorf("no leave balance configured for %s in %d", leaveType, year)
	}
	if RemainingDays(entitled, carried, used) < days {
		return fmt.Errorf("insufficient %s balance to approve", leaveType)
	}
	_, err := tx.Exec(ctx, `
		UPDATE leave_balances SET used_days = used_days + $1, updated_at=NOW()
		WHERE organization_id=$2 AND user_id=$3 AND leave_type=$4 AND year=$5`,
		days, orgID, userID, leaveType, year)
	return err
}

func requiresBalance(ctx context.Context, tx pgx.Tx, orgID, leaveType string) bool {
	var required bool
	err := tx.QueryRow(ctx, `
		SELECT requires_balance FROM leave_policies
		WHERE organization_id=$1 AND leave_type=$2 AND active=TRUE`, orgID, leaveType).Scan(&required)
	if err != nil {
		// Default: unpaid skips balance; everything else requires it.
		return leaveType != "unpaid"
	}
	return required
}

func ensureBalanceRow(ctx context.Context, tx pgx.Tx, orgID, userID, leaveType string, year int) error {
	var entitled float64
	_ = tx.QueryRow(ctx, `SELECT entitled_days FROM leave_policies WHERE organization_id=$1 AND leave_type=$2 AND active=TRUE`, orgID, leaveType).Scan(&entitled)
	_, err := tx.Exec(ctx, `
		INSERT INTO leave_balances (organization_id, user_id, leave_type, year, entitled_days)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (organization_id, user_id, leave_type, year) DO NOTHING`,
		orgID, userID, leaveType, year, entitled)
	return err
}

func markAttendanceLeave(ctx context.Context, tx pgx.Tx, orgID, userID, startDate, endDate string) {
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

func createLeaveCalendarEvent(ctx context.Context, tx pgx.Tx, orgID, userID, leaveType, startDate, endDate string) {
	start, err1 := time.Parse("2006-01-02", startDate)
	end, err2 := time.Parse("2006-01-02", endDate)
	if err1 != nil || err2 != nil {
		return
	}
	// All-day: exclusive end is the day after the last leave day.
	startsAt := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	endsAt := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	_, _ = tx.Exec(ctx, `
		INSERT INTO calendar_events (organization_id, created_by, title, description, starts_at, ends_at, all_day, visibility)
		VALUES ($1,$2,$3,$4,$5,$6,TRUE,'organization')`,
		orgID, userID, "Leave · "+leaveType, "Approved leave", startsAt, endsAt)
}

func yearOf(date string) int {
	if t, err := time.Parse("2006-01-02", date); err == nil {
		return t.Year()
	}
	return time.Now().UTC().Year()
}
