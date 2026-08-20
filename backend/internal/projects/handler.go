package projects

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

type Project struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	LeadID      string `json:"lead_id"`
	LeadName    string `json:"lead_name"`
	TaskCount   int    `json:"task_count"`
	CreatedAt   string `json:"created_at"`
}

// List returns the organization's projects with open task counts.
func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT p.id, p.key, p.name, p.description, p.status, COALESCE(p.lead_id::text,''), COALESCE(l.display_name,''),
		       (SELECT COUNT(*) FROM tasks t WHERE t.project_id=p.id), p.created_at::text
		FROM projects p LEFT JOIN users l ON l.id=p.lead_id
		WHERE p.organization_id=$1 ORDER BY p.status, p.name`, user.OrganizationID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load projects")
		return
	}
	defer rows.Close()
	items := make([]Project, 0)
	for rows.Next() {
		var item Project
		if err := rows.Scan(&item.ID, &item.Key, &item.Name, &item.Description, &item.Status, &item.LeadID, &item.LeadName, &item.TaskCount, &item.CreatedAt); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read projects")
			return
		}
		items = append(items, item)
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

type createRequest struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	LeadID      string `json:"lead_id"`
}

// Create adds a project scoped to the caller's organization.
func (h Handler) Create(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var input createRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Key) == "" || strings.TrimSpace(input.Name) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "key and name are required")
		return
	}
	var leadID any
	if strings.TrimSpace(input.LeadID) != "" {
		leadID = input.LeadID
	}
	var created Project
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO projects (organization_id, key, name, description, lead_id, created_by)
		SELECT $1,$2,$3,$4,$5,$6
		WHERE $5::uuid IS NULL OR EXISTS (SELECT 1 FROM users u WHERE u.id=$5 AND u.organization_id=$1)
		RETURNING id, key, name, description, status, COALESCE(lead_id::text,''), created_at::text`,
		user.OrganizationID, strings.ToUpper(strings.TrimSpace(input.Key)), strings.TrimSpace(input.Name), strings.TrimSpace(input.Description), leadID, user.ID).
		Scan(&created.ID, &created.Key, &created.Name, &created.Description, &created.Status, &created.LeadID, &created.CreatedAt)
	if err != nil {
		httpapi.WriteError(w, http.StatusConflict, "conflict", "project key may already exist")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'project.created','project',$3,$4)`, user.OrganizationID, user.ID, created.ID, map[string]any{"key": created.Key})
	httpapi.WriteJSON(w, http.StatusCreated, created)
}
