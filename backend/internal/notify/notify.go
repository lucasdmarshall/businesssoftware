// Package notify is the shared platform notification service. Any module can
// emit a notification to a user; the notifications handler serves the recipient
// their own feed.
package notify

import (
	"context"

	"github.com/jackc/pgx/v5/pgconn"
)

// Querier is satisfied by both *pgxpool.Pool and pgx.Tx so callers can emit
// inside an existing transaction or on their own.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Emit records a notification for one recipient. It never notifies the actor
// about their own action when actorID is provided and equals userID.
func Emit(ctx context.Context, db Querier, orgID, userID, kind, title, body, entityType, entityID string) error {
	if userID == "" {
		return nil
	}
	_, err := db.Exec(ctx,
		`INSERT INTO notifications (organization_id, user_id, kind, title, body, entity_type, entity_id) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		orgID, userID, kind, title, body, entityType, entityID)
	return err
}

// EmitUnlessActor emits to userID unless it is the actor performing the action.
func EmitUnlessActor(ctx context.Context, db Querier, orgID, actorID, userID, kind, title, body, entityType, entityID string) error {
	if userID == "" || userID == actorID {
		return nil
	}
	return Emit(ctx, db, orgID, userID, kind, title, body, entityType, entityID)
}
