package shifts

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"name/backend/internal/auth"
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
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT s.id,s.assigned_to,u.display_name,s.title,s.shift_date::text,s.starts_at::text,s.ends_at::text,s.status,s.note FROM shifts s JOIN users u ON u.id=s.assigned_to WHERE s.organization_id=$1 ORDER BY s.shift_date,s.starts_at LIMIT 1000`, user.OrganizationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load shifts"})
		return
	}
	defer rows.Close()
	items := make([]Shift, 0)
	for rows.Next() {
		var item Shift
		if err := rows.Scan(&item.ID, &item.AssignedTo, &item.DisplayName, &item.Title, &item.ShiftDate, &item.StartsAt, &item.EndsAt, &item.Status, &item.Note); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read shifts"})
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}

func (h Handler) Create(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var input createRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Title) == "" || input.ShiftDate == "" || input.StartsAt == "" || input.EndsAt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title, date, start, and end are required"})
		return
	}
	if input.AssignedTo == "" {
		input.AssignedTo = user.ID
	}
	var item Shift
	err = h.DB.QueryRow(r.Context(), `INSERT INTO shifts (id,organization_id,assigned_to,title,shift_date,starts_at,ends_at,note) VALUES (COALESCE(NULLIF($1,'')::uuid,gen_random_uuid()),$2,$3,$4,$5,$6,$7,$8) RETURNING id,assigned_to,title,shift_date::text,starts_at::text,ends_at::text,status,note`, input.ID, user.OrganizationID, input.AssignedTo, input.Title, input.ShiftDate, input.StartsAt, input.EndsAt, input.Note).Scan(&item.ID, &item.AssignedTo, &item.Title, &item.ShiftDate, &item.StartsAt, &item.EndsAt, &item.Status, &item.Note)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not create shift"})
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'shift.created','shift',$3,$4)`, user.OrganizationID, user.ID, item.ID, map[string]any{"shift_date": item.ShiftDate, "assigned_to": item.AssignedTo})
	writeJSON(w, http.StatusCreated, item)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
