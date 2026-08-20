package calendar

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

type Event struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	AllDay      bool      `json:"all_day"`
	Visibility  string    `json:"visibility"`
	CreatedBy   string    `json:"created_by"`
	CreatorName string    `json:"creator_name"`
}

// Overlaps reports whether two half-open intervals [aStart, aEnd) and
// [bStart, bEnd) intersect. Used to detect calendar time conflicts.
func Overlaps(aStart, aEnd, bStart, bEnd time.Time) bool {
	return aStart.Before(bEnd) && bStart.Before(aEnd)
}

// List returns events visible to the caller within an optional [from,to] window.
func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	rows, err := h.DB.Query(r.Context(), `
		SELECT e.id, e.title, e.description, e.starts_at, e.ends_at, e.all_day, e.visibility, COALESCE(e.created_by::text,''), COALESCE(u.display_name,'')
		FROM calendar_events e LEFT JOIN users u ON u.id=e.created_by
		WHERE e.organization_id=$1
		  AND (e.visibility='organization' OR e.created_by=$2)
		  AND ($3='' OR e.ends_at >= $3::timestamptz)
		  AND ($4='' OR e.starts_at <= $4::timestamptz)
		ORDER BY e.starts_at`, user.OrganizationID, user.ID, from, to)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load events")
		return
	}
	defer rows.Close()
	items := make([]Event, 0)
	for rows.Next() {
		var item Event
		if err := rows.Scan(&item.ID, &item.Title, &item.Description, &item.StartsAt, &item.EndsAt, &item.AllDay, &item.Visibility, &item.CreatedBy, &item.CreatorName); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read events")
			return
		}
		items = append(items, item)
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

type createRequest struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	AllDay      bool      `json:"all_day"`
	Visibility  string    `json:"visibility"`
}

// Create adds a calendar event and reports whether it conflicts with the
// creator's existing events.
func (h Handler) Create(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var input createRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Title) == "" || input.StartsAt.IsZero() || input.EndsAt.IsZero() {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "title, starts_at, and ends_at are required")
		return
	}
	if !input.EndsAt.After(input.StartsAt) {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_range", "ends_at must be after starts_at")
		return
	}
	if input.Visibility != "private" {
		input.Visibility = "organization"
	}
	// Detect a conflict against the creator's own events (half-open overlap).
	var conflict bool
	_ = h.DB.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM calendar_events WHERE organization_id=$1 AND created_by=$2 AND starts_at < $4 AND $3 < ends_at)`, user.OrganizationID, user.ID, input.StartsAt, input.EndsAt).Scan(&conflict)

	var item Event
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO calendar_events (organization_id, created_by, title, description, starts_at, ends_at, all_day, visibility)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, title, description, starts_at, ends_at, all_day, visibility`,
		user.OrganizationID, user.ID, strings.TrimSpace(input.Title), strings.TrimSpace(input.Description), input.StartsAt, input.EndsAt, input.AllDay, input.Visibility).
		Scan(&item.ID, &item.Title, &item.Description, &item.StartsAt, &item.EndsAt, &item.AllDay, &item.Visibility)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create event")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]any{"event": item, "conflict": conflict})
}
