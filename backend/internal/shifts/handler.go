package shifts

import (
	"encoding/json"
	"fmt"
	"net/http"
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

type Shift struct {
	ID           string `json:"id"`
	AssignedTo   string `json:"assigned_to"`
	DisplayName  string `json:"display_name,omitempty"`
	Title        string `json:"title"`
	ShiftDate    string `json:"shift_date"`
	StartsAt     string `json:"starts_at"`
	EndsAt       string `json:"ends_at"`
	Status       string `json:"status"`
	Note         string `json:"note"`
	Duration     string `json:"duration,omitempty"`
	PositionName string `json:"position_name,omitempty"`
	EmployeeID   string `json:"employee_id,omitempty"`
}

type WeekSummary struct {
	From           string `json:"from"`
	To             string `json:"to"`
	ScheduledCount int    `json:"scheduled_count"`
	ConfirmedCount int    `json:"confirmed_count"`
	CompletedCount int    `json:"completed_count"`
	CancelledCount int    `json:"cancelled_count"`
	TotalHours     string `json:"total_hours"`
}

type createRequest struct {
	ID         string `json:"id"`
	AssignedTo string `json:"assigned_to"`
	Title      string `json:"title"`
	ShiftDate  string `json:"shift_date"`
	StartsAt   string `json:"starts_at"`
	EndsAt     string `json:"ends_at"`
	Note       string `json:"note"`
	Status     string `json:"status"`
}

func enrich(item Shift) Shift {
	item.StartsAt = trimClock(item.StartsAt)
	item.EndsAt = trimClock(item.EndsAt)
	item.Duration = DurationHMS(item.StartsAt, item.EndsAt)
	return item
}

func trimClock(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 8 {
		return value[:8]
	}
	if len(value) == 5 {
		return value + ":00"
	}
	return value
}

func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	canManage := h.Auth.HasPermission(r.Context(), user, "shifts.manage")
	query := `
		SELECT s.id, s.assigned_to, u.display_name, s.title, s.shift_date::text,
		       to_char(s.starts_at, 'HH24:MI:SS'), to_char(s.ends_at, 'HH24:MI:SS'),
		       s.status, s.note, COALESCE(jt.name,''), COALESCE(ep.employee_code,'')
		FROM shifts s
		JOIN users u ON u.id=s.assigned_to
		LEFT JOIN job_titles jt ON jt.id=u.job_title_id
		LEFT JOIN employee_profiles ep ON ep.user_id=u.id AND ep.organization_id=s.organization_id
		WHERE s.organization_id=$1`
	args := []any{user.OrganizationID}
	argN := 2

	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	if scope == "" {
		if canManage {
			scope = "team"
		} else {
			scope = "mine"
		}
	}
	if scope == "mine" || !canManage {
		query += fmt.Sprintf(` AND s.assigned_to=$%d`, argN)
		args = append(args, user.ID)
		argN++
	}
	if uid := strings.TrimSpace(r.URL.Query().Get("user_id")); uid != "" && canManage {
		query += fmt.Sprintf(` AND s.assigned_to=$%d`, argN)
		args = append(args, uid)
		argN++
	}
	if from := strings.TrimSpace(r.URL.Query().Get("from")); from != "" {
		query += fmt.Sprintf(` AND s.shift_date>=$%d`, argN)
		args = append(args, from)
		argN++
	}
	if to := strings.TrimSpace(r.URL.Query().Get("to")); to != "" {
		query += fmt.Sprintf(` AND s.shift_date<=$%d`, argN)
		args = append(args, to)
		argN++
	}
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" && ValidStatus(status) {
		query += fmt.Sprintf(` AND s.status=$%d`, argN)
		args = append(args, status)
		argN++
	}
	query += ` ORDER BY s.shift_date, s.starts_at LIMIT 1000`

	rows, err := h.DB.Query(r.Context(), query, args...)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load shifts")
		return
	}
	defer rows.Close()
	items := make([]Shift, 0)
	for rows.Next() {
		var item Shift
		if err := rows.Scan(&item.ID, &item.AssignedTo, &item.DisplayName, &item.Title, &item.ShiftDate,
			&item.StartsAt, &item.EndsAt, &item.Status, &item.Note, &item.PositionName, &item.EmployeeID); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read shifts")
			return
		}
		items = append(items, enrich(item))
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

func (h Handler) Week(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if from == "" || to == "" {
		now := time.Now().UTC()
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := now.AddDate(0, 0, -(weekday - 1))
		from = start.Format("2006-01-02")
		to = start.AddDate(0, 0, 6).Format("2006-01-02")
	}
	canManage := h.Auth.HasPermission(r.Context(), user, "shifts.manage")
	query := `
		SELECT
			COUNT(*) FILTER (WHERE status='scheduled'),
			COUNT(*) FILTER (WHERE status='confirmed'),
			COUNT(*) FILTER (WHERE status='completed'),
			COUNT(*) FILTER (WHERE status='cancelled'),
			COALESCE(SUM(EXTRACT(EPOCH FROM (ends_at - starts_at))) FILTER (WHERE status <> 'cancelled'), 0)
		FROM shifts
		WHERE organization_id=$1 AND shift_date>=$2::date AND shift_date<=$3::date`
	args := []any{user.OrganizationID, from, to}
	if !canManage {
		query += ` AND assigned_to=$4`
		args = append(args, user.ID)
	}
	var summary WeekSummary
	summary.From, summary.To = from, to
	var seconds float64
	if err := h.DB.QueryRow(r.Context(), query, args...).Scan(
		&summary.ScheduledCount, &summary.ConfirmedCount, &summary.CompletedCount, &summary.CancelledCount, &seconds,
	); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load week summary")
		return
	}
	total := int64(seconds)
	summary.TotalHours = fmt.Sprintf("%02d:%02d:%02d", total/3600, (total%3600)/60, total%60)
	httpapi.WriteJSON(w, http.StatusOK, summary)
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
	start, err := NormalizeClock(input.StartsAt)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_time", "starts_at must be HH:MM")
		return
	}
	end, err := NormalizeClock(input.EndsAt)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_time", "ends_at must be HH:MM")
		return
	}
	if !ParseMustAfter(start, end) {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_range", "end time must be after start time")
		return
	}
	if input.AssignedTo == "" {
		input.AssignedTo = user.ID
	}
	if input.AssignedTo != user.ID && !h.Auth.HasPermission(r.Context(), user, "shifts.manage") {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "shifts.manage required to assign others")
		return
	}
	if input.Status == "" {
		input.Status = "scheduled"
	}
	if !ValidStatus(input.Status) || input.Status == "cancelled" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_status", "status must be scheduled, confirmed, or completed")
		return
	}

	var onLeave bool
	_ = h.DB.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM leave_requests
			WHERE organization_id=$1 AND requested_by=$2 AND status='approved'
			  AND start_date<=$3::date AND end_date>=$3::date
		)`, user.OrganizationID, input.AssignedTo, input.ShiftDate).Scan(&onLeave)
	if onLeave {
		httpapi.WriteError(w, http.StatusConflict, "on_leave", "person has approved leave on this date")
		return
	}

	if conflict, ok := h.findOverlap(r, user.OrganizationID, input.AssignedTo, input.ShiftDate, start, end, ""); ok {
		httpapi.WriteError(w, http.StatusConflict, "overlap", fmt.Sprintf("overlaps existing shift %s (%s–%s)", conflict.Title, trimClock(conflict.StartsAt), trimClock(conflict.EndsAt)))
		return
	}

	var item Shift
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO shifts (id, organization_id, assigned_to, title, shift_date, starts_at, ends_at, note, status, created_by)
		VALUES (COALESCE(NULLIF($1,'')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6::time, $7::time, $8, $9, $10)
		RETURNING id, assigned_to, title, shift_date::text, to_char(starts_at,'HH24:MI:SS'), to_char(ends_at,'HH24:MI:SS'), status, note`,
		input.ID, user.OrganizationID, input.AssignedTo, strings.TrimSpace(input.Title), input.ShiftDate, start, end, input.Note, input.Status, user.ID,
	).Scan(&item.ID, &item.AssignedTo, &item.Title, &item.ShiftDate, &item.StartsAt, &item.EndsAt, &item.Status, &item.Note)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create shift")
		return
	}
	_ = h.DB.QueryRow(r.Context(), `SELECT display_name FROM users WHERE id=$1`, item.AssignedTo).Scan(&item.DisplayName)
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'shift.created','shift',$3,$4)`,
		user.OrganizationID, user.ID, item.ID, map[string]any{"shift_date": item.ShiftDate, "assigned_to": item.AssignedTo})
	if item.AssignedTo != user.ID {
		_ = notify.Emit(r.Context(), h.DB, user.OrganizationID, item.AssignedTo, "shift.assigned", "Shift assigned",
			fmt.Sprintf("You were scheduled for %s (%s–%s).", item.ShiftDate, item.StartsAt, item.EndsAt), "shift", item.ID)
	}
	httpapi.WriteJSON(w, http.StatusCreated, enrich(item))
}

func ParseMustAfter(start, end string) bool {
	s, err1 := ParseClock(start)
	e, err2 := ParseClock(end)
	return err1 == nil && err2 == nil && e.After(s)
}

func (h Handler) findOverlap(r *http.Request, orgID, assignedTo, date, start, end, excludeID string) (Shift, bool) {
	rows, err := h.DB.Query(r.Context(), `
		SELECT id, title, to_char(starts_at,'HH24:MI:SS'), to_char(ends_at,'HH24:MI:SS')
		FROM shifts
		WHERE organization_id=$1 AND assigned_to=$2 AND shift_date=$3::date AND status <> 'cancelled'
		  AND ($4='' OR id<>$4::uuid)`, orgID, assignedTo, date, excludeID)
	if err != nil {
		return Shift{}, false
	}
	defer rows.Close()
	for rows.Next() {
		var item Shift
		if rows.Scan(&item.ID, &item.Title, &item.StartsAt, &item.EndsAt) != nil {
			continue
		}
		if ClocksOverlap(start, end, item.StartsAt, item.EndsAt) {
			return item, true
		}
	}
	return Shift{}, false
}

func (h Handler) Update(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var input createRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.ID) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}
	var existing Shift
	err = h.DB.QueryRow(r.Context(), `
		SELECT id, assigned_to, title, shift_date::text, to_char(starts_at,'HH24:MI:SS'), to_char(ends_at,'HH24:MI:SS'), status, note
		FROM shifts WHERE id=$1 AND organization_id=$2`, input.ID, user.OrganizationID,
	).Scan(&existing.ID, &existing.AssignedTo, &existing.Title, &existing.ShiftDate, &existing.StartsAt, &existing.EndsAt, &existing.Status, &existing.Note)
	if err != nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "shift not found")
		return
	}
	canManage := h.Auth.HasPermission(r.Context(), user, "shifts.manage")
	if existing.AssignedTo != user.ID && !canManage {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "cannot edit another person's shift")
		return
	}

	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = existing.Title
	}
	date := input.ShiftDate
	if date == "" {
		date = existing.ShiftDate
	}
	start := existing.StartsAt
	end := existing.EndsAt
	if input.StartsAt != "" {
		start, err = NormalizeClock(input.StartsAt)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_time", "starts_at must be HH:MM")
			return
		}
	}
	if input.EndsAt != "" {
		end, err = NormalizeClock(input.EndsAt)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_time", "ends_at must be HH:MM")
			return
		}
	}
	if !ParseMustAfter(start, end) {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_range", "end time must be after start time")
		return
	}
	assigned := existing.AssignedTo
	if input.AssignedTo != "" && input.AssignedTo != existing.AssignedTo {
		if !canManage {
			httpapi.WriteError(w, http.StatusForbidden, "forbidden", "shifts.manage required to reassign")
			return
		}
		assigned = input.AssignedTo
	}
	note := existing.Note
	if input.Note != "" || r.URL.Query().Get("clear_note") == "1" {
		note = input.Note
	}
	status := existing.Status
	if input.Status != "" {
		if !ValidStatus(input.Status) {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_status", "invalid status")
			return
		}
		status = input.Status
	}

	if status != "cancelled" {
		if conflict, ok := h.findOverlap(r, user.OrganizationID, assigned, date, start, end, existing.ID); ok {
			httpapi.WriteError(w, http.StatusConflict, "overlap", fmt.Sprintf("overlaps existing shift %s", conflict.Title))
			return
		}
	}

	var item Shift
	err = h.DB.QueryRow(r.Context(), `
		UPDATE shifts SET assigned_to=$1, title=$2, shift_date=$3::date, starts_at=$4::time, ends_at=$5::time,
			note=$6, status=$7, updated_at=NOW()
		WHERE id=$8 AND organization_id=$9
		RETURNING id, assigned_to, title, shift_date::text, to_char(starts_at,'HH24:MI:SS'), to_char(ends_at,'HH24:MI:SS'), status, note`,
		assigned, title, date, start, end, note, status, existing.ID, user.OrganizationID,
	).Scan(&item.ID, &item.AssignedTo, &item.Title, &item.ShiftDate, &item.StartsAt, &item.EndsAt, &item.Status, &item.Note)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "update_failed", "could not update shift")
		return
	}
	_ = h.DB.QueryRow(r.Context(), `SELECT display_name FROM users WHERE id=$1`, item.AssignedTo).Scan(&item.DisplayName)
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'shift.updated','shift',$3,$4)`,
		user.OrganizationID, user.ID, item.ID, map[string]any{"status": item.Status, "shift_date": item.ShiftDate})
	httpapi.WriteJSON(w, http.StatusOK, enrich(item))
}

func (h Handler) SetStatus(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var input struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.ID == "" || !ValidStatus(input.Status) {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "id and valid status are required")
		return
	}
	var assigned string
	err = h.DB.QueryRow(r.Context(), `SELECT assigned_to FROM shifts WHERE id=$1 AND organization_id=$2`, input.ID, user.OrganizationID).Scan(&assigned)
	if err != nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "shift not found")
		return
	}
	canManage := h.Auth.HasPermission(r.Context(), user, "shifts.manage")
	if assigned != user.ID && !canManage {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "cannot change this shift")
		return
	}
	// Assignees can confirm/complete their own; cancel requires manage unless it's theirs and still scheduled.
	if input.Status == "cancelled" && assigned != user.ID && !canManage {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "only managers can cancel others' shifts")
		return
	}
	var item Shift
	err = h.DB.QueryRow(r.Context(), `
		UPDATE shifts SET status=$1, updated_at=NOW()
		WHERE id=$2 AND organization_id=$3
		RETURNING id, assigned_to, title, shift_date::text, to_char(starts_at,'HH24:MI:SS'), to_char(ends_at,'HH24:MI:SS'), status, note`,
		input.Status, input.ID, user.OrganizationID,
	).Scan(&item.ID, &item.AssignedTo, &item.Title, &item.ShiftDate, &item.StartsAt, &item.EndsAt, &item.Status, &item.Note)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "update_failed", "could not update status")
		return
	}
	_ = h.DB.QueryRow(r.Context(), `SELECT display_name FROM users WHERE id=$1`, item.AssignedTo).Scan(&item.DisplayName)
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'shift.status_set','shift',$3,$4)`,
		user.OrganizationID, user.ID, item.ID, map[string]any{"status": item.Status})
	httpapi.WriteJSON(w, http.StatusOK, enrich(item))
}
