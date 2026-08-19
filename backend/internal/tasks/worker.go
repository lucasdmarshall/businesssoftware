package tasks

import (
	"context"
	"log"
	"time"
)

// StartRecurringWorker runs the recurring-task processor for this installation.
// The worker is intentionally backend-owned so desktop clients never need to be online
// for scheduling to continue while the company server is running.
func (h Handler) StartRecurringWorker(ctx context.Context) {
	if h.DB == nil {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := h.GenerateDueTasks(ctx); err != nil {
				log.Printf("recurring task worker: %v", err)
			}
		}
	}
}

func (h Handler) GenerateDueTasks(ctx context.Context) error {
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id,organization_id,created_by,title,description,priority,assigned_to,recurrence_rule,recurrence_next_at FROM tasks WHERE recurrence_rule IN ('daily','weekly','monthly') AND recurrence_next_at IS NOT NULL AND recurrence_next_at <= NOW() ORDER BY recurrence_next_at FOR UPDATE SKIP LOCKED LIMIT 100`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type recurring struct {
		ID, OrganizationID, CreatedBy, Title, Description, Priority string
		AssignedTo                                                  *string
		Rule                                                        string
		NextAt                                                      time.Time
	}
	items := make([]recurring, 0)
	for rows.Next() {
		var item recurring
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.CreatedBy, &item.Title, &item.Description, &item.Priority, &item.AssignedTo, &item.Rule, &item.NextAt); err != nil {
			return err
		}
		items = append(items, item)
	}
	for _, item := range items {
		next := item.NextAt
		switch item.Rule {
		case "daily":
			next = next.AddDate(0, 0, 1)
		case "weekly":
			next = next.AddDate(0, 0, 7)
		case "monthly":
			next = next.AddDate(0, 1, 0)
		}
		_, err = tx.Exec(ctx, `INSERT INTO tasks (organization_id,created_by,title,description,status,priority,due_at,assigned_to,recurrence_rule,recurrence_next_at) VALUES ($1,$2,$3,$4,'todo',$5,$6,$7,$8,$9)`, item.OrganizationID, item.CreatedBy, item.Title, item.Description, item.Priority, next, item.AssignedTo, item.Rule, next)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE tasks SET recurrence_rule=NULL,recurrence_next_at=NULL,updated_at=NOW() WHERE id=$1`, item.ID)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
