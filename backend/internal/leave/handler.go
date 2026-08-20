package leave

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"name/backend/internal/auth"
	"name/backend/internal/workflow"
)

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}

type Request struct {
	ID                 string    `json:"id"`
	RequestedBy        string    `json:"requested_by"`
	DisplayName        string    `json:"display_name,omitempty"`
	LeaveType          string    `json:"leave_type"`
	StartDate          string    `json:"start_date"`
	EndDate            string    `json:"end_date"`
	Reason             string    `json:"reason"`
	Status             string    `json:"status"`
	WorkflowInstanceID string    `json:"workflow_instance_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

type createRequest struct {
	LeaveType string `json:"leave_type"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Reason    string `json:"reason"`
}
type decisionRequest struct {
	ID string `json:"id"`
}

func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT l.id,l.requested_by,u.display_name,l.leave_type,l.start_date::text,l.end_date::text,l.reason,l.status,COALESCE(l.workflow_instance_id::text,''),l.created_at FROM leave_requests l JOIN users u ON u.id=l.requested_by WHERE l.organization_id=$1 ORDER BY l.created_at DESC LIMIT 500`, user.OrganizationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load leave requests"})
		return
	}
	defer rows.Close()
	items := make([]Request, 0)
	for rows.Next() {
		var item Request
		if err := rows.Scan(&item.ID, &item.RequestedBy, &item.DisplayName, &item.LeaveType, &item.StartDate, &item.EndDate, &item.Reason, &item.Status, &item.WorkflowInstanceID, &item.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read leave requests"})
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}

func (h Handler) Create(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var input createRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.StartDate) == "" || strings.TrimSpace(input.EndDate) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "start_date and end_date are required"})
		return
	}
	if input.LeaveType == "" {
		input.LeaveType = "annual"
	}
	var item Request
	err = h.DB.QueryRow(r.Context(), `INSERT INTO leave_requests (organization_id,requested_by,leave_type,start_date,end_date,reason) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id,requested_by,leave_type,start_date::text,end_date::text,reason,status,created_at`, user.OrganizationID, user.ID, input.LeaveType, input.StartDate, input.EndDate, input.Reason).Scan(&item.ID, &item.RequestedBy, &item.LeaveType, &item.StartDate, &item.EndDate, &item.Reason, &item.Status, &item.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not create leave request"})
		return
	}

	// Route through the generic workflow engine when a leave definition exists.
	definitionID := workflow.FindDefinitionByEntity(r.Context(), h.DB, user.OrganizationID, "leave")
	if definitionID != "" {
		title := "Leave: " + item.LeaveType + " (" + item.StartDate + " → " + item.EndDate + ")"
		instanceID, err := workflow.Start(r.Context(), h.DB, user.OrganizationID, definitionID, title, "leave", item.ID, nil, user.ID)
		if err == nil {
			_, _ = h.DB.Exec(r.Context(), `UPDATE leave_requests SET workflow_instance_id=$1 WHERE id=$2`, instanceID, item.ID)
			item.WorkflowInstanceID = instanceID
		}
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h Handler) Decide(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var input decisionRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	status := "approved"
	if strings.HasSuffix(r.URL.Path, "/reject") {
		status = "rejected"
	}

	// When a leave request is already in a live workflow, decisions must go
	// through Approvals rather than the module-local shortcut.
	var workflowStatus string
	_ = h.DB.QueryRow(r.Context(), `
		SELECT COALESCE(i.status,'') FROM leave_requests l
		LEFT JOIN workflow_instances i ON i.id=l.workflow_instance_id
		WHERE l.id=$1 AND l.organization_id=$2`, input.ID, user.OrganizationID).Scan(&workflowStatus)
	if workflowStatus == "in_review" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "this leave request is in a workflow; decide it from Approvals"})
		return
	}

	result, err := h.DB.Exec(r.Context(), `UPDATE leave_requests SET status=$1, reviewed_by=$2, updated_at=NOW() WHERE id=$3 AND organization_id=$4 AND status='pending'`, status, user.ID, input.ID, user.OrganizationID)
	if err != nil || result.RowsAffected() == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "leave request could not be updated"})
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,$3,'leave_request',$4,$5)`, user.OrganizationID, user.ID, "leave."+status, input.ID, map[string]any{"status": status})
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
