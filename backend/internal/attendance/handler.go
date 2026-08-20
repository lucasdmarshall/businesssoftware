package attendance

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

type Correction struct {
	ID                 string     `json:"id"`
	AttendanceID       string     `json:"attendance_id"`
	UserID             string     `json:"user_id"`
	DisplayName        string     `json:"display_name,omitempty"`
	CorrectedBy        string     `json:"corrected_by"`
	CorrectorName      string     `json:"corrector_name,omitempty"`
	WorkDate           string     `json:"work_date"`
	PreviousCheckInAt  *time.Time `json:"previous_check_in_at"`
	PreviousCheckOutAt *time.Time `json:"previous_check_out_at"`
	PreviousStatus     string     `json:"previous_status"`
	PreviousNote       string     `json:"previous_note"`
	CheckInAt          *time.Time `json:"check_in_at"`
	CheckOutAt         *time.Time `json:"check_out_at"`
	Status             string     `json:"status"`
	Note               string     `json:"note"`
	CreatedAt          time.Time  `json:"created_at"`
}

func (h Handler) ListOrganization(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT a.id, a.user_id, u.display_name, a.work_date::text, a.check_in_at, a.check_out_at, a.status, a.note FROM attendance_records a JOIN users u ON u.id = a.user_id WHERE a.organization_id = $1 ORDER BY a.work_date DESC, u.display_name LIMIT 1000`, user.OrganizationID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load organization attendance")
		return
	}
	defer rows.Close()
	items := make([]Record, 0)
	for rows.Next() {
		var item Record
		if err := rows.Scan(&item.ID, &item.UserID, &item.DisplayName, &item.WorkDate, &item.CheckInAt, &item.CheckOutAt, &item.Status, &item.Note); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read organization attendance")
			return
		}
		items = append(items, item)
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
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
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var input correctionRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.WorkDate) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "user_id and work_date are required")
		return
	}
	if input.Status == "" {
		input.Status = "present"
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "tx_failed", "could not start correction")
		return
	}
	defer tx.Rollback(r.Context())

	var prev Record
	_ = tx.QueryRow(r.Context(), `SELECT id, user_id, work_date::text, check_in_at, check_out_at, status, note FROM attendance_records WHERE organization_id=$1 AND user_id=$2 AND work_date=$3`,
		user.OrganizationID, input.UserID, input.WorkDate).Scan(&prev.ID, &prev.UserID, &prev.WorkDate, &prev.CheckInAt, &prev.CheckOutAt, &prev.Status, &prev.Note)

	var item Record
	err = tx.QueryRow(r.Context(), `
		INSERT INTO attendance_records (organization_id,user_id,work_date,check_in_at,check_out_at,status,note)
		SELECT $1,id,$3,$4,$5,$6,$7 FROM users WHERE id=$2 AND organization_id=$1
		ON CONFLICT (organization_id,user_id,work_date) DO UPDATE SET
			check_in_at=EXCLUDED.check_in_at, check_out_at=EXCLUDED.check_out_at, status=EXCLUDED.status, note=EXCLUDED.note, updated_at=NOW()
		RETURNING id, user_id, work_date::text, check_in_at, check_out_at, status, note`,
		user.OrganizationID, input.UserID, input.WorkDate, input.CheckInAt, input.CheckOutAt, input.Status, input.Note,
	).Scan(&item.ID, &item.UserID, &item.WorkDate, &item.CheckInAt, &item.CheckOutAt, &item.Status, &item.Note)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "correct_failed", "could not correct attendance")
		return
	}

	var correctionID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO attendance_corrections (
			organization_id, attendance_id, user_id, corrected_by, work_date,
			previous_check_in_at, previous_check_out_at, previous_status, previous_note,
			check_in_at, check_out_at, status, note
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
		user.OrganizationID, item.ID, input.UserID, user.ID, input.WorkDate,
		prev.CheckInAt, prev.CheckOutAt, prev.Status, prev.Note,
		input.CheckInAt, input.CheckOutAt, input.Status, input.Note,
	).Scan(&correctionID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "history_failed", "could not record correction history")
		return
	}

	_, _ = tx.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'attendance.corrected','attendance',$3,$4)`,
		user.OrganizationID, user.ID, item.ID, map[string]any{"work_date": item.WorkDate, "correction_id": correctionID, "user_id": input.UserID})

	if err := tx.Commit(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not save correction")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"status": "corrected", "correction_id": correctionID, "record": item})
}

func (h Handler) ListCorrections(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT c.id, COALESCE(c.attendance_id::text,''), c.user_id, u.display_name, c.corrected_by, actor.display_name,
			c.work_date::text, c.previous_check_in_at, c.previous_check_out_at, c.previous_status, c.previous_note,
			c.check_in_at, c.check_out_at, c.status, c.note, c.created_at
		FROM attendance_corrections c
		JOIN users u ON u.id=c.user_id
		JOIN users actor ON actor.id=c.corrected_by
		WHERE c.organization_id=$1
		ORDER BY c.created_at DESC LIMIT 200`, user.OrganizationID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load correction history")
		return
	}
	defer rows.Close()
	items := make([]Correction, 0)
	for rows.Next() {
		var item Correction
		if err := rows.Scan(&item.ID, &item.AttendanceID, &item.UserID, &item.DisplayName, &item.CorrectedBy, &item.CorrectorName,
			&item.WorkDate, &item.PreviousCheckInAt, &item.PreviousCheckOutAt, &item.PreviousStatus, &item.PreviousNote,
			&item.CheckInAt, &item.CheckOutAt, &item.Status, &item.Note, &item.CreatedAt); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read correction history")
			return
		}
		items = append(items, item)
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
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
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT id, user_id, work_date::text, check_in_at, check_out_at, status, note FROM attendance_records WHERE organization_id = $1 AND user_id = $2 ORDER BY work_date DESC LIMIT 180`, user.OrganizationID, user.ID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load attendance")
		return
	}
	defer rows.Close()
	items := make([]Record, 0)
	for rows.Next() {
		var item Record
		if err := rows.Scan(&item.ID, &item.UserID, &item.WorkDate, &item.CheckInAt, &item.CheckOutAt, &item.Status, &item.Note); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read attendance")
			return
		}
		items = append(items, item)
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

func (h Handler) Mutate(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var input mutation
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid attendance request")
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
	// Upsert is keyed on (organization_id, user_id, work_date) so cross-device
	// sync merges into the same day record rather than creating duplicates.
	query := `INSERT INTO attendance_records (id, organization_id, user_id, work_date, ` + column + `, note) VALUES (COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6) ON CONFLICT (organization_id, user_id, work_date) DO UPDATE SET ` + column + ` = EXCLUDED.` + column + `, note = EXCLUDED.note, updated_at = NOW() RETURNING id, user_id, work_date::text, check_in_at, check_out_at, status, note`
	var item Record
	err = h.DB.QueryRow(r.Context(), query, input.ID, user.OrganizationID, user.ID, workDate, input.At, input.Note).Scan(&item.ID, &item.UserID, &item.WorkDate, &item.CheckInAt, &item.CheckOutAt, &item.Status, &item.Note)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "save_failed", "could not save attendance")
		return
	}
	action := "attendance.check_in"
	if column == "check_out_at" {
		action = "attendance.check_out"
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,$3,'attendance',$4,$5)`, user.OrganizationID, user.ID, action, item.ID, map[string]any{"work_date": item.WorkDate})
	httpapi.WriteJSON(w, http.StatusOK, item)
}
