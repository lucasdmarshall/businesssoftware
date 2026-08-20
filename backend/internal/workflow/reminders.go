package workflow

import (
	"context"
	"log/slog"
	"time"

	"name/backend/internal/notify"
)

// StartReminderWorker polls for approaching and overdue approval deadlines and
// notifies the current step's approvers (including active delegates).
func (h Handler) StartReminderWorker(ctx context.Context) {
	if h.DB == nil {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := h.sendDueReminders(ctx); err != nil {
			slog.Warn("workflow reminder pass failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (h Handler) sendDueReminders(ctx context.Context) error {
	rows, err := h.DB.Query(ctx, `
		SELECT i.id, i.organization_id, i.title, i.current_step_order, i.due_at, i.last_reminded_at,
		       i.definition_id, COALESCE(s.name,'')
		FROM workflow_instances i
		LEFT JOIN workflow_steps s ON s.definition_id=i.definition_id AND s.step_order=i.current_step_order
		WHERE i.status='in_review' AND i.due_at IS NOT NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()

	now := time.Now().UTC()
	upcomingWindow := 24 * time.Hour
	remindEvery := 12 * time.Hour

	type pending struct {
		ID, OrgID, Title, DefinitionID, StepName string
		StepOrder                                *int
		DueAt                                    time.Time
		LastReminded                             *time.Time
	}
	items := make([]pending, 0)
	for rows.Next() {
		var item pending
		if err := rows.Scan(&item.ID, &item.OrgID, &item.Title, &item.StepOrder, &item.DueAt, &item.LastReminded, &item.DefinitionID, &item.StepName); err != nil {
			return err
		}
		items = append(items, item)
	}

	for _, item := range items {
		kind := ReminderKind(now, item.DueAt, item.LastReminded, upcomingWindow, remindEvery)
		if kind == "" || item.StepOrder == nil {
			continue
		}
		recipients, err := h.stepRecipients(ctx, item.OrgID, item.DefinitionID, *item.StepOrder)
		if err != nil {
			return err
		}
		title := "Approval deadline approaching"
		if kind == "overdue" {
			title = "Approval overdue"
		}
		body := item.Title
		if item.StepName != "" {
			body = item.Title + " · " + item.StepName
		}
		for _, recipientID := range recipients {
			_ = notify.Emit(ctx, h.DB, item.OrgID, recipientID, "workflow."+kind, title, body, "workflow_instance", item.ID)
			_, _ = h.DB.Exec(ctx, `INSERT INTO workflow_reminders (organization_id, instance_id, recipient_id, step_order, kind) VALUES ($1,$2,$3,$4,$5)`,
				item.OrgID, item.ID, recipientID, *item.StepOrder, kind)
		}
		_, _ = h.DB.Exec(ctx, `INSERT INTO workflow_actions (instance_id, step_order, action, reason) VALUES ($1,$2,'remind',$3)`,
			item.ID, *item.StepOrder, kind)
		_, _ = h.DB.Exec(ctx, `UPDATE workflow_instances SET last_reminded_at=NOW() WHERE id=$1`, item.ID)
	}
	return nil
}

func (h Handler) stepRecipients(ctx context.Context, orgID, definitionID string, stepOrder int) ([]string, error) {
	rows, err := h.DB.Query(ctx, `
		WITH principals AS (
			SELECT s.approver_user_id::text AS user_id
			FROM workflow_steps s
			WHERE s.definition_id=$1 AND s.step_order=$2 AND s.approver_user_id IS NOT NULL
			UNION
			SELECT ur.user_id::text
			FROM workflow_steps s
			JOIN roles ro ON ro.organization_id=$3 AND ro.code=s.approver_role_code
			JOIN user_roles ur ON ur.role_id=ro.id
			WHERE s.definition_id=$1 AND s.step_order=$2 AND s.approver_role_code IS NOT NULL
			UNION
			SELECT ur.user_id::text
			FROM workflow_steps s
			JOIN role_permissions rp ON rp.permission_code='workflow.act'
			JOIN roles ro ON ro.id=rp.role_id AND ro.organization_id=$3
			JOIN user_roles ur ON ur.role_id=ro.id
			WHERE s.definition_id=$1 AND s.step_order=$2
			  AND s.approver_role_code IS NULL AND s.approver_user_id IS NULL
		),
		covered AS (
			SELECT d.delegate_id::text AS user_id
			FROM principals p
			JOIN workflow_delegations d ON d.organization_id=$3 AND d.delegator_id=p.user_id::uuid
			WHERE `+activeDelegationSQL+`
		)
		SELECT DISTINCT user_id FROM (
			SELECT user_id FROM principals WHERE user_id IS NOT NULL AND user_id <> ''
			UNION
			SELECT user_id FROM covered
		) t`, definitionID, stepOrder, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	seen := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}
