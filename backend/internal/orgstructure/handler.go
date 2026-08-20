package orgstructure

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

type JobTitle struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

type ReportingLine struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	UserName    string `json:"user_name"`
	ManagerID   string `json:"manager_id"`
	ManagerName string `json:"manager_name"`
	IsPrimary   bool   `json:"is_primary"`
}

type File struct {
	ID          string `json:"id"`
	UploadedBy  string `json:"uploaded_by"`
	EntityType  string `json:"entity_type"`
	EntityID    string `json:"entity_id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	ScanStatus  string `json:"scan_status"`
	CreatedAt   string `json:"created_at"`
}

var validCategories = map[string]bool{
	"governance": true, "executive": true, "general_management": true,
	"senior_management": true, "middle_management": true, "professional": true, "junior": true,
}

// JobTitles lists (GET) or creates (POST) the organization's job-title catalogue.
func (h Handler) JobTitles(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `SELECT id, name, category FROM job_titles WHERE organization_id=$1 ORDER BY name`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load job titles")
			return
		}
		defer rows.Close()
		items := make([]JobTitle, 0)
		for rows.Next() {
			var item JobTitle
			if err := rows.Scan(&item.ID, &item.Name, &item.Category); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read job titles")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		Name     string `json:"name"`
		Category string `json:"category"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if input.Category == "" {
		input.Category = "professional"
	}
	if !validCategories[input.Category] {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_category", "category is not a recognized hierarchy group")
		return
	}
	var created JobTitle
	if err := h.DB.QueryRow(r.Context(), `INSERT INTO job_titles (organization_id, name, category) VALUES ($1,$2,$3) RETURNING id, name, category`, user.OrganizationID, strings.TrimSpace(input.Name), input.Category).Scan(&created.ID, &created.Name, &created.Category); err != nil {
		httpapi.WriteError(w, http.StatusConflict, "conflict", "job title may already exist")
		return
	}
	h.audit(r, user, "job_title.created", "job_title", created.ID)
	httpapi.WriteJSON(w, http.StatusCreated, created)
}

// AssignPlacement sets a user's job title and primary department. These are
// separate from permission roles, which are managed through the RBAC endpoints.
func (h Handler) AssignPlacement(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var input struct {
		UserID              string `json:"user_id"`
		JobTitleID          string `json:"job_title_id"`
		PrimaryDepartmentID string `json:"primary_department_id"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.UserID == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "user_id is required")
		return
	}
	var jobTitleID, departmentID any
	if strings.TrimSpace(input.JobTitleID) != "" {
		jobTitleID = input.JobTitleID
	}
	if strings.TrimSpace(input.PrimaryDepartmentID) != "" {
		departmentID = input.PrimaryDepartmentID
	}
	// Only touch users, titles, and departments inside the caller's organization.
	result, err := h.DB.Exec(r.Context(), `
		UPDATE users SET job_title_id=$2, primary_department_id=$3, updated_at=NOW()
		WHERE id=$1 AND organization_id=$4
		  AND ($2::uuid IS NULL OR EXISTS (SELECT 1 FROM job_titles t WHERE t.id=$2 AND t.organization_id=$4))
		  AND ($3::uuid IS NULL OR EXISTS (SELECT 1 FROM departments d WHERE d.id=$3 AND d.organization_id=$4))`,
		input.UserID, jobTitleID, departmentID, user.OrganizationID)
	if err != nil || result.RowsAffected() == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_placement", "user, title, or department does not belong to this organization")
		return
	}
	h.audit(r, user, "user.placement_updated", "user", input.UserID)
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ReportingLines lists (GET) or creates (POST) manager relationships.
func (h Handler) ReportingLines(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT rl.id, rl.user_id, u.display_name, rl.manager_id, m.display_name, rl.is_primary
			FROM reporting_lines rl
			JOIN users u ON u.id=rl.user_id
			JOIN users m ON m.id=rl.manager_id
			WHERE rl.organization_id=$1
			ORDER BY u.display_name`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load reporting lines")
			return
		}
		defer rows.Close()
		items := make([]ReportingLine, 0)
		for rows.Next() {
			var item ReportingLine
			if err := rows.Scan(&item.ID, &item.UserID, &item.UserName, &item.ManagerID, &item.ManagerName, &item.IsPrimary); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read reporting lines")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		UserID    string `json:"user_id"`
		ManagerID string `json:"manager_id"`
		IsPrimary *bool  `json:"is_primary"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.UserID == "" || input.ManagerID == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "user_id and manager_id are required")
		return
	}
	if input.UserID == input.ManagerID {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "a user cannot report to themselves")
		return
	}
	isPrimary := input.IsPrimary == nil || *input.IsPrimary
	// If this is a primary line, demote any existing primary for the user first.
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "tx_failed", "could not begin update")
		return
	}
	defer tx.Rollback(r.Context())
	if isPrimary {
		if _, err := tx.Exec(r.Context(), `UPDATE reporting_lines SET is_primary=FALSE WHERE user_id=$1 AND is_primary`, input.UserID); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "update_failed", "could not update reporting lines")
			return
		}
	}
	result, err := tx.Exec(r.Context(), `
		INSERT INTO reporting_lines (organization_id, user_id, manager_id, is_primary)
		SELECT $1, u.id, m.id, $4 FROM users u JOIN users m ON m.organization_id=u.organization_id
		WHERE u.id=$2 AND m.id=$3 AND u.organization_id=$1
		ON CONFLICT (user_id, manager_id) DO UPDATE SET is_primary=EXCLUDED.is_primary`,
		user.OrganizationID, input.UserID, input.ManagerID, isPrimary)
	if err != nil || result.RowsAffected() == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_line", "user or manager does not belong to this organization")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not save reporting line")
		return
	}
	h.audit(r, user, "reporting_line.set", "user", input.UserID)
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "set"})
}

// Files lists file metadata for the organization, optionally filtered by the
// entity a file is attached to.
func (h Handler) Files(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	entityType := r.URL.Query().Get("entity_type")
	entityID := r.URL.Query().Get("entity_id")
	rows, err := h.DB.Query(r.Context(), `
		SELECT id, COALESCE(uploaded_by::text,''), entity_type, COALESCE(entity_id::text,''), filename, content_type, size_bytes, scan_status, created_at::text
		FROM files
		WHERE organization_id=$1
		  AND ($2='' OR entity_type=$2)
		  AND ($3='' OR entity_id=$3::uuid)
		ORDER BY created_at DESC LIMIT 500`, user.OrganizationID, entityType, entityID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load files")
		return
	}
	defer rows.Close()
	items := make([]File, 0)
	for rows.Next() {
		var item File
		if err := rows.Scan(&item.ID, &item.UploadedBy, &item.EntityType, &item.EntityID, &item.Filename, &item.ContentType, &item.SizeBytes, &item.ScanStatus, &item.CreatedAt); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read files")
			return
		}
		items = append(items, item)
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

func (h Handler) audit(r *http.Request, user auth.SessionUser, action, entityType, entityID string) {
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id) VALUES ($1,$2,$3,$4,$5)`, user.OrganizationID, user.ID, action, entityType, entityID)
}
