package leave

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"name/backend/internal/auth"
	"name/backend/internal/httpapi"
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
	Reason      string    `json:"reason"`
	Status      string    `json:"status"`
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
	RemainingDays   float64 `json:"remaining_days"`
}

type createRequest struct {
	LeaveType string `json:"leave_type"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Reason    string `json:"reason"`
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
	rows, err := h.DB.Query(r.Context(), `SELECT l.id,l.requested_by,u.display_name,l.leave_type,l.start_date::text,l.end_date::text,l.reason,l.status,l.created_at FROM leave_requests l JOIN users u ON u.id=l.requested_by WHERE l.organization_id=$1 ORDER BY l.created_at DESC LIMIT 500`, user.OrganizationID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load leave requests")
		return
	}
	defer rows.Close()
	items := make([]Request, 0)
	for rows.Next() {
		var item Request
		if err := rows.Scan(&item.ID, &item.RequestedBy, &item.DisplayName, &item.LeaveType, &item.StartDate, &item.EndDate, &item.Reason, &item.Status, &item.CreatedAt); err != nil {
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
	var item Request
	err = h.DB.QueryRow(r.Context(), `INSERT INTO leave_requests (organization_id,requested_by,leave_type,start_date,end_date,reason) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id,requested_by,leave_type,start_date::text,end_date::text,reason,status,created_at`, user.OrganizationID, user.ID, input.LeaveType, input.StartDate, input.EndDate, input.Reason).Scan(&item.ID, &item.RequestedBy, &item.LeaveType, &item.StartDate, &item.EndDate, &item.Reason, &item.Status, &item.CreatedAt)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create leave request")
		return
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
	err = tx.QueryRow(r.Context(), `
		UPDATE leave_requests SET status=$1, reviewed_by=$2, updated_at=NOW()
		WHERE id=$3 AND organization_id=$4 AND status='pending'
		RETURNING leave_type, start_date::text, end_date::text, requested_by`,
		status, user.ID, input.ID, user.OrganizationID,
	).Scan(&leaveType, &startDate, &endDate, &requestedBy)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "update_failed", "leave request could not be updated")
		return
	}

	if status == "approved" && leaveType != "unpaid" {
		days := InclusiveLeaveDays(startDate, endDate)
		year := time.Now().UTC().Year()
		if t, err := time.Parse("2006-01-02", startDate); err == nil {
			year = t.Year()
		}
		_, err = tx.Exec(r.Context(), `
			INSERT INTO leave_balances (organization_id, user_id, leave_type, year, entitled_days, used_days)
			VALUES ($1,$2,$3,$4,0,$5)
			ON CONFLICT (organization_id, user_id, leave_type, year)
			DO UPDATE SET used_days = leave_balances.used_days + EXCLUDED.used_days, updated_at=NOW()`,
			user.OrganizationID, requestedBy, leaveType, year, days)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "balance_failed", "could not update leave balance")
			return
		}
	}

	_, _ = tx.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,$3,'leave_request',$4,$5)`, user.OrganizationID, user.ID, "leave."+status, input.ID, map[string]any{"status": status})
	if err := tx.Commit(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not save leave decision")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": status})
}

// Balances lists leave balances for the organization (managers) or the caller.
func (h Handler) Balances(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		year := time.Now().UTC().Year()
		rows, err := h.DB.Query(r.Context(), `
			SELECT b.id, b.user_id, u.display_name, b.leave_type, b.year, b.entitled_days, b.used_days, b.carried_over_days
			FROM leave_balances b JOIN users u ON u.id=b.user_id
			WHERE b.organization_id=$1 AND b.year=$2
			ORDER BY u.display_name, b.leave_type`, user.OrganizationID, year)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load leave balances")
			return
		}
		defer rows.Close()
		items := make([]Balance, 0)
		for rows.Next() {
			var item Balance
			if err := rows.Scan(&item.ID, &item.UserID, &item.DisplayName, &item.LeaveType, &item.Year, &item.EntitledDays, &item.UsedDays, &item.CarriedOverDays); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read leave balances")
				return
			}
			item.RemainingDays = item.EntitledDays + item.CarriedOverDays - item.UsedDays
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
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
	item.RemainingDays = item.EntitledDays + item.CarriedOverDays - item.UsedDays
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'leave.balance_set','leave_balance',$3,$4)`,
		user.OrganizationID, user.ID, item.ID, map[string]any{"user_id": item.UserID, "leave_type": item.LeaveType, "year": item.Year, "entitled_days": item.EntitledDays})
	httpapi.WriteJSON(w, http.StatusOK, item)
}
