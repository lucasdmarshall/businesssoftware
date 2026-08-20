// Package lookups serves and manages admin-editable dropdown option lists
// (reference data) such as leave types, expense categories and ticket
// priorities. Reads are available to any authenticated member so forms can be
// populated; writes are gated by the domain "manage" permission that owns the
// category (e.g. leave types require hr.manage), which the org owner always has.
package lookups

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"name/backend/internal/auth"
)

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}

// Category metadata: the permission that governs edits and a human title.
type categoryMeta struct {
	Permission string
	Title      string
}

var categories = map[string]categoryMeta{
	"leave_type":        {Permission: "hr.manage", Title: "Leave types"},
	"expense_category":  {Permission: "finance.manage", Title: "Expense categories"},
	"ticket_priority":   {Permission: "itops.manage", Title: "Ticket priorities"},
	"ticket_category":   {Permission: "itops.manage", Title: "Ticket categories"},
	"asset_type":        {Permission: "itops.manage", Title: "Asset types"},
	"lead_source":       {Permission: "sales.manage", Title: "Lead sources"},
	"opportunity_stage": {Permission: "sales.manage", Title: "Opportunity stages"},
	"project_status":    {Permission: "projects.manage", Title: "Project statuses"},
	"event_category":    {Permission: "calendar.manage", Title: "Event categories"},
}

type defaultOption struct {
	Value string
	Label string
	Color string
}

// Built-in defaults seeded per organization the first time a category is read.
var defaults = map[string][]defaultOption{
	"leave_type": {
		{"annual", "Annual leave", "#2d6a58"},
		{"sick", "Sick leave", "#aa6c29"},
		{"personal", "Personal leave", "#3b6ea5"},
		{"unpaid", "Unpaid leave", "#777975"},
	},
	"expense_category": {
		{"travel", "Travel", "#3b6ea5"}, {"meals", "Meals", "#aa6c29"},
		{"software", "Software", "#2d6a58"}, {"equipment", "Equipment", "#6b4fa5"},
		{"office", "Office", "#777975"}, {"other", "Other", "#a5a6a1"},
	},
	"ticket_priority": {
		{"low", "Low", "#777975"}, {"medium", "Medium", "#3b6ea5"},
		{"high", "High", "#aa6c29"}, {"urgent", "Urgent", "#b3453b"},
	},
	"ticket_category": {
		{"hardware", "Hardware", "#6b4fa5"}, {"software", "Software", "#2d6a58"},
		{"access", "Access", "#3b6ea5"}, {"network", "Network", "#aa6c29"}, {"other", "Other", "#a5a6a1"},
	},
	"asset_type": {
		{"laptop", "Laptop", "#2d6a58"}, {"monitor", "Monitor", "#3b6ea5"},
		{"phone", "Phone", "#6b4fa5"}, {"license", "License", "#aa6c29"}, {"peripheral", "Peripheral", "#777975"},
	},
	"lead_source": {
		{"referral", "Referral", "#2d6a58"}, {"website", "Website", "#3b6ea5"},
		{"event", "Event", "#6b4fa5"}, {"outbound", "Outbound", "#aa6c29"}, {"other", "Other", "#a5a6a1"},
	},
	"opportunity_stage": {
		{"prospecting", "Prospecting", "#777975"}, {"qualified", "Qualified", "#3b6ea5"},
		{"proposal", "Proposal", "#6b4fa5"}, {"negotiation", "Negotiation", "#aa6c29"},
		{"won", "Won", "#2d6a58"}, {"lost", "Lost", "#b3453b"},
	},
	"project_status": {
		{"planned", "Planned", "#777975"}, {"active", "Active", "#2d6a58"},
		{"on_hold", "On hold", "#aa6c29"}, {"completed", "Completed", "#3b6ea5"},
	},
	"event_category": {
		{"meeting", "Meeting", "#2d6a58"}, {"deadline", "Deadline", "#b3453b"},
		{"holiday", "Holiday", "#3b6ea5"}, {"reminder", "Reminder", "#aa6c29"}, {"other", "Other", "#a5a6a1"},
	},
}

type Option struct {
	ID        string `json:"id"`
	Category  string `json:"category"`
	Value     string `json:"value"`
	Label     string `json:"label"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
	IsActive  bool   `json:"is_active"`
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	return strings.Trim(slugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "_"), "_")
}

// Catalog returns every category with its title and whether the current user
// may edit it — used by the Settings screen to show only manageable lists.
func (h Handler) Catalog(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	type catalogItem struct {
		Category   string `json:"category"`
		Title      string `json:"title"`
		Permission string `json:"permission"`
		Editable   bool   `json:"editable"`
	}
	items := make([]catalogItem, 0, len(categories))
	for category, meta := range categories {
		items = append(items, catalogItem{
			Category:   category,
			Title:      meta.Title,
			Permission: meta.Permission,
			Editable:   h.Auth.HasPermission(r.Context(), user, meta.Permission),
		})
	}
	writeJSON(w, http.StatusOK, items)
}

// List returns the active options for one category, seeding defaults on first
// use, and reports whether the caller can edit them.
func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	category := r.PathValue("category")
	meta, ok := categories[category]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown category"})
		return
	}
	h.seedDefaults(r, user.OrganizationID, category)

	includeInactive := r.URL.Query().Get("all") == "1"
	query := `SELECT id, category, value, label, COALESCE(color,''), sort_order, is_active FROM lookup_options WHERE organization_id=$1 AND category=$2`
	if !includeInactive {
		query += ` AND is_active=TRUE`
	}
	query += ` ORDER BY sort_order, label`
	rows, err := h.DB.Query(r.Context(), query, user.OrganizationID, category)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load options"})
		return
	}
	defer rows.Close()
	options := make([]Option, 0)
	for rows.Next() {
		var o Option
		if err := rows.Scan(&o.ID, &o.Category, &o.Value, &o.Label, &o.Color, &o.SortOrder, &o.IsActive); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read options"})
			return
		}
		options = append(options, o)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"category": category,
		"title":    meta.Title,
		"editable": h.Auth.HasPermission(r.Context(), user, meta.Permission),
		"options":  options,
	})
}

// seedDefaults inserts the built-in options for a category the first time an
// organization touches it, so the lists are immediately editable. Idempotent
// via the (organization_id, category, value) unique constraint.
func (h Handler) seedDefaults(r *http.Request, organizationID, category string) {
	var count int
	if err := h.DB.QueryRow(r.Context(), `SELECT COUNT(*) FROM lookup_options WHERE organization_id=$1 AND category=$2`, organizationID, category).Scan(&count); err != nil || count > 0 {
		return
	}
	for i, d := range defaults[category] {
		_, _ = h.DB.Exec(r.Context(), `INSERT INTO lookup_options (organization_id, category, value, label, color, sort_order) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (organization_id, category, value) DO NOTHING`, organizationID, category, d.Value, d.Label, d.Color, i)
	}
}

type upsertRequest struct {
	Value     string `json:"value"`
	Label     string `json:"label"`
	Color     string `json:"color"`
	SortOrder *int   `json:"sort_order"`
	IsActive  *bool  `json:"is_active"`
}

// Create adds a new option to a category (requires the domain manage permission).
func (h Handler) Create(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	category := r.PathValue("category")
	meta, ok := categories[category]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown category"})
		return
	}
	if !h.Auth.HasPermission(r.Context(), user, meta.Permission) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "you do not have permission to edit this list"})
		return
	}
	var input upsertRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Label) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "label is required"})
		return
	}
	value := slugify(input.Value)
	if value == "" {
		value = slugify(input.Label)
	}
	if value == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a value could not be derived from the label"})
		return
	}
	sortOrder := 0
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	} else {
		_ = h.DB.QueryRow(r.Context(), `SELECT COALESCE(MAX(sort_order)+1,0) FROM lookup_options WHERE organization_id=$1 AND category=$2`, user.OrganizationID, category).Scan(&sortOrder)
	}
	var o Option
	err = h.DB.QueryRow(r.Context(), `INSERT INTO lookup_options (organization_id, category, value, label, color, sort_order, created_by) VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7) RETURNING id, category, value, label, COALESCE(color,''), sort_order, is_active`, user.OrganizationID, category, value, strings.TrimSpace(input.Label), strings.TrimSpace(input.Color), sortOrder, user.ID).Scan(&o.ID, &o.Category, &o.Value, &o.Label, &o.Color, &o.SortOrder, &o.IsActive)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "an option with that value already exists"})
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

// Update edits an option's label/color/order/active flag (requires the manage
// permission of the option's own category).
func (h Handler) Update(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	id := r.PathValue("id")
	var category string
	if err := h.DB.QueryRow(r.Context(), `SELECT category FROM lookup_options WHERE id=$1 AND organization_id=$2`, id, user.OrganizationID).Scan(&category); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "option not found"})
		return
	}
	meta := categories[category]
	if !h.Auth.HasPermission(r.Context(), user, meta.Permission) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "you do not have permission to edit this list"})
		return
	}
	var input upsertRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	var o Option
	err = h.DB.QueryRow(r.Context(), `UPDATE lookup_options SET
			label = COALESCE(NULLIF($3,''), label),
			color = CASE WHEN $4='' THEN color ELSE $4 END,
			sort_order = COALESCE($5, sort_order),
			is_active = COALESCE($6, is_active),
			updated_at = NOW()
		WHERE id=$1 AND organization_id=$2
		RETURNING id, category, value, label, COALESCE(color,''), sort_order, is_active`,
		id, user.OrganizationID, strings.TrimSpace(input.Label), strings.TrimSpace(input.Color), input.SortOrder, input.IsActive).
		Scan(&o.ID, &o.Category, &o.Value, &o.Label, &o.Color, &o.SortOrder, &o.IsActive)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "option not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update option"})
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// Delete deactivates an option (soft delete) so historical records keep their
// label. Requires the manage permission of the option's category.
func (h Handler) Delete(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	id := r.PathValue("id")
	var category string
	if err := h.DB.QueryRow(r.Context(), `SELECT category FROM lookup_options WHERE id=$1 AND organization_id=$2`, id, user.OrganizationID).Scan(&category); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "option not found"})
		return
	}
	if !h.Auth.HasPermission(r.Context(), user, categories[category].Permission) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "you do not have permission to edit this list"})
		return
	}
	if _, err := h.DB.Exec(r.Context(), `UPDATE lookup_options SET is_active=FALSE, updated_at=NOW() WHERE id=$1 AND organization_id=$2`, id, user.OrganizationID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not remove option"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
