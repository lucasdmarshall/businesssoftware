package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"name/backend/internal/auth"
	"name/backend/internal/notify"
)

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}

type Step struct {
	ID                string   `json:"id"`
	StepOrder         int      `json:"step_order"`
	Name              string   `json:"name"`
	ApproverRoleCode  string   `json:"approver_role_code"`
	ApproverUserID    string   `json:"approver_user_id"`
	RequiredApprovals int      `json:"required_approvals"`
	MinAmount         *float64 `json:"min_amount"`
	MaxAmount         *float64 `json:"max_amount"`
}

type Definition struct {
	ID         string `json:"id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	EntityType string `json:"entity_type"`
	Active     bool   `json:"active"`
	Steps      []Step `json:"steps"`
}

type Action struct {
	ID        string `json:"id"`
	StepOrder *int   `json:"step_order"`
	ActorName string `json:"actor_name"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

type Instance struct {
	ID               string   `json:"id"`
	DefinitionID     string   `json:"definition_id"`
	DefinitionName   string   `json:"definition_name"`
	Title            string   `json:"title"`
	EntityType       string   `json:"entity_type"`
	Amount           *float64 `json:"amount"`
	Status           string   `json:"status"`
	CurrentStepOrder *int     `json:"current_step_order"`
	CurrentStepName  string   `json:"current_step_name"`
	SubmittedBy      string   `json:"submitted_by"`
	SubmitterName    string   `json:"submitter_name"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
	Actions          []Action `json:"actions,omitempty"`
}

var errStepNotActionable = errors.New("step is not actionable by this user")

// Definitions lists every workflow definition with its ordered steps.
func (h Handler) Definitions(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT id, code, name, entity_type, active FROM workflow_definitions WHERE organization_id=$1 ORDER BY name`, user.OrganizationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load workflows"})
		return
	}
	defer rows.Close()
	definitions := make([]Definition, 0)
	index := map[string]int{}
	for rows.Next() {
		var d Definition
		if err := rows.Scan(&d.ID, &d.Code, &d.Name, &d.EntityType, &d.Active); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read workflows"})
			return
		}
		d.Steps = make([]Step, 0)
		index[d.ID] = len(definitions)
		definitions = append(definitions, d)
	}
	if len(definitions) == 0 {
		writeJSON(w, http.StatusOK, definitions)
		return
	}
	stepRows, err := h.DB.Query(r.Context(), `SELECT s.id, s.definition_id, s.step_order, s.name, COALESCE(s.approver_role_code,''), COALESCE(s.approver_user_id::text,''), s.required_approvals, s.min_amount, s.max_amount FROM workflow_steps s JOIN workflow_definitions d ON d.id=s.definition_id WHERE d.organization_id=$1 ORDER BY s.step_order`, user.OrganizationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load workflow steps"})
		return
	}
	defer stepRows.Close()
	for stepRows.Next() {
		var definitionID string
		var s Step
		if err := stepRows.Scan(&s.ID, &definitionID, &s.StepOrder, &s.Name, &s.ApproverRoleCode, &s.ApproverUserID, &s.RequiredApprovals, &s.MinAmount, &s.MaxAmount); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read workflow steps"})
			return
		}
		if position, ok := index[definitionID]; ok {
			definitions[position].Steps = append(definitions[position].Steps, s)
		}
	}
	writeJSON(w, http.StatusOK, definitions)
}

type createDefinitionRequest struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	EntityType string `json:"entity_type"`
	Steps      []struct {
		Name              string   `json:"name"`
		ApproverRoleCode  string   `json:"approver_role_code"`
		ApproverUserID    string   `json:"approver_user_id"`
		RequiredApprovals int      `json:"required_approvals"`
		MinAmount         *float64 `json:"min_amount"`
		MaxAmount         *float64 `json:"max_amount"`
	} `json:"steps"`
}

// CreateDefinition stores a workflow template and its ordered steps.
func (h Handler) CreateDefinition(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var input createDefinitionRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Code) == "" || strings.TrimSpace(input.Name) == "" || len(input.Steps) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code, name, and at least one step are required"})
		return
	}
	if input.EntityType == "" {
		input.EntityType = "generic"
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not begin workflow creation"})
		return
	}
	defer tx.Rollback(r.Context())
	var definitionID string
	if err := tx.QueryRow(r.Context(), `INSERT INTO workflow_definitions (organization_id, code, name, entity_type) VALUES ($1,$2,$3,$4) RETURNING id`, user.OrganizationID, strings.TrimSpace(input.Code), strings.TrimSpace(input.Name), input.EntityType).Scan(&definitionID); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "workflow code may already exist"})
		return
	}
	for order, step := range input.Steps {
		if strings.TrimSpace(step.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "each step needs a name"})
			return
		}
		required := step.RequiredApprovals
		if required < 1 {
			required = 1
		}
		var roleCode, userID any
		if strings.TrimSpace(step.ApproverRoleCode) != "" {
			roleCode = strings.TrimSpace(step.ApproverRoleCode)
		}
		if strings.TrimSpace(step.ApproverUserID) != "" {
			userID = strings.TrimSpace(step.ApproverUserID)
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO workflow_steps (definition_id, step_order, name, approver_role_code, approver_user_id, required_approvals, min_amount, max_amount) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, definitionID, order+1, strings.TrimSpace(step.Name), roleCode, userID, required, step.MinAmount, step.MaxAmount); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not save workflow step"})
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save workflow"})
		return
	}
	h.audit(r.Context(), user, "workflow.definition.created", "workflow_definition", definitionID, map[string]any{"code": input.Code})
	writeJSON(w, http.StatusCreated, map[string]string{"id": definitionID})
}

type createInstanceRequest struct {
	DefinitionID string   `json:"definition_id"`
	Title        string   `json:"title"`
	EntityType   string   `json:"entity_type"`
	EntityID     string   `json:"entity_id"`
	Amount       *float64 `json:"amount"`
	Submit       *bool    `json:"submit"`
}

// CreateInstance opens a request and, unless submit is false, routes it to the
// first applicable approval step immediately.
func (h Handler) CreateInstance(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var input createInstanceRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.DefinitionID == "" || strings.TrimSpace(input.Title) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "definition_id and title are required"})
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not begin request"})
		return
	}
	defer tx.Rollback(r.Context())

	var entityType string
	if err := tx.QueryRow(r.Context(), `SELECT entity_type FROM workflow_definitions WHERE id=$1 AND organization_id=$2 AND active=TRUE`, input.DefinitionID, user.OrganizationID).Scan(&entityType); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "workflow definition not found"})
		return
	}
	if input.EntityType != "" {
		entityType = input.EntityType
	}
	var entityID any
	if strings.TrimSpace(input.EntityID) != "" {
		entityID = strings.TrimSpace(input.EntityID)
	}

	var instanceID string
	if err := tx.QueryRow(r.Context(), `INSERT INTO workflow_instances (organization_id, definition_id, title, entity_type, entity_id, amount, submitted_by) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, user.OrganizationID, input.DefinitionID, strings.TrimSpace(input.Title), entityType, entityID, input.Amount, user.ID).Scan(&instanceID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not create request"})
		return
	}

	submit := input.Submit == nil || *input.Submit
	if submit {
		if err := advance(r.Context(), tx, instanceID, input.DefinitionID, input.Amount, 0, user.ID, "submit", ""); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not submit request"})
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save request"})
		return
	}
	h.audit(r.Context(), user, "workflow.submitted", "workflow_instance", instanceID, map[string]any{"title": input.Title})
	writeJSON(w, http.StatusCreated, map[string]string{"id": instanceID, "status": statusAfterSubmit(submit)})
}

func statusAfterSubmit(submitted bool) string {
	if submitted {
		return "in_review"
	}
	return "draft"
}

// advance routes an instance to the first step after afterOrder whose amount
// band includes amount. When no further step applies the instance is approved.
// It records the given action on the transition.
func advance(ctx context.Context, tx pgx.Tx, instanceID, definitionID string, amount *float64, afterOrder int, actorID, action, reason string) error {
	value := 0.0
	if amount != nil {
		value = *amount
	}
	var nextOrder int
	var nextName string
	err := tx.QueryRow(ctx, `
		SELECT step_order, name FROM workflow_steps
		WHERE definition_id=$1 AND step_order>$2
		  AND (min_amount IS NULL OR $3 >= min_amount)
		  AND (max_amount IS NULL OR $3 <= max_amount)
		ORDER BY step_order ASC LIMIT 1`, definitionID, afterOrder, value).Scan(&nextOrder, &nextName)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := tx.Exec(ctx, `UPDATE workflow_instances SET status='approved', current_step_order=NULL, updated_at=NOW() WHERE id=$1`, instanceID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO workflow_actions (instance_id, step_order, actor_id, action, reason) VALUES ($1,$2,$3,$4,$5)`, instanceID, afterOrder, actorID, action, reason)
		return err
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE workflow_instances SET status='in_review', current_step_order=$2, updated_at=NOW() WHERE id=$1`, instanceID, nextOrder); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO workflow_actions (instance_id, step_order, actor_id, action, reason) VALUES ($1,$2,$3,$4,$5)`, instanceID, afterOrder, actorID, action, reason)
	return err
}

type decisionRequest struct {
	Reason string `json:"reason"`
}

// Approve records an approval on the current step and advances the instance
// once the step has collected its required distinct approvals.
func (h Handler) Approve(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, "approve")
}

// Reject terminates the instance with a recorded reason.
func (h Handler) Reject(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, "reject")
}

func (h Handler) decide(w http.ResponseWriter, r *http.Request, decision string) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	instanceID := r.PathValue("id")
	if instanceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "instance id is required"})
		return
	}
	var input decisionRequest
	_ = json.NewDecoder(r.Body).Decode(&input)
	if decision == "reject" && strings.TrimSpace(input.Reason) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a rejection reason is required"})
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not begin decision"})
		return
	}
	defer tx.Rollback(r.Context())

	var definitionID, status, title, submittedBy string
	var currentStep *int
	var amount *float64
	if err := tx.QueryRow(r.Context(), `SELECT definition_id, status, current_step_order, amount, title, submitted_by FROM workflow_instances WHERE id=$1 AND organization_id=$2 FOR UPDATE`, instanceID, user.OrganizationID).Scan(&definitionID, &status, &currentStep, &amount, &title, &submittedBy); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "request not found"})
		return
	}
	if status != "in_review" || currentStep == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "request is not awaiting a decision"})
		return
	}

	required, canAct, err := stepAuthority(r.Context(), tx, definitionID, *currentStep, user)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not evaluate approval authority"})
		return
	}
	if !canAct {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "you are not an approver for this step"})
		return
	}

	if decision == "reject" {
		if _, err := tx.Exec(r.Context(), `UPDATE workflow_instances SET status='rejected', current_step_order=NULL, updated_at=NOW() WHERE id=$1`, instanceID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not reject request"})
			return
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO workflow_actions (instance_id, step_order, actor_id, action, reason) VALUES ($1,$2,$3,'reject',$4)`, instanceID, *currentStep, user.ID, strings.TrimSpace(input.Reason)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not record rejection"})
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save decision"})
			return
		}
		h.audit(r.Context(), user, "workflow.rejected", "workflow_instance", instanceID, map[string]any{"step": *currentStep})
		_ = notify.EmitUnlessActor(r.Context(), h.DB, user.OrganizationID, user.ID, submittedBy, "workflow.rejected", "Request rejected", title, "workflow_instance", instanceID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
		return
	}

	// Approve: one approval per actor per step.
	var already bool
	if err := tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM workflow_actions WHERE instance_id=$1 AND step_order=$2 AND actor_id=$3 AND action='approve')`, instanceID, *currentStep, user.ID).Scan(&already); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not check prior approval"})
		return
	}
	if already {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "you have already approved this step"})
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO workflow_actions (instance_id, step_order, actor_id, action, reason) VALUES ($1,$2,$3,'approve',$4)`, instanceID, *currentStep, user.ID, strings.TrimSpace(input.Reason)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not record approval"})
		return
	}
	var approvals int
	if err := tx.QueryRow(r.Context(), `SELECT COUNT(*) FROM workflow_actions WHERE instance_id=$1 AND step_order=$2 AND action='approve'`, instanceID, *currentStep).Scan(&approvals); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not count approvals"})
		return
	}
	outcome := "in_review"
	if approvals >= required {
		if err := advance(r.Context(), tx, instanceID, definitionID, amount, *currentStep, user.ID, "approve", strings.TrimSpace(input.Reason)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not advance request"})
			return
		}
		var newStatus string
		if err := tx.QueryRow(r.Context(), `SELECT status FROM workflow_instances WHERE id=$1`, instanceID).Scan(&newStatus); err == nil {
			outcome = newStatus
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save decision"})
		return
	}
	h.audit(r.Context(), user, "workflow.approved", "workflow_instance", instanceID, map[string]any{"step": *currentStep, "outcome": outcome})
	if outcome == "approved" {
		_ = notify.EmitUnlessActor(r.Context(), h.DB, user.OrganizationID, user.ID, submittedBy, "workflow.approved", "Request approved", title, "workflow_instance", instanceID)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": outcome})
}

// stepAuthority returns the step's required approval count and whether the user
// may act on it. A step with no named role or user is open to any authorized
// approver (a plain supervisor gate).
func stepAuthority(ctx context.Context, tx pgx.Tx, definitionID string, stepOrder int, user auth.SessionUser) (int, bool, error) {
	var required int
	var canAct bool
	err := tx.QueryRow(ctx, `
		SELECT s.required_approvals,
		       (s.approver_user_id = $3
		        OR (s.approver_role_code IS NULL AND s.approver_user_id IS NULL)
		        OR EXISTS (
		            SELECT 1 FROM user_roles ur JOIN roles ro ON ro.id=ur.role_id
		            WHERE ur.user_id=$3 AND ro.organization_id=$4 AND ro.code=s.approver_role_code))
		FROM workflow_steps s
		WHERE s.definition_id=$1 AND s.step_order=$2`, definitionID, stepOrder, user.ID, user.OrganizationID).Scan(&required, &canAct)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, errStepNotActionable
	}
	return required, canAct, err
}

type resubmitRequest struct {
	Reason string `json:"reason"`
}

// Resubmit reopens a rejected, cancelled, or draft request from its first step.
// Only the original submitter may resubmit.
func (h Handler) Resubmit(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	instanceID := r.PathValue("id")
	var input resubmitRequest
	_ = json.NewDecoder(r.Body).Decode(&input)

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not begin resubmission"})
		return
	}
	defer tx.Rollback(r.Context())

	var definitionID, status, submittedBy string
	var amount *float64
	if err := tx.QueryRow(r.Context(), `SELECT definition_id, status, submitted_by, amount FROM workflow_instances WHERE id=$1 AND organization_id=$2 FOR UPDATE`, instanceID, user.OrganizationID).Scan(&definitionID, &status, &submittedBy, &amount); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "request not found"})
		return
	}
	if submittedBy != user.ID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only the original submitter can resubmit"})
		return
	}
	if status != "rejected" && status != "cancelled" && status != "draft" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "only a rejected, cancelled, or draft request can be resubmitted"})
		return
	}
	if err := advance(r.Context(), tx, instanceID, definitionID, amount, 0, user.ID, "resubmit", strings.TrimSpace(input.Reason)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not resubmit request"})
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save resubmission"})
		return
	}
	h.audit(r.Context(), user, "workflow.resubmitted", "workflow_instance", instanceID, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "in_review"})
}

// Cancel withdraws a request that has not yet completed. Only the submitter may.
func (h Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	instanceID := r.PathValue("id")
	result, err := h.DB.Exec(r.Context(), `UPDATE workflow_instances SET status='cancelled', current_step_order=NULL, updated_at=NOW() WHERE id=$1 AND organization_id=$2 AND submitted_by=$3 AND status IN ('draft','in_review')`, instanceID, user.OrganizationID, user.ID)
	if err != nil || result.RowsAffected() == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request could not be cancelled"})
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO workflow_actions (instance_id, actor_id, action) VALUES ($1,$2,'cancel')`, instanceID, user.ID)
	h.audit(r.Context(), user, "workflow.cancelled", "workflow_instance", instanceID, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// Instances lists requests. ?inbox=1 narrows to items awaiting the caller's
// decision; ?mine=1 narrows to requests the caller submitted.
func (h Handler) Instances(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	filters := ""
	args := []any{user.OrganizationID}
	if r.URL.Query().Get("mine") == "1" {
		filters += ` AND i.submitted_by=$2`
		args = append(args, user.ID)
	} else if r.URL.Query().Get("inbox") == "1" {
		// Items in review whose current step the caller may act on and has not
		// already approved.
		filters += ` AND i.status='in_review' AND EXISTS (
			SELECT 1 FROM workflow_steps s
			WHERE s.definition_id=i.definition_id AND s.step_order=i.current_step_order
			  AND (s.approver_user_id=$2
			       OR (s.approver_role_code IS NULL AND s.approver_user_id IS NULL)
			       OR EXISTS (SELECT 1 FROM user_roles ur JOIN roles ro ON ro.id=ur.role_id WHERE ur.user_id=$2 AND ro.organization_id=$1 AND ro.code=s.approver_role_code))
		) AND NOT EXISTS (
			SELECT 1 FROM workflow_actions a
			WHERE a.instance_id=i.id AND a.step_order=i.current_step_order AND a.actor_id=$2 AND a.action='approve')`
		args = append(args, user.ID)
	}
	query := `
		SELECT i.id, i.definition_id, d.name, i.title, i.entity_type, i.amount, i.status, i.current_step_order,
		       COALESCE(cs.name,''), i.submitted_by, u.display_name, i.created_at::text, i.updated_at::text
		FROM workflow_instances i
		JOIN workflow_definitions d ON d.id=i.definition_id
		JOIN users u ON u.id=i.submitted_by
		LEFT JOIN workflow_steps cs ON cs.definition_id=i.definition_id AND cs.step_order=i.current_step_order
		WHERE i.organization_id=$1` + filters + ` ORDER BY i.updated_at DESC LIMIT 500`
	rows, err := h.DB.Query(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load requests"})
		return
	}
	defer rows.Close()
	items := make([]Instance, 0)
	for rows.Next() {
		var item Instance
		if err := rows.Scan(&item.ID, &item.DefinitionID, &item.DefinitionName, &item.Title, &item.EntityType, &item.Amount, &item.Status, &item.CurrentStepOrder, &item.CurrentStepName, &item.SubmittedBy, &item.SubmitterName, &item.CreatedAt, &item.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read requests"})
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}

// Instance returns one request with its complete approval history.
func (h Handler) Instance(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	instanceID := r.PathValue("id")
	var item Instance
	err = h.DB.QueryRow(r.Context(), `
		SELECT i.id, i.definition_id, d.name, i.title, i.entity_type, i.amount, i.status, i.current_step_order,
		       COALESCE(cs.name,''), i.submitted_by, u.display_name, i.created_at::text, i.updated_at::text
		FROM workflow_instances i
		JOIN workflow_definitions d ON d.id=i.definition_id
		JOIN users u ON u.id=i.submitted_by
		LEFT JOIN workflow_steps cs ON cs.definition_id=i.definition_id AND cs.step_order=i.current_step_order
		WHERE i.id=$1 AND i.organization_id=$2`, instanceID, user.OrganizationID).Scan(&item.ID, &item.DefinitionID, &item.DefinitionName, &item.Title, &item.EntityType, &item.Amount, &item.Status, &item.CurrentStepOrder, &item.CurrentStepName, &item.SubmittedBy, &item.SubmitterName, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "request not found"})
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT a.id, a.step_order, COALESCE(u.display_name,'System'), a.action, a.reason, a.created_at::text FROM workflow_actions a LEFT JOIN users u ON u.id=a.actor_id WHERE a.instance_id=$1 ORDER BY a.created_at ASC`, instanceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load history"})
		return
	}
	defer rows.Close()
	item.Actions = make([]Action, 0)
	for rows.Next() {
		var a Action
		if err := rows.Scan(&a.ID, &a.StepOrder, &a.ActorName, &a.Action, &a.Reason, &a.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read history"})
			return
		}
		item.Actions = append(item.Actions, a)
	}
	writeJSON(w, http.StatusOK, item)
}

func (h Handler) audit(ctx context.Context, user auth.SessionUser, action, entityType, entityID string, metadata map[string]any) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	_, _ = h.DB.Exec(ctx, `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,$3,$4,$5,$6)`, user.OrganizationID, user.ID, action, entityType, entityID, metadata)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
