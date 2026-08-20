package reports

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"name/backend/internal/analytics"
	"name/backend/internal/auth"
	"name/backend/internal/httpapi"
	"name/backend/internal/notify"
)

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}

type Definition struct {
	ID          string          `json:"id"`
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	ReportType  string          `json:"report_type"`
	Description string          `json:"description"`
	Config      json.RawMessage `json:"config"`
	CreatedAt   string          `json:"created_at"`
}

type Schedule struct {
	ID             string  `json:"id"`
	DefinitionID   string  `json:"definition_id"`
	DefinitionName string  `json:"definition_name"`
	ReportType     string  `json:"report_type"`
	Cadence        string  `json:"cadence"`
	NextRunAt      string  `json:"next_run_at"`
	LastRunAt      *string `json:"last_run_at"`
	Active         bool    `json:"active"`
	CreatedAt      string  `json:"created_at"`
}

// Definitions lists (GET) or creates (POST) saved report definitions.
func (h Handler) Definitions(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT id, code, name, report_type, description, config, created_at::text
			FROM report_definitions WHERE organization_id=$1 ORDER BY name`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load reports")
			return
		}
		defer rows.Close()
		items := make([]Definition, 0)
		for rows.Next() {
			var item Definition
			if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.ReportType, &item.Description, &item.Config, &item.CreatedAt); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read reports")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}

	var input struct {
		Code        string          `json:"code"`
		Name        string          `json:"name"`
		ReportType  string          `json:"report_type"`
		Description string          `json:"description"`
		Config      json.RawMessage `json:"config"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Code) == "" || strings.TrimSpace(input.Name) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "code and name are required")
		return
	}
	switch input.ReportType {
	case "spending", "attendance", "sales", "custom":
	default:
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "report_type must be spending, attendance, sales, or custom")
		return
	}
	if len(input.Config) == 0 {
		input.Config = json.RawMessage(`{}`)
	}
	var created Definition
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO report_definitions (organization_id, code, name, report_type, description, config, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, code, name, report_type, description, config, created_at::text`,
		user.OrganizationID, strings.ToLower(strings.TrimSpace(input.Code)), strings.TrimSpace(input.Name), input.ReportType, strings.TrimSpace(input.Description), input.Config, user.ID,
	).Scan(&created.ID, &created.Code, &created.Name, &created.ReportType, &created.Description, &created.Config, &created.CreatedAt)
	if err != nil {
		httpapi.WriteError(w, http.StatusConflict, "create_failed", "could not create report (code may already exist)")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'report.created','report_definition',$3,$4)`,
		user.OrganizationID, user.ID, created.ID, map[string]any{"code": created.Code, "report_type": created.ReportType})
	httpapi.WriteJSON(w, http.StatusCreated, created)
}

// Schedules lists (GET) or creates (POST) scheduled report runs.
func (h Handler) Schedules(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT s.id, s.definition_id, d.name, d.report_type, s.cadence, s.next_run_at::text, s.last_run_at::text, s.active, s.created_at::text
			FROM scheduled_reports s JOIN report_definitions d ON d.id=s.definition_id
			WHERE s.organization_id=$1 ORDER BY s.next_run_at`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load schedules")
			return
		}
		defer rows.Close()
		items := make([]Schedule, 0)
		for rows.Next() {
			var item Schedule
			var last *string
			if err := rows.Scan(&item.ID, &item.DefinitionID, &item.DefinitionName, &item.ReportType, &item.Cadence, &item.NextRunAt, &last, &item.Active, &item.CreatedAt); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read schedules")
				return
			}
			item.LastRunAt = last
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}

	var input struct {
		DefinitionID string `json:"definition_id"`
		Cadence      string `json:"cadence"`
		NextRunAt    string `json:"next_run_at"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.DefinitionID == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "definition_id is required")
		return
	}
	switch input.Cadence {
	case "daily", "weekly", "monthly":
	default:
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "cadence must be daily, weekly, or monthly")
		return
	}
	nextRun := time.Now().UTC().Add(24 * time.Hour)
	if strings.TrimSpace(input.NextRunAt) != "" {
		parsed, err := time.Parse(time.RFC3339, input.NextRunAt)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "next_run_at must be RFC3339")
			return
		}
		nextRun = parsed.UTC()
	}
	var created Schedule
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO scheduled_reports (organization_id, definition_id, cadence, next_run_at, created_by)
		SELECT $1, d.id, $2, $3, $4 FROM report_definitions d WHERE d.id=$5 AND d.organization_id=$1
		RETURNING id, definition_id, cadence, next_run_at::text, active, created_at::text`,
		user.OrganizationID, input.Cadence, nextRun, user.ID, input.DefinitionID,
	).Scan(&created.ID, &created.DefinitionID, &created.Cadence, &created.NextRunAt, &created.Active, &created.CreatedAt)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create schedule")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'report.schedule_created','scheduled_report',$3,$4)`,
		user.OrganizationID, user.ID, created.ID, map[string]any{"cadence": created.Cadence})
	httpapi.WriteJSON(w, http.StatusCreated, created)
}

// Export builds a snapshot for a report definition, records an export audit
// event, and returns the payload for download/display.
func (h Handler) Export(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	id := r.PathValue("id")
	var def Definition
	if err := h.DB.QueryRow(r.Context(), `
		SELECT id, code, name, report_type, description, config, created_at::text
		FROM report_definitions WHERE id=$1 AND organization_id=$2`, id, user.OrganizationID).
		Scan(&def.ID, &def.Code, &def.Name, &def.ReportType, &def.Description, &def.Config, &def.CreatedAt); err != nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "report not found")
		return
	}

	period := "30d"
	var cfg map[string]any
	if json.Unmarshal(def.Config, &cfg) == nil {
		if p, ok := cfg["period"].(string); ok && p != "" {
			period = p
		}
	}
	from, to := analytics.PeriodBounds(time.Now().UTC(), period)
	snapshot := map[string]any{
		"report":      def,
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"period":      period,
		"from":        from,
		"to":          to,
	}

	switch def.ReportType {
	case "spending":
		var total float64
		var count int
		_ = h.DB.QueryRow(r.Context(), `
			SELECT COALESCE(SUM(amount),0), COUNT(*) FROM expenses
			WHERE organization_id=$1 AND status IN ('submitted','paid')
			  AND created_at::date BETWEEN $2::date AND $3::date`, user.OrganizationID, from, to).Scan(&total, &count)
		snapshot["summary"] = map[string]any{"total_spent": total, "expense_count": count}
	case "attendance":
		var present, completed int
		_ = h.DB.QueryRow(r.Context(), `
			SELECT COUNT(*) FILTER (WHERE status='present'), COUNT(*) FILTER (WHERE check_in_at IS NOT NULL AND check_out_at IS NOT NULL)
			FROM attendance_records WHERE organization_id=$1 AND work_date BETWEEN $2::date AND $3::date`,
			user.OrganizationID, from, to).Scan(&present, &completed)
		snapshot["summary"] = map[string]any{"present_days": present, "completed_days": completed}
	case "sales":
		snapshot["summary"] = map[string]any{"module_available": false, "message": "Sales/CRM module is not installed yet"}
	default:
		snapshot["summary"] = map[string]any{"config": def.Config}
	}

	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'report.exported','report_definition',$3,$4)`,
		user.OrganizationID, user.ID, def.ID, map[string]any{"code": def.Code, "report_type": def.ReportType, "period": period, "from": from, "to": to})
	httpapi.WriteJSON(w, http.StatusOK, snapshot)
}

// StartScheduleWorker runs due scheduled reports: notifies the creator and
// advances next_run_at. Full PDF delivery is deferred; the audit trail records each run.
func (h Handler) StartScheduleWorker(ctx context.Context) {
	if h.DB == nil {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := h.runDueSchedules(ctx); err != nil {
			slog.Warn("scheduled report pass failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (h Handler) runDueSchedules(ctx context.Context) error {
	rows, err := h.DB.Query(ctx, `
		SELECT s.id, s.organization_id, s.definition_id, s.cadence, s.next_run_at, d.name, COALESCE(s.created_by::text,'')
		FROM scheduled_reports s
		JOIN report_definitions d ON d.id=s.definition_id
		WHERE s.active=TRUE AND s.next_run_at <= NOW()
		ORDER BY s.next_run_at LIMIT 50`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type due struct {
		ID, OrgID, DefID, Cadence, Name, Creator string
		NextRun                                  time.Time
	}
	items := make([]due, 0)
	for rows.Next() {
		var item due
		if err := rows.Scan(&item.ID, &item.OrgID, &item.DefID, &item.Cadence, &item.NextRun, &item.Name, &item.Creator); err != nil {
			return err
		}
		items = append(items, item)
	}
	for _, item := range items {
		next := analytics.NextRunAfter(item.NextRun, item.Cadence)
		if _, err := h.DB.Exec(ctx, `UPDATE scheduled_reports SET last_run_at=NOW(), next_run_at=$2, updated_at=NOW() WHERE id=$1`, item.ID, next); err != nil {
			return err
		}
		_, _ = h.DB.Exec(ctx, `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'report.scheduled_run','scheduled_report',$3,$4)`,
			item.OrgID, nilIfEmpty(item.Creator), item.ID, map[string]any{"definition_id": item.DefID, "name": item.Name, "next_run_at": next.Format(time.RFC3339)})
		if item.Creator != "" {
			_ = notify.Emit(ctx, h.DB, item.OrgID, item.Creator, "report.scheduled", "Scheduled report ready", item.Name+" is ready to export", "scheduled_report", item.ID)
		}
	}
	return nil
}

func nilIfEmpty(id string) any {
	if id == "" {
		return nil
	}
	return id
}
