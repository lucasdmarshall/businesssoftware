package rbac

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
type Role struct {
	ID          string   `json:"id"`
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}
type createRoleRequest struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}
type assignRequest struct {
	UserID string `json:"user_id"`
	RoleID string `json:"role_id"`
}

func (h Handler) Permissions(w http.ResponseWriter, r *http.Request) {
	if _, err := h.Auth.Authenticate(r); err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT code,description FROM permissions ORDER BY code`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load permissions"})
		return
	}
	defer rows.Close()
	type item struct {
		Code        string `json:"code"`
		Description string `json:"description"`
	}
	items := make([]item, 0)
	for rows.Next() {
		var value item
		if err := rows.Scan(&value.Code, &value.Description); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read permissions"})
			return
		}
		items = append(items, value)
	}
	writeJSON(w, http.StatusOK, items)
}

func (h Handler) Roles(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT ro.id,ro.code,ro.name,COALESCE(array_agg(rp.permission_code) FILTER (WHERE rp.permission_code IS NOT NULL),'{}') FROM roles ro LEFT JOIN role_permissions rp ON rp.role_id=ro.id WHERE ro.organization_id=$1 GROUP BY ro.id ORDER BY ro.name`, user.OrganizationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load roles"})
		return
	}
	defer rows.Close()
	items := make([]Role, 0)
	for rows.Next() {
		var value Role
		if err := rows.Scan(&value.ID, &value.Code, &value.Name, &value.Permissions); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read roles"})
			return
		}
		items = append(items, value)
	}
	writeJSON(w, http.StatusOK, items)
}

func (h Handler) CreateRole(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var input createRoleRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Code) == "" || strings.TrimSpace(input.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code and name are required"})
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not begin role creation"})
		return
	}
	defer tx.Rollback(r.Context())
	var roleID string
	if err = tx.QueryRow(r.Context(), `INSERT INTO roles (organization_id,code,name) VALUES ($1,$2,$3) RETURNING id`, user.OrganizationID, strings.TrimSpace(input.Code), strings.TrimSpace(input.Name)).Scan(&roleID); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "role code may already exist"})
		return
	}
	for _, permission := range input.Permissions {
		if _, err = tx.Exec(r.Context(), `INSERT INTO role_permissions (role_id,permission_code) SELECT $1,code FROM permissions WHERE code=$2 ON CONFLICT DO NOTHING`, roleID, permission); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not assign role permission"})
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save role"})
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'role.created','role',$3,$4)`, user.OrganizationID, user.ID, roleID, map[string]any{"code": input.Code, "name": input.Name})
	writeJSON(w, http.StatusCreated, map[string]string{"id": roleID})
}

func (h Handler) Assign(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var input assignRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.UserID == "" || input.RoleID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id and role_id are required"})
		return
	}
	result, err := h.DB.Exec(r.Context(), `INSERT INTO user_roles (user_id,role_id) SELECT u.id,ro.id FROM users u JOIN roles ro ON ro.organization_id=u.organization_id WHERE u.id=$1 AND ro.id=$2 AND u.organization_id=$3 ON CONFLICT DO NOTHING`, input.UserID, input.RoleID, user.OrganizationID)
	if err != nil || result.RowsAffected() == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user or role does not belong to this organization"})
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'role.assigned','user',$3,$4)`, user.OrganizationID, user.ID, input.UserID, map[string]any{"role_id": input.RoleID})
	writeJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
