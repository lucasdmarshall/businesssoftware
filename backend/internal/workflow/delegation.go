package workflow

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"name/backend/internal/auth"
	"name/backend/internal/httpapi"
)

// Delegation lets one user act on approval steps assigned to another for a window.
type Delegation struct {
	ID            string     `json:"id"`
	DelegatorID   string     `json:"delegator_id"`
	DelegatorName string     `json:"delegator_name,omitempty"`
	DelegateID    string     `json:"delegate_id"`
	DelegateName  string     `json:"delegate_name,omitempty"`
	StartsAt      time.Time  `json:"starts_at"`
	EndsAt        *time.Time `json:"ends_at"`
	Reason        string     `json:"reason"`
	Active        bool       `json:"active"`
}

// Delegations lists (GET) or creates (POST) approval delegations for the caller.
func (h Handler) Delegations(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT d.id, d.delegator_id, a.display_name, d.delegate_id, b.display_name,
			       d.starts_at, d.ends_at, d.reason, d.active
			FROM workflow_delegations d
			JOIN users a ON a.id=d.delegator_id
			JOIN users b ON b.id=d.delegate_id
			WHERE d.organization_id=$1 AND (d.delegator_id=$2 OR d.delegate_id=$2)
			ORDER BY d.created_at DESC LIMIT 200`, user.OrganizationID, user.ID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load delegations")
			return
		}
		defer rows.Close()
		items := make([]Delegation, 0)
		for rows.Next() {
			var item Delegation
			if err := rows.Scan(&item.ID, &item.DelegatorID, &item.DelegatorName, &item.DelegateID, &item.DelegateName, &item.StartsAt, &item.EndsAt, &item.Reason, &item.Active); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read delegations")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}

	var input struct {
		DelegateID string  `json:"delegate_id"`
		StartsAt   *string `json:"starts_at"`
		EndsAt     *string `json:"ends_at"`
		Reason     string  `json:"reason"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.DelegateID) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "delegate_id is required")
		return
	}
	if input.DelegateID == user.ID {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "cannot delegate to yourself")
		return
	}
	startsAt := time.Now().UTC()
	if input.StartsAt != nil && strings.TrimSpace(*input.StartsAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*input.StartsAt))
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "starts_at must be RFC3339")
			return
		}
		startsAt = parsed.UTC()
	}
	var endsAt any
	if input.EndsAt != nil && strings.TrimSpace(*input.EndsAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*input.EndsAt))
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "ends_at must be RFC3339")
			return
		}
		endsAt = parsed.UTC()
	}

	var created Delegation
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO workflow_delegations (organization_id, delegator_id, delegate_id, starts_at, ends_at, reason)
		SELECT $1,$2,u.id,$3,$4,$5 FROM users u WHERE u.id=$6 AND u.organization_id=$1
		RETURNING id, delegator_id, delegate_id, starts_at, ends_at, reason, active`,
		user.OrganizationID, user.ID, startsAt, endsAt, strings.TrimSpace(input.Reason), input.DelegateID,
	).Scan(&created.ID, &created.DelegatorID, &created.DelegateID, &created.StartsAt, &created.EndsAt, &created.Reason, &created.Active)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "save_failed", "could not create delegation")
		return
	}
	h.audit(r.Context(), user, "workflow.delegation.created", "workflow_delegation", created.ID, map[string]any{
		"delegate_id": created.DelegateID, "ends_at": created.EndsAt,
	})
	httpapi.WriteJSON(w, http.StatusCreated, created)
}

// RevokeDelegation deactivates a delegation owned by the caller.
func (h Handler) RevokeDelegation(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	id := r.PathValue("id")
	result, err := h.DB.Exec(r.Context(), `
		UPDATE workflow_delegations SET active=FALSE
		WHERE id=$1 AND organization_id=$2 AND delegator_id=$3 AND active=TRUE`, id, user.OrganizationID, user.ID)
	if err != nil || result.RowsAffected() == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "revoke_failed", "delegation could not be revoked")
		return
	}
	h.audit(r.Context(), user, "workflow.delegation.revoked", "workflow_delegation", id, nil)
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// activeDelegationSQL is the shared predicate for an active delegation row.
const activeDelegationSQL = `d.active=TRUE AND d.starts_at<=NOW() AND (d.ends_at IS NULL OR d.ends_at>NOW())`

// stepAuthority returns the step's required approval count and whether the user
// may act on it directly or via an active delegation.
func stepAuthority(ctx context.Context, tx pgx.Tx, definitionID string, stepOrder int, user auth.SessionUser) (int, bool, error) {
	var required int
	var canAct bool
	err := tx.QueryRow(ctx, `
		SELECT s.required_approvals,
		       (
		         s.approver_user_id = $3
		         OR (s.approver_role_code IS NULL AND s.approver_user_id IS NULL)
		         OR EXISTS (
		             SELECT 1 FROM user_roles ur JOIN roles ro ON ro.id=ur.role_id
		             WHERE ur.user_id=$3 AND ro.organization_id=$4 AND ro.code=s.approver_role_code)
		         OR EXISTS (
		             SELECT 1 FROM workflow_delegations d
		             WHERE d.organization_id=$4 AND d.delegate_id=$3 AND d.delegator_id=s.approver_user_id
		               AND `+activeDelegationSQL+`)
		         OR EXISTS (
		             SELECT 1 FROM workflow_delegations d
		             JOIN user_roles ur ON ur.user_id=d.delegator_id
		             JOIN roles ro ON ro.id=ur.role_id
		             WHERE d.organization_id=$4 AND d.delegate_id=$3
		               AND ro.organization_id=$4 AND ro.code=s.approver_role_code
		               AND `+activeDelegationSQL+`)
		       )
		FROM workflow_steps s
		WHERE s.definition_id=$1 AND s.step_order=$2`, definitionID, stepOrder, user.ID, user.OrganizationID).Scan(&required, &canAct)
	if err == pgx.ErrNoRows {
		return 0, false, errStepNotActionable
	}
	return required, canAct, err
}

// resolveOnBehalfOf finds the primary principal the actor is covering for this step.
func resolveOnBehalfOf(ctx context.Context, tx pgx.Tx, orgID, definitionID string, stepOrder int, actorID string) string {
	var onBehalf string
	_ = tx.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT s.approver_user_id::text FROM workflow_steps s
			 WHERE s.definition_id=$1 AND s.step_order=$2
			   AND s.approver_user_id IS NOT NULL AND s.approver_user_id <> $3::uuid
			   AND EXISTS (
			       SELECT 1 FROM workflow_delegations d
			       WHERE d.organization_id=$4 AND d.delegate_id=$3 AND d.delegator_id=s.approver_user_id
			         AND `+activeDelegationSQL+`)
			 LIMIT 1),
			(SELECT d.delegator_id::text FROM workflow_steps s
			 JOIN workflow_delegations d ON d.organization_id=$4 AND d.delegate_id=$3
			 JOIN user_roles ur ON ur.user_id=d.delegator_id
			 JOIN roles ro ON ro.id=ur.role_id AND ro.organization_id=$4 AND ro.code=s.approver_role_code
			 WHERE s.definition_id=$1 AND s.step_order=$2 AND s.approver_role_code IS NOT NULL
			   AND `+activeDelegationSQL+`
			 LIMIT 1),
			''
		)`, definitionID, stepOrder, actorID, orgID).Scan(&onBehalf)
	return onBehalf
}

// inboxAuthoritySQL extends the inbox filter so delegates also see covered items.
func inboxAuthoritySQL() string {
	return ` AND i.status='in_review' AND EXISTS (
			SELECT 1 FROM workflow_steps s
			WHERE s.definition_id=i.definition_id AND s.step_order=i.current_step_order
			  AND (
			       s.approver_user_id=$2
			       OR (s.approver_role_code IS NULL AND s.approver_user_id IS NULL)
			       OR EXISTS (SELECT 1 FROM user_roles ur JOIN roles ro ON ro.id=ur.role_id WHERE ur.user_id=$2 AND ro.organization_id=$1 AND ro.code=s.approver_role_code)
			       OR EXISTS (SELECT 1 FROM workflow_delegations d WHERE d.organization_id=$1 AND d.delegate_id=$2 AND d.delegator_id=s.approver_user_id AND ` + activeDelegationSQL + `)
			       OR EXISTS (
			           SELECT 1 FROM workflow_delegations d
			           JOIN user_roles ur ON ur.user_id=d.delegator_id
			           JOIN roles ro ON ro.id=ur.role_id
			           WHERE d.organization_id=$1 AND d.delegate_id=$2 AND ro.organization_id=$1 AND ro.code=s.approver_role_code
			             AND ` + activeDelegationSQL + `)
			  )
		) AND NOT EXISTS (
			SELECT 1 FROM workflow_actions a
			WHERE a.instance_id=i.id AND a.step_order=i.current_step_order AND a.actor_id=$2 AND a.action='approve')`
}
