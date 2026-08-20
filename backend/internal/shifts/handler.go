package shifts

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"name/backend/internal/auth"
	"name/backend/internal/httpapi"
)

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}
type Shift struct {
	ID          string `json:"id"`
	AssignedTo  string `json:"assigned_to"`
	DisplayName string `json:"display_name,omitempty"`
	Title       string `json:"title"`
	ShiftDate   string `json:"shift_date"`
	StartsAt    string `json:"starts_at"`
	EndsAt      string `json:"ends_at"`
	Status      string `json:"status"`
	Note        string `json:"note"`
}
type createRequest struct {
	ID         string `json:"id"`
	AssignedTo string `json:"assigned_to"`
	Title      string `json:"title"`
	ShiftDate  string `json:"shift_date"`
	StartsAt   string `json:"starts_at"`
	EndsAt     string `json:"ends_at"`
	Note       string `json:"note"`
}

func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT s.id,s.assigned_to,u.display_name,s.title,s.shift_date::text,s.starts_at::text,s.ends_at::text,s.status,s.note FROM shifts s JOIN users u ON u.id=s.assigned_to WHERE s.organization_id=$1 ORDER BY s.shift_date,s.starts_at LIMIT 1000`, user.OrganizationID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load shifts")
		return
	}
	defer rows.Close()
	items := make([]Shift, 0)
	for rows.Next() {
		var item Shift
		if err := rows.Scan(&item.ID, &item.AssignedTo, &item.DisplayName, &item.Title, &item.ShiftDate, &item.StartsAt, &item.EndsAt, &item.Status, &item.Note); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read shifts")
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
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Title) == "" || input.ShiftDate == "" || input.StartsAt == "" || input.EndsAt == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "title, date, start, and end are required")
		return
	}
	if input.AssignedTo == "" {
		input.AssignedTo = user.ID
	}
	var item Shift
	err = h.DB.QueryRow(r.Context(), `INSERT INTO shifts (id,organization_id,assigned_to,title,shift_date,starts_at,ends_at,note) VALUES (COALESCE(NULLIF($1,'')::uuid,gen_random_uuid()),$2,$3,$4,$5,$6,$7,$8) RETURNING id,assigned_to,title,shift_date::text,starts_at::text,ends_at::text,status,note`, input.ID, user.OrganizationID, input.AssignedTo, input.Title, input.ShiftDate, input.StartsAt, input.EndsAt, input.Note).Scan(&item.ID, &item.AssignedTo, &item.Title, &item.ShiftDate, &item.StartsAt, &item.EndsAt, &item.Status, &item.Note)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create shift")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'shift.created','shift',$3,$4)`, user.OrganizationID, user.ID, item.ID, map[string]any{"shift_date": item.ShiftDate, "assigned_to": item.AssignedTo})
	httpapi.WriteJSON(w, http.StatusCreated, item)
}

// UpdateStatus moves a shift between scheduled, confirmed, cancelled, and completed.
func (h Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	id := r.PathValue("id")
	var input struct {
		Status string `json:"status"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "status is required")
		return
	}
	switch input.Status {
	case "scheduled", "confirmed", "cancelled", "completed":
	default:
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_status", "status must be scheduled, confirmed, cancelled, or completed")
		return
	}
	var item Shift
	err = h.DB.QueryRow(r.Context(), `
		UPDATE shifts SET status=$1, updated_at=NOW()
		WHERE id=$2 AND organization_id=$3
		RETURNING id, assigned_to, title, shift_date::text, starts_at::text, ends_at::text, status, note`,
		input.Status, id, user.OrganizationID,
	).Scan(&item.ID, &item.AssignedTo, &item.Title, &item.ShiftDate, &item.StartsAt, &item.EndsAt, &item.Status, &item.Note)
	if err != nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "shift not found")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'shift.status_updated','shift',$3,$4)`,
		user.OrganizationID, user.ID, item.ID, map[string]any{"status": item.Status})
	httpapi.WriteJSON(w, http.StatusOK, item)
}
