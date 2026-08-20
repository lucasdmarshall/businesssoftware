package workflow

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// syncLinkedEntity updates module records when a workflow reaches a terminal
// approval state. Leave is synced here; expenses remain submitted until paid.
func syncLinkedEntity(ctx context.Context, tx pgx.Tx, instanceID, status, actorID string) error {
	var orgID, entityType string
	var entityID *string
	if err := tx.QueryRow(ctx, `SELECT organization_id, entity_type, entity_id::text FROM workflow_instances WHERE id=$1`, instanceID).Scan(&orgID, &entityType, &entityID); err != nil {
		return err
	}
	if entityID == nil || *entityID == "" {
		return nil
	}
	switch entityType {
	case "leave":
		if status != "approved" && status != "rejected" {
			return nil
		}
		_, err := tx.Exec(ctx, `
			UPDATE leave_requests SET status=$1, reviewed_by=$2, updated_at=NOW()
			WHERE id=$3 AND organization_id=$4 AND status='pending'`, status, actorID, *entityID, orgID)
		return err
	default:
		return nil
	}
}
