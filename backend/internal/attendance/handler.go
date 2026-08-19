package attendance

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"name/backend/internal/auth"
)

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}

type Record struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	WorkDate    string     `json:"work_date"`
	CheckInAt   *time.Time `json:"check_in_at"`
	CheckOutAt  *time.Time `json:"check_out_at"`
	Status      string     `json:"status"`
	Note        string     `json:"note"`
	DisplayName string     `json:"display_name,omitempty"`
}

func (h Handler) ListOrganization(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT a.id, a.user_id, u.display_name, a.work_date::text, a.check_in_at, a.check_out_at, a.status, a.note FROM attendance_records a JOIN users u ON u.id = a.user_id WHERE a.organization_id = $1 ORDER BY a.work_date DESC, u.display_name LIMIT 1000`, user.OrganizationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load organization attendance"})
		return
	}
	defer rows.Close()
	items := make([]Record, 0)
	for rows.Next() {
		var item Record
		if err := rows.Scan(&item.ID, &item.UserID, &item.DisplayName, &item.WorkDate, &item.CheckInAt, &item.CheckOutAt, &item.Status, &item.Note); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read organization attendance"})
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}

type correctionRequest struct {
	UserID     string     `json:"user_id"`
	WorkDate   string     `json:"work_date"`
	CheckInAt  *time.Time `json:"check_in_at"`
	CheckOutAt *time.Time `json:"check_out_at"`
	Status     string     `json:"status"`
	Note       string     `json:"note"`
}

func (h Handler) Correct(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var input correctionRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.WorkDate) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id and work_date are required"})
		return
	}
	if input.Status == "" {
		input.Status = "present"
	}
	result, err := h.DB.Exec(r.Context(), `INSERT INTO attendance_records (organization_id,user_id,work_date,check_in_at,check_out_at,status,note) SELECT $1,id,$3,$4,$5,$6,$7 FROM users WHERE id=$2 AND organization_id=$1 ON CONFLICT (organization_id,user_id,work_date) DO UPDATE SET check_in_at=EXCLUDED.check_in_at, check_out_at=EXCLUDED.check_out_at, status=EXCLUDED.status, note=EXCLUDED.note, updated_at=NOW()`, user.OrganizationID, input.UserID, input.WorkDate, input.CheckInAt, input.CheckOutAt, input.Status, input.Note)
	if err != nil || result.RowsAffected() == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not correct attendance"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "corrected"})
}

type mutation struct {
	ID       string     `json:"id"`
	WorkDate string     `json:"work_date"`
	At       *time.Time `json:"at"`
	Note     string     `json:"note"`
}

func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT id, user_id, work_date::text, check_in_at, check_out_at, status, note FROM attendance_records WHERE organization_id = $1 AND user_id = $2 ORDER BY work_date DESC LIMIT 180`, user.OrganizationID, user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load attendance"})
		return
	}
	defer rows.Close()
	items := make([]Record, 0)
	for rows.Next() {
		var item Record
		if err := rows.Scan(&item.ID, &item.UserID, &item.WorkDate, &item.CheckInAt, &item.CheckOutAt, &item.Status, &item.Note); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read attendance"})
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}

func (h Handler) Mutate(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var input mutation
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid attendance request"})
		return
	}
	workDate := strings.TrimSpace(input.WorkDate)
	if workDate == "" {
		workDate = time.Now().UTC().Format("2006-01-02")
	}
	if input.At == nil {
		now := time.Now().UTC()
		input.At = &now
	}
	column := "check_in_at"
	if r.URL.Path == "/api/v1/attendance/check-out" {
		column = "check_out_at"
	}
	query := `INSERT INTO attendance_records (id, organization_id, user_id, work_date, ` + column + `, note) VALUES (COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6) ON CONFLICT (organization_id, user_id, work_date) DO UPDATE SET ` + column + ` = EXCLUDED.` + column + `, note = EXCLUDED.note, updated_at = NOW() RETURNING id, user_id, work_date::text, check_in_at, check_out_at, status, note`
	var item Record
	err = h.DB.QueryRow(r.Context(), query, input.ID, user.OrganizationID, user.ID, workDate, input.At, input.Note).Scan(&item.ID, &item.UserID, &item.WorkDate, &item.CheckInAt, &item.CheckOutAt, &item.Status, &item.Note)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not save attendance"})
		return
	}
	action := "attendance.check_in"
	if column == "check_out_at" {
		action = "attendance.check_out"
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,$3,'attendance',$4,$5)`, user.OrganizationID, user.ID, action, item.ID, map[string]any{"work_date": item.WorkDate})
	writeJSON(w, http.StatusOK, item)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
