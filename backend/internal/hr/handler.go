package hr

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

type Profile struct {
	UserID           string `json:"user_id"`
	DisplayName      string `json:"display_name"`
	Email            string `json:"email"`
	Phone            string `json:"phone"`
	HireDate         string `json:"hire_date"`
	EmploymentType   string `json:"employment_type"`
	EmergencyContact string `json:"emergency_contact"`
	Bio              string `json:"bio"`
	JobTitle         string `json:"job_title"`
	EmployeeCode     string `json:"employee_code"`
}

// Profiles lists employee profiles joined with the user directory.
func (h Handler) Profiles(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT u.id, u.display_name, u.email, COALESCE(p.phone,''), COALESCE(p.hire_date::text,''),
		       COALESCE(p.employment_type,'full_time'), COALESCE(p.emergency_contact,''), COALESCE(p.bio,''), COALESCE(jt.name,''), COALESCE(p.employee_code,'')
		FROM users u
		LEFT JOIN employee_profiles p ON p.user_id=u.id
		LEFT JOIN job_titles jt ON jt.id=u.job_title_id
		WHERE u.organization_id=$1 ORDER BY u.display_name`, user.OrganizationID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load profiles")
		return
	}
	defer rows.Close()
	items := make([]Profile, 0)
	for rows.Next() {
		var item Profile
		if err := rows.Scan(&item.UserID, &item.DisplayName, &item.Email, &item.Phone, &item.HireDate, &item.EmploymentType, &item.EmergencyContact, &item.Bio, &item.JobTitle, &item.EmployeeCode); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read profiles")
			return
		}
		items = append(items, item)
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

// UpsertProfile creates or updates the HR profile for a user in the org.
func (h Handler) UpsertProfile(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var input struct {
		UserID           string `json:"user_id"`
		Phone            string `json:"phone"`
		HireDate         string `json:"hire_date"`
		EmploymentType   string `json:"employment_type"`
		EmergencyContact string `json:"emergency_contact"`
		Bio              string `json:"bio"`
		EmployeeCode     string `json:"employee_code"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.UserID == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "user_id is required")
		return
	}
	if input.EmploymentType == "" {
		input.EmploymentType = "full_time"
	}
	var hireDate any
	if strings.TrimSpace(input.HireDate) != "" {
		hireDate = input.HireDate
	}
	result, err := h.DB.Exec(r.Context(), `
		INSERT INTO employee_profiles (user_id, organization_id, phone, hire_date, employment_type, emergency_contact, bio, employee_code)
		SELECT u.id, u.organization_id, $2, $3, $4, $5, $6, $7 FROM users u WHERE u.id=$1 AND u.organization_id=$8
		ON CONFLICT (user_id) DO UPDATE SET phone=EXCLUDED.phone, hire_date=EXCLUDED.hire_date, employment_type=EXCLUDED.employment_type, emergency_contact=EXCLUDED.emergency_contact, bio=EXCLUDED.bio, employee_code=EXCLUDED.employee_code, updated_at=NOW()`,
		input.UserID, strings.TrimSpace(input.Phone), hireDate, input.EmploymentType, strings.TrimSpace(input.EmergencyContact), strings.TrimSpace(input.Bio), strings.TrimSpace(input.EmployeeCode), user.OrganizationID)
	if err != nil || result.RowsAffected() == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_user", "user does not belong to this organization")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id) VALUES ($1,$2,'employee.profile_updated','user',$3)`, user.OrganizationID, user.ID, input.UserID)
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

type OnboardingTask struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
	Title    string `json:"title"`
	Status   string `json:"status"`
}

// Onboarding lists (GET) or creates (POST) onboarding tasks for new hires.
func (h Handler) Onboarding(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `SELECT o.id, o.user_id, u.display_name, o.title, o.status FROM onboarding_tasks o JOIN users u ON u.id=o.user_id WHERE o.organization_id=$1 ORDER BY u.display_name, o.created_at`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load onboarding tasks")
			return
		}
		defer rows.Close()
		items := make([]OnboardingTask, 0)
		for rows.Next() {
			var item OnboardingTask
			if err := rows.Scan(&item.ID, &item.UserID, &item.UserName, &item.Title, &item.Status); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read onboarding tasks")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		UserID string `json:"user_id"`
		Title  string `json:"title"`
		Done   bool   `json:"done"`
		ID     string `json:"id"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	// Toggle completion when an id is supplied.
	if input.ID != "" {
		status := "pending"
		if input.Done {
			status = "done"
		}
		if _, err := h.DB.Exec(r.Context(), `UPDATE onboarding_tasks SET status=$1 WHERE id=$2 AND organization_id=$3`, status, input.ID, user.OrganizationID); err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "update_failed", "could not update task")
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": status})
		return
	}
	if input.UserID == "" || strings.TrimSpace(input.Title) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "user_id and title are required")
		return
	}
	result, err := h.DB.Exec(r.Context(), `INSERT INTO onboarding_tasks (organization_id, user_id, title) SELECT $1, u.id, $3 FROM users u WHERE u.id=$2 AND u.organization_id=$1`, user.OrganizationID, input.UserID, strings.TrimSpace(input.Title))
	if err != nil || result.RowsAffected() == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_user", "user does not belong to this organization")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

type Document struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name"`
	Title     string `json:"title"`
	DocType   string `json:"doc_type"`
	CreatedAt string `json:"created_at"`
}

// Documents lists (GET) or registers (POST) HR document metadata.
func (h Handler) Documents(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `SELECT d.id, d.user_id, u.display_name, d.title, d.doc_type, d.created_at::text FROM hr_documents d JOIN users u ON u.id=d.user_id WHERE d.organization_id=$1 ORDER BY d.created_at DESC LIMIT 500`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load documents")
			return
		}
		defer rows.Close()
		items := make([]Document, 0)
		for rows.Next() {
			var item Document
			if err := rows.Scan(&item.ID, &item.UserID, &item.UserName, &item.Title, &item.DocType, &item.CreatedAt); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read documents")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		UserID  string `json:"user_id"`
		Title   string `json:"title"`
		DocType string `json:"doc_type"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.UserID == "" || strings.TrimSpace(input.Title) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "user_id and title are required")
		return
	}
	if input.DocType == "" {
		input.DocType = "general"
	}
	result, err := h.DB.Exec(r.Context(), `INSERT INTO hr_documents (organization_id, user_id, title, doc_type, created_by) SELECT $1, u.id, $3, $4, $5 FROM users u WHERE u.id=$2 AND u.organization_id=$1`, user.OrganizationID, input.UserID, strings.TrimSpace(input.Title), input.DocType, user.ID)
	if err != nil || result.RowsAffected() == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_user", "user does not belong to this organization")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}
