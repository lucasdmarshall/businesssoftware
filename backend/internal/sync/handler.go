package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"name/backend/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}

type Operation struct {
	ID        string          `json:"id"`
	Entity    string          `json:"entity"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

type pushRequest struct {
	Operations []Operation `json:"operations"`
}

type pushResponse struct {
	Accepted   []string `json:"accepted"`
	Duplicates []string `json:"duplicates"`
	Rejected   []string `json:"rejected"`
	Conflicts  []string `json:"conflicts"`
}

var ErrConflict = errors.New("sync conflict")

func (h Handler) Push(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database is not configured"})
		return
	}

	var input pushRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sync request"})
		return
	}
	if len(input.Operations) > 500 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sync batch is too large"})
		return
	}

	result := pushResponse{Accepted: make([]string, 0), Duplicates: make([]string, 0), Rejected: make([]string, 0), Conflicts: make([]string, 0)}
	for _, operation := range input.Operations {
		if operation.ID == "" || operation.Entity == "" || operation.Action == "" || len(operation.Payload) == 0 {
			result.Rejected = append(result.Rejected, operation.ID)
			continue
		}
		if operation.CreatedAt.IsZero() {
			operation.CreatedAt = time.Now().UTC()
		}
		var exists bool
		if err := h.DB.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM sync_operations WHERE organization_id = $1 AND operation_id = $2)`, user.OrganizationID, operation.ID).Scan(&exists); err != nil {
			result.Rejected = append(result.Rejected, operation.ID)
			continue
		}
		if exists {
			result.Duplicates = append(result.Duplicates, operation.ID)
			continue
		}
		tx, err := h.DB.Begin(r.Context())
		if err != nil {
			result.Rejected = append(result.Rejected, operation.ID)
			continue
		}
		if operation.Entity == "task" {
			if err := applyTask(tx, user.OrganizationID, user.ID, operation); err != nil {
				_ = tx.Rollback(r.Context())
				if errors.Is(err, ErrConflict) {
					result.Conflicts = append(result.Conflicts, operation.ID)
				} else {
					result.Rejected = append(result.Rejected, operation.ID)
				}
				continue
			}
		}
		if operation.Entity == "attendance" {
			if err := applyAttendance(tx, user.OrganizationID, user.ID, operation); err != nil {
				_ = tx.Rollback(r.Context())
				result.Rejected = append(result.Rejected, operation.ID)
				continue
			}
		}
		if operation.Entity == "leave" {
			if err := applyLeave(tx, user.OrganizationID, user.ID, operation); err != nil {
				_ = tx.Rollback(r.Context())
				result.Rejected = append(result.Rejected, operation.ID)
				continue
			}
		}
		if operation.Entity == "shift" {
			if err := applyShift(tx, user.OrganizationID, user.ID, operation); err != nil {
				_ = tx.Rollback(r.Context())
				result.Rejected = append(result.Rejected, operation.ID)
				continue
			}
		}
		var insertedID string
		err = tx.QueryRow(r.Context(), `
			INSERT INTO sync_operations (organization_id, user_id, operation_id, entity, action, payload, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (organization_id, operation_id) DO NOTHING
			RETURNING operation_id`, user.OrganizationID, user.ID, operation.ID, operation.Entity, operation.Action, operation.Payload, operation.CreatedAt).Scan(&insertedID)
		if err != nil {
			_ = tx.Rollback(r.Context())
			// A conflict is intentionally retained as a rejected operation until the
			// entity-specific resolver is implemented.
			result.Rejected = append(result.Rejected, operation.ID)
			continue
		}
		if err := tx.Commit(r.Context()); err != nil {
			result.Rejected = append(result.Rejected, operation.ID)
			continue
		}
		if insertedID == "" {
			result.Duplicates = append(result.Duplicates, operation.ID)
		} else {
			result.Accepted = append(result.Accepted, insertedID)
		}
	}
	writeJSON(w, http.StatusOK, result)
}

type shiftPayload struct {
	ID         string `json:"id"`
	AssignedTo string `json:"assigned_to"`
	Title      string `json:"title"`
	ShiftDate  string `json:"shift_date"`
	StartsAt   string `json:"starts_at"`
	EndsAt     string `json:"ends_at"`
	Note       string `json:"note"`
}

func applyShift(tx pgx.Tx, organizationID, userID string, operation Operation) error {
	if operation.Action != "create" {
		return fmt.Errorf("unsupported shift operation")
	}
	var p shiftPayload
	if err := json.Unmarshal(operation.Payload, &p); err != nil {
		return err
	}
	if p.AssignedTo == "" {
		p.AssignedTo = userID
	}
	if p.Title == "" || p.ShiftDate == "" || p.StartsAt == "" || p.EndsAt == "" {
		return fmt.Errorf("shift fields are required")
	}
	_, err := tx.Exec(context.Background(), `INSERT INTO shifts (id,organization_id,assigned_to,title,shift_date,starts_at,ends_at,note) VALUES (COALESCE(NULLIF($1,'')::uuid,gen_random_uuid()),$2,$3,$4,$5,$6,$7,$8)`, p.ID, organizationID, p.AssignedTo, p.Title, p.ShiftDate, p.StartsAt, p.EndsAt, p.Note)
	return err
}

type leavePayload struct {
	ID        string `json:"id"`
	LeaveType string `json:"leave_type"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Reason    string `json:"reason"`
}

func applyLeave(tx pgx.Tx, organizationID, userID string, operation Operation) error {
	if operation.Action != "create" {
		return fmt.Errorf("unsupported leave operation")
	}
	var payload leavePayload
	if err := json.Unmarshal(operation.Payload, &payload); err != nil {
		return err
	}
	if payload.LeaveType == "" {
		payload.LeaveType = "annual"
	}
	if payload.StartDate == "" || payload.EndDate == "" {
		return fmt.Errorf("leave dates are required")
	}
	_, err := tx.Exec(context.Background(), `INSERT INTO leave_requests (id, organization_id, requested_by, leave_type, start_date, end_date, reason) VALUES (COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7)`, payload.ID, organizationID, userID, payload.LeaveType, payload.StartDate, payload.EndDate, payload.Reason)
	return err
}

type attendancePayload struct {
	ID       string     `json:"id"`
	WorkDate string     `json:"work_date"`
	At       *time.Time `json:"at"`
	Note     string     `json:"note"`
}

func applyAttendance(tx pgx.Tx, organizationID, userID string, operation Operation) error {
	var payload attendancePayload
	if err := json.Unmarshal(operation.Payload, &payload); err != nil {
		return err
	}
	if payload.WorkDate == "" {
		payload.WorkDate = time.Now().UTC().Format("2006-01-02")
	}
	if payload.At == nil {
		now := time.Now().UTC()
		payload.At = &now
	}
	column := "check_in_at"
	if operation.Action == "check_out" {
		column = "check_out_at"
	} else if operation.Action != "check_in" {
		return fmt.Errorf("unsupported attendance operation")
	}
	_, err := tx.Exec(context.Background(), `INSERT INTO attendance_records (id, organization_id, user_id, work_date, `+column+`, note) VALUES (COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6) ON CONFLICT (organization_id, user_id, work_date) DO UPDATE SET `+column+` = EXCLUDED.`+column+`, note = EXCLUDED.note, updated_at = NOW()`, payload.ID, organizationID, userID, payload.WorkDate, payload.At, payload.Note)
	return err
}

type taskPayload struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	Status           string     `json:"status"`
	Priority         string     `json:"priority"`
	DueAt            *time.Time `json:"due_at"`
	AssignedTo       *string    `json:"assigned_to"`
	RecurrenceRule   *string    `json:"recurrence_rule"`
	RecurrenceNextAt *time.Time `json:"recurrence_next_at"`
}

func applyTask(tx pgx.Tx, organizationID, userID string, operation Operation) error {
	var payload taskPayload
	if err := json.Unmarshal(operation.Payload, &payload); err != nil {
		return err
	}
	if payload.Title == "" {
		return fmt.Errorf("task title is required")
	}
	if payload.Status == "" {
		payload.Status = "todo"
	}
	if payload.Priority == "" {
		payload.Priority = "normal"
	}
	if operation.Action == "create" {
		if payload.ID == "" {
			return tx.QueryRow(context.Background(), `INSERT INTO tasks (organization_id, created_by, title, description, status, priority, due_at, assigned_to, recurrence_rule, recurrence_next_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`, organizationID, userID, payload.Title, payload.Description, payload.Status, payload.Priority, payload.DueAt, payload.AssignedTo, payload.RecurrenceRule, payload.RecurrenceNextAt).Scan(new(string))
		}
		result, err := tx.Exec(context.Background(), `INSERT INTO tasks (id,organization_id,created_by,title,description,status,priority,due_at,assigned_to,recurrence_rule,recurrence_next_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (id) DO NOTHING`, payload.ID, organizationID, userID, payload.Title, payload.Description, payload.Status, payload.Priority, payload.DueAt, payload.AssignedTo, payload.RecurrenceRule, payload.RecurrenceNextAt)
		if err == nil && result.RowsAffected() == 0 {
			return ErrConflict
		}
		return err
	}
	if operation.Action == "update" && payload.ID != "" {
		result, err := tx.Exec(context.Background(), `UPDATE tasks SET title=$1,description=$2,status=$3,priority=$4,due_at=$5,assigned_to=$6,recurrence_rule=$7,recurrence_next_at=$8,updated_at=NOW() WHERE id=$9 AND organization_id=$10`, payload.Title, payload.Description, payload.Status, payload.Priority, payload.DueAt, payload.AssignedTo, payload.RecurrenceRule, payload.RecurrenceNextAt, payload.ID, organizationID)
		if err == nil && result.RowsAffected() == 0 {
			return ErrConflict
		}
		return err
	}
	return fmt.Errorf("unsupported task operation")
}

func (h Handler) Pull(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database is not configured"})
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT operation_id, entity, action, payload, created_at
		FROM sync_operations
		WHERE organization_id = $1
		ORDER BY created_at ASC
		LIMIT 500`, user.OrganizationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load sync changes"})
		return
	}
	defer rows.Close()
	operations := make([]Operation, 0)
	for rows.Next() {
		var operation Operation
		if err := rows.Scan(&operation.ID, &operation.Entity, &operation.Action, &operation.Payload, &operation.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read sync changes"})
			return
		}
		operations = append(operations, operation)
	}
	writeJSON(w, http.StatusOK, map[string]any{"operations": operations})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
