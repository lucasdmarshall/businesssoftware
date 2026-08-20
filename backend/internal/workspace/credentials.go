package workspace

import (
	"encoding/json"
	"net/http"
	"strings"

	"name/backend/internal/auth"
	"name/backend/internal/httpapi"
)

// GenerateCredentials creates a company-issued login for a new employee:
// username + temporary password + employee ID, assigned to a department.
// Allowed for users.manage (company People) or credentials.manage in a workspace.
func (h Handler) GenerateCredentials(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var input struct {
		DisplayName     string `json:"display_name"`
		DepartmentID    string `json:"department_id"`
		Email           string `json:"email"`
		IsHead          bool   `json:"is_head"`
		MakePrimary     bool   `json:"is_primary"`
		WorkspaceDeptID string `json:"-"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.DisplayName) == "" || strings.TrimSpace(input.DepartmentID) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "display_name and department_id are required")
		return
	}
	workspaceDeptID := r.PathValue("id")
	companyPeople := h.Auth.HasPermission(r.Context(), user, "users.manage") || h.Auth.HasPermission(r.Context(), user, "organization.manage")
	if workspaceDeptID != "" {
		if !h.canAccessModule(r.Context(), user, workspaceDeptID, "credentials", true) && !companyPeople {
			httpapi.WriteError(w, http.StatusForbidden, "forbidden", "credentials.manage required")
			return
		}
	} else if !companyPeople {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "users.manage required")
		return
	}

	var deptOrg string
	var archived bool
	err = h.DB.QueryRow(r.Context(), `
		SELECT organization_id, archived_at IS NOT NULL FROM departments WHERE id=$1`, input.DepartmentID).Scan(&deptOrg, &archived)
	if err != nil || deptOrg != user.OrganizationID || archived {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_department", "department not found")
		return
	}

	username, err := GenerateUsername(r.Context(), h.DB, user.OrganizationID, input.DisplayName)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "username_failed", "could not generate username")
		return
	}
	plainPassword, err := GeneratePassword()
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "password_failed", "could not generate password")
		return
	}
	passwordHash, err := auth.HashPassword(plainPassword)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "password_failed", err.Error())
		return
	}
	employeeID, err := NextEmployeeID(r.Context(), h.DB, user.OrganizationID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "employee_id_failed", "could not allocate employee id")
		return
	}
	email := strings.TrimSpace(strings.ToLower(input.Email))
	if email == "" {
		email = username + "@internal.local"
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "tx_failed", "could not create credentials")
		return
	}
	defer tx.Rollback(r.Context())

	var userID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO users (organization_id, email, username, display_name, password_hash, primary_department_id)
		VALUES ($1, lower($2), lower($3), $4, $5, $6)
		RETURNING id`,
		user.OrganizationID, email, username, strings.TrimSpace(input.DisplayName), passwordHash, input.DepartmentID,
	).Scan(&userID)
	if err != nil {
		httpapi.WriteError(w, http.StatusConflict, "conflict", "username or email may already exist")
		return
	}

	_ = SeedDepartmentPositions(r.Context(), h.DB, user.OrganizationID, input.DepartmentID)
	var positionID any
	posCode := "employee"
	if input.IsHead {
		posCode = "head"
	}
	var pid string
	if err := tx.QueryRow(r.Context(), `
		SELECT id FROM department_positions WHERE department_id=$1 AND code=$2`, input.DepartmentID, posCode).Scan(&pid); err == nil {
		positionID = pid
	}
	primary := true
	if !input.MakePrimary {
		primary = true // first assignment is always primary for new hires
	}
	_, err = tx.Exec(r.Context(), `
		INSERT INTO user_departments (user_id, department_id, is_primary, is_head, position_id)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (user_id, department_id) DO UPDATE
		SET is_primary=EXCLUDED.is_primary, is_head=EXCLUDED.is_head, position_id=COALESCE(EXCLUDED.position_id, user_departments.position_id)`,
		userID, input.DepartmentID, primary, input.IsHead, positionID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "membership_failed", "could not assign department")
		return
	}
	_, _ = tx.Exec(r.Context(), `
		INSERT INTO employee_profiles (user_id, organization_id, employee_code)
		VALUES ($1,$2,$3)
		ON CONFLICT (user_id) DO UPDATE SET employee_code=EXCLUDED.employee_code, updated_at=NOW()`,
		userID, user.OrganizationID, employeeID)
	if err := tx.Commit(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not save credentials")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'credentials.generated','user',$3,$4)`,
		user.OrganizationID, user.ID, userID, map[string]any{"username": username, "employee_id": employeeID, "department_id": input.DepartmentID})

	httpapi.WriteJSON(w, http.StatusCreated, map[string]any{
		"user_id":       userID,
		"display_name":  strings.TrimSpace(input.DisplayName),
		"username":      username,
		"password":      plainPassword,
		"employee_id":   employeeID,
		"email":         email,
		"department_id": input.DepartmentID,
		"message":       "Share these credentials with the employee once. The password is not stored in plain text.",
	})
}
