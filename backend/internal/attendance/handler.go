package attendance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"name/backend/internal/auth"
	"name/backend/internal/httpapi"
	"name/backend/internal/notify"
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
	Hours       float64    `json:"hours"`
	DisplayName string     `json:"display_name,omitempty"`
	Position    string     `json:"position,omitempty"`
	PositionID  string     `json:"position_id,omitempty"`
	EmployeeID  string     `json:"employee_id,omitempty"`
	EarlyBy     string     `json:"early_by,omitempty"`
	LateBy      string     `json:"late_by,omitempty"`
}

type Person struct {
	UserID       string `json:"user_id"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	PositionID   string `json:"position_id"`
	PositionName string `json:"position_name"`
	EmployeeID   string `json:"employee_id"`
}

type Settings struct {
	ExpectedCheckInTime string `json:"expected_check_in_time"`
	CanEdit             bool   `json:"can_edit"`
	UpdatedAt           string `json:"updated_at,omitempty"`
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

type TodaySummary struct {
	WorkDate     string `json:"work_date"`
	PresentCount int    `json:"present_count"`
	RemoteCount  int    `json:"remote_count"`
	LeaveCount   int    `json:"leave_count"`
	AbsentCount  int    `json:"absent_count"`
	CheckedIn    int    `json:"checked_in"`
	CheckedOut   int    `json:"checked_out"`
	StillWorking int    `json:"still_working"`
}

func withHours(item Record) Record {
	item.Hours = HoursWorked(item.CheckInAt, item.CheckOutAt)
	return item
}

func (h Handler) expectedCheckIn(ctx context.Context, orgID string) time.Time {
	var raw string
	err := h.DB.QueryRow(ctx, `
		SELECT to_char(expected_check_in_time, 'HH24:MI:SS')
		FROM organization_attendance_settings WHERE organization_id=$1`, orgID).Scan(&raw)
	if err != nil || raw == "" {
		t, _ := ParseClock("09:00:00")
		return t
	}
	t, err := ParseClock(raw)
	if err != nil {
		t, _ = ParseClock("09:00:00")
	}
	return t
}

func (h Handler) enrich(item Record, expected time.Time) Record {
	item = withHours(item)
	item.EarlyBy, item.LateBy = EarlyLate(item.CheckInAt, expected)
	return item
}

func (h Handler) ensureSettings(ctx context.Context, orgID string) {
	_, _ = h.DB.Exec(ctx, `
		INSERT INTO organization_attendance_settings (organization_id, expected_check_in_time)
		VALUES ($1, TIME '09:00:00') ON CONFLICT DO NOTHING`, orgID)
}

func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT id, user_id, work_date::text, check_in_at, check_out_at, status, note
		FROM attendance_records
		WHERE organization_id=$1 AND user_id=$2
		ORDER BY work_date DESC LIMIT 180`, user.OrganizationID, user.ID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load attendance")
		return
	}
	defer rows.Close()
	expected := h.expectedCheckIn(r.Context(), user.OrganizationID)
	items := make([]Record, 0)
	for rows.Next() {
		var item Record
		if err := rows.Scan(&item.ID, &item.UserID, &item.WorkDate, &item.CheckInAt, &item.CheckOutAt, &item.Status, &item.Note); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read attendance")
			return
		}
		items = append(items, h.enrich(item, expected))
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

func (h Handler) ListOrganization(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	h.ensureSettings(r.Context(), user.OrganizationID)
	expected := h.expectedCheckIn(r.Context(), user.OrganizationID)
	query := `
		SELECT a.id, a.user_id, u.display_name, a.work_date::text, a.check_in_at, a.check_out_at, a.status, a.note,
		       COALESCE(jt.id::text,''), COALESCE(jt.name,''), COALESCE(ep.employee_code,'')
		FROM attendance_records a
		JOIN users u ON u.id=a.user_id
		LEFT JOIN job_titles jt ON jt.id=u.job_title_id
		LEFT JOIN employee_profiles ep ON ep.user_id=u.id AND ep.organization_id=a.organization_id
		WHERE a.organization_id=$1`
	args := []any{user.OrganizationID}
	argN := 2
	if date := strings.TrimSpace(r.URL.Query().Get("date")); date != "" {
		query += fmt.Sprintf(` AND a.work_date=$%d`, argN)
		args = append(args, date)
		argN++
	}
	if from := strings.TrimSpace(r.URL.Query().Get("from")); from != "" {
		query += fmt.Sprintf(` AND a.work_date>=$%d`, argN)
		args = append(args, from)
		argN++
	}
	if to := strings.TrimSpace(r.URL.Query().Get("to")); to != "" {
		query += fmt.Sprintf(` AND a.work_date<=$%d`, argN)
		args = append(args, to)
		argN++
	}
	if uid := strings.TrimSpace(r.URL.Query().Get("user_id")); uid != "" {
		query += fmt.Sprintf(` AND a.user_id=$%d`, argN)
		args = append(args, uid)
		argN++
	}
	if pos := strings.TrimSpace(r.URL.Query().Get("position_id")); pos != "" {
		query += fmt.Sprintf(` AND u.job_title_id=$%d`, argN)
		args = append(args, pos)
		argN++
	}
	limit := 1000
	if strings.TrimSpace(r.URL.Query().Get("recent")) == "1" {
		limit = 5
	}
	if n := strings.TrimSpace(r.URL.Query().Get("limit")); n != "" {
		if parsed, err := strconv.Atoi(n); err == nil && parsed > 0 {
			limit = parsed
			if limit > 1000 {
				limit = 1000
			}
		}
	}
	query += fmt.Sprintf(` ORDER BY COALESCE(a.check_in_at, a.updated_at) DESC, u.display_name LIMIT %d`, limit)

	rows, err := h.DB.Query(r.Context(), query, args...)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load organization attendance")
		return
	}
	defer rows.Close()
	items := make([]Record, 0)
	for rows.Next() {
		var item Record
		if err := rows.Scan(&item.ID, &item.UserID, &item.DisplayName, &item.WorkDate, &item.CheckInAt, &item.CheckOutAt, &item.Status, &item.Note,
			&item.PositionID, &item.Position, &item.EmployeeID); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read organization attendance")
			return
		}
		items = append(items, h.enrich(item, expected))
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

// Today returns a same-day rollup for managers.
func (h Handler) Today(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	var summary TodaySummary
	summary.WorkDate = date
	_ = h.DB.QueryRow(r.Context(), `
		SELECT
			COUNT(*) FILTER (WHERE status='present'),
			COUNT(*) FILTER (WHERE status='remote'),
			COUNT(*) FILTER (WHERE status='leave'),
			COUNT(*) FILTER (WHERE status='absent'),
			COUNT(*) FILTER (WHERE check_in_at IS NOT NULL),
			COUNT(*) FILTER (WHERE check_out_at IS NOT NULL),
			COUNT(*) FILTER (WHERE check_in_at IS NOT NULL AND check_out_at IS NULL)
		FROM attendance_records
		WHERE organization_id=$1 AND work_date=$2::date`,
		user.OrganizationID, date,
	).Scan(&summary.PresentCount, &summary.RemoteCount, &summary.LeaveCount, &summary.AbsentCount,
		&summary.CheckedIn, &summary.CheckedOut, &summary.StillWorking)
	httpapi.WriteJSON(w, http.StatusOK, summary)
}

func (h Handler) Settings(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	h.ensureSettings(r.Context(), user.OrganizationID)
	canEdit := h.Auth.HasPermission(r.Context(), user, "organization.manage")
	if r.Method == http.MethodGet {
		var clock, updated string
		_ = h.DB.QueryRow(r.Context(), `
			SELECT to_char(expected_check_in_time, 'HH24:MI:SS'), COALESCE(to_char(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),'')
			FROM organization_attendance_settings WHERE organization_id=$1`, user.OrganizationID,
		).Scan(&clock, &updated)
		if clock == "" {
			clock = "09:00:00"
		}
		httpapi.WriteJSON(w, http.StatusOK, Settings{ExpectedCheckInTime: clock, CanEdit: canEdit, UpdatedAt: updated})
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		httpapi.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET or POST required")
		return
	}
	if !canEdit {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "only company-level access (organization.manage) can change the company check-in time — department heads cannot")
		return
	}
	var input struct {
		ExpectedCheckInTime string `json:"expected_check_in_time"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "expected_check_in_time is required")
		return
	}
	clock := strings.TrimSpace(input.ExpectedCheckInTime)
	if _, err := ParseClock(clock); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_time", "expected_check_in_time must be HH:MM or HH:MM:SS")
		return
	}
	if len(clock) == 5 {
		clock += ":00"
	}
	_, err = h.DB.Exec(r.Context(), `
		INSERT INTO organization_attendance_settings (organization_id, expected_check_in_time, updated_by, updated_at)
		VALUES ($1, $2::time, $3, NOW())
		ON CONFLICT (organization_id) DO UPDATE SET
			expected_check_in_time=EXCLUDED.expected_check_in_time,
			updated_by=EXCLUDED.updated_by,
			updated_at=NOW()`,
		user.OrganizationID, clock, user.ID)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "save_failed", "could not save company check-in time")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'attendance.company_check_in_updated','organization',$1,$3)`,
		user.OrganizationID, user.ID, map[string]any{"expected_check_in_time": clock})
	httpapi.WriteJSON(w, http.StatusOK, Settings{ExpectedCheckInTime: clock, CanEdit: true})
}

func (h Handler) People(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT u.id, u.display_name, u.email, COALESCE(jt.id::text,''), COALESCE(jt.name,''), COALESCE(ep.employee_code,'')
		FROM users u
		LEFT JOIN job_titles jt ON jt.id=u.job_title_id
		LEFT JOIN employee_profiles ep ON ep.user_id=u.id AND ep.organization_id=u.organization_id
		WHERE u.organization_id=$1 AND u.status='active'
		ORDER BY u.display_name`, user.OrganizationID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load people")
		return
	}
	defer rows.Close()
	items := make([]Person, 0)
	for rows.Next() {
		var item Person
		if err := rows.Scan(&item.UserID, &item.DisplayName, &item.Email, &item.PositionID, &item.PositionName, &item.EmployeeID); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read people")
			return
		}
		items = append(items, item)
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

type mutation struct {
	ID       string     `json:"id"`
	UserID   string     `json:"user_id"`
	WorkDate string     `json:"work_date"`
	At       *time.Time `json:"at"`
	Note     string     `json:"note"`
	Status   string     `json:"status"`
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
	isCheckOut := strings.HasSuffix(r.URL.Path, "/check-out")
	targetUser := user.ID
	if input.UserID != "" && input.UserID != user.ID {
		if !h.Auth.HasPermission(r.Context(), user, "attendance.manage") {
			httpapi.WriteError(w, http.StatusForbidden, "forbidden", "attendance.manage required to check in others")
			return
		}
		targetUser = input.UserID
	}

	var existing Record
	_ = h.DB.QueryRow(r.Context(), `
		SELECT id, user_id, work_date::text, check_in_at, check_out_at, status, note
		FROM attendance_records WHERE organization_id=$1 AND user_id=$2 AND work_date=$3`,
		user.OrganizationID, targetUser, workDate,
	).Scan(&existing.ID, &existing.UserID, &existing.WorkDate, &existing.CheckInAt, &existing.CheckOutAt, &existing.Status, &existing.Note)

	if existing.Status == "leave" {
		httpapi.WriteError(w, http.StatusConflict, "on_leave", "cannot check in or out on an approved leave day")
		return
	}
	if existing.Status == "absent" && !isCheckOut {
		httpapi.WriteError(w, http.StatusConflict, "marked_absent", "day is marked absent; ask a manager to correct it")
		return
	}

	if isCheckOut {
		if existing.CheckInAt == nil {
			httpapi.WriteError(w, http.StatusBadRequest, "not_checked_in", "check in before checking out")
			return
		}
		if existing.CheckOutAt != nil {
			httpapi.WriteError(w, http.StatusConflict, "already_checked_out", "already checked out for this day")
			return
		}
		if !input.At.After(*existing.CheckInAt) {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_time", "check-out must be after check-in")
			return
		}
	} else if existing.CheckInAt != nil {
		httpapi.WriteError(w, http.StatusConflict, "already_checked_in", "already checked in for this day")
		return
	}

	status := "present"
	if input.Status != "" && ValidStatus(input.Status) && input.Status != "leave" && input.Status != "absent" {
		status = input.Status
	} else if existing.Status == "remote" {
		status = "remote"
	}

	var item Record
	if isCheckOut {
		err = h.DB.QueryRow(r.Context(), `
			UPDATE attendance_records SET check_out_at=$1, note=CASE WHEN $2='' THEN note ELSE $2 END, updated_at=NOW()
			WHERE organization_id=$3 AND user_id=$4 AND work_date=$5
			RETURNING id, user_id, work_date::text, check_in_at, check_out_at, status, note`,
			input.At, input.Note, user.OrganizationID, targetUser, workDate,
		).Scan(&item.ID, &item.UserID, &item.WorkDate, &item.CheckInAt, &item.CheckOutAt, &item.Status, &item.Note)
	} else {
		err = h.DB.QueryRow(r.Context(), `
			INSERT INTO attendance_records (id, organization_id, user_id, work_date, check_in_at, status, note)
			VALUES (COALESCE(NULLIF($1,'')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7)
			ON CONFLICT (organization_id, user_id, work_date) DO UPDATE SET
				check_in_at=EXCLUDED.check_in_at,
				status=EXCLUDED.status,
				note=EXCLUDED.note,
				updated_at=NOW()
			RETURNING id, user_id, work_date::text, check_in_at, check_out_at, status, note`,
			input.ID, user.OrganizationID, targetUser, workDate, input.At, status, input.Note,
		).Scan(&item.ID, &item.UserID, &item.WorkDate, &item.CheckInAt, &item.CheckOutAt, &item.Status, &item.Note)
	}
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "save_failed", "could not save attendance")
		return
	}

	action := "attendance.check_in"
	if isCheckOut {
		action = "attendance.check_out"
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,$3,'attendance',$4,$5)`,
		user.OrganizationID, user.ID, action, item.ID, map[string]any{"work_date": item.WorkDate, "hours": HoursWorked(item.CheckInAt, item.CheckOutAt), "user_id": targetUser})
	if targetUser != user.ID {
		_ = notify.Emit(r.Context(), h.DB, user.OrganizationID, targetUser, "attendance.check_in", "Checked in",
			fmt.Sprintf("You were checked in for %s.", item.WorkDate), "attendance", item.ID)
	}
	expected := h.expectedCheckIn(r.Context(), user.OrganizationID)
	httpapi.WriteJSON(w, http.StatusOK, h.enrich(item, expected))
}

// SetStatus lets a person mark today as remote (or a manager mark absent/leave/present).
func (h Handler) SetStatus(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var input struct {
		UserID   string `json:"user_id"`
		WorkDate string `json:"work_date"`
		Status   string `json:"status"`
		Note     string `json:"note"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || !ValidStatus(input.Status) {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "valid status is required")
		return
	}
	if input.WorkDate == "" {
		input.WorkDate = time.Now().UTC().Format("2006-01-02")
	}
	targetUser := user.ID
	if input.UserID != "" && input.UserID != user.ID {
		if !h.Auth.HasPermission(r.Context(), user, "attendance.manage") {
			httpapi.WriteError(w, http.StatusForbidden, "forbidden", "attendance.manage required to set status for others")
			return
		}
		targetUser = input.UserID
	}
	if input.Status == "absent" || input.Status == "leave" {
		if !h.Auth.HasPermission(r.Context(), user, "attendance.manage") && targetUser != user.ID {
			httpapi.WriteError(w, http.StatusForbidden, "forbidden", "only managers can mark absent/leave for others")
			return
		}
		if input.Status == "absent" && targetUser == user.ID && !h.Auth.HasPermission(r.Context(), user, "attendance.manage") {
			// Self-marking absent is allowed for the day (manager can correct later).
		}
	}

	var item Record
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO attendance_records (organization_id, user_id, work_date, status, note)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (organization_id, user_id, work_date) DO UPDATE SET
			status=EXCLUDED.status,
			note=CASE WHEN EXCLUDED.note='' THEN attendance_records.note ELSE EXCLUDED.note END,
			updated_at=NOW()
		RETURNING id, user_id, work_date::text, check_in_at, check_out_at, status, note`,
		user.OrganizationID, targetUser, input.WorkDate, input.Status, input.Note,
	).Scan(&item.ID, &item.UserID, &item.WorkDate, &item.CheckInAt, &item.CheckOutAt, &item.Status, &item.Note)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "save_failed", "could not update attendance status")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'attendance.status_set','attendance',$3,$4)`,
		user.OrganizationID, user.ID, item.ID, map[string]any{"work_date": item.WorkDate, "status": item.Status, "user_id": targetUser})
	if targetUser != user.ID {
		_ = notify.Emit(r.Context(), h.DB, user.OrganizationID, targetUser, "attendance.status", "Attendance updated",
			fmt.Sprintf("Your status for %s was set to %s.", item.WorkDate, item.Status), "attendance", item.ID)
	}
	httpapi.WriteJSON(w, http.StatusOK, withHours(item))
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
	if !ValidStatus(input.Status) {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_status", "status must be present, remote, leave, or absent")
		return
	}
	if input.CheckInAt != nil && input.CheckOutAt != nil && !input.CheckOutAt.After(*input.CheckInAt) {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_time", "check-out must be after check-in")
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "tx_failed", "could not start correction")
		return
	}
	defer tx.Rollback(r.Context())

	var prev Record
	_ = tx.QueryRow(r.Context(), `
		SELECT id, user_id, work_date::text, check_in_at, check_out_at, status, note
		FROM attendance_records WHERE organization_id=$1 AND user_id=$2 AND work_date=$3`,
		user.OrganizationID, input.UserID, input.WorkDate,
	).Scan(&prev.ID, &prev.UserID, &prev.WorkDate, &prev.CheckInAt, &prev.CheckOutAt, &prev.Status, &prev.Note)

	var item Record
	err = tx.QueryRow(r.Context(), `
		INSERT INTO attendance_records (organization_id,user_id,work_date,check_in_at,check_out_at,status,note)
		SELECT $1,id,$3,$4,$5,$6,$7 FROM users WHERE id=$2 AND organization_id=$1
		ON CONFLICT (organization_id,user_id,work_date) DO UPDATE SET
			check_in_at=EXCLUDED.check_in_at, check_out_at=EXCLUDED.check_out_at,
			status=EXCLUDED.status, note=EXCLUDED.note, updated_at=NOW()
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
	_ = notify.EmitUnlessActor(r.Context(), h.DB, user.OrganizationID, user.ID, input.UserID, "attendance.corrected", "Attendance corrected",
		fmt.Sprintf("Your attendance for %s was corrected.", input.WorkDate), "attendance", item.ID)
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"status": "corrected", "correction_id": correctionID, "record": withHours(item)})
}

func (h Handler) ListCorrections(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT c.id, COALESCE(c.attendance_id::text,''), c.user_id, u.display_name, c.corrected_by, actor.display_name,
			c.work_date::text, c.previous_check_in_at, c.previous_check_out_at, COALESCE(c.previous_status,''), COALESCE(c.previous_note,''),
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
