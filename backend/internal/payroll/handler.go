package payroll

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"name/backend/internal/auth"
	"name/backend/internal/httpapi"
)

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}

type hourSummary struct {
	UserID      string     `json:"user_id"`
	DisplayName string     `json:"display_name"`
	Days        []DayHours `json:"days"`
	TotalWorked float64    `json:"total_worked"`
	TotalOT     float64    `json:"total_overtime"`
}

// Rules lists (GET) or upserts (POST) the active organization payroll rule.
func (h Handler) Rules(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		var rule Rule
		err := h.DB.QueryRow(r.Context(), `
			SELECT id, name, standard_hours_per_day, overtime_after_hours, overtime_multiplier, weekend_multiplier, currency, active
			FROM payroll_rules WHERE organization_id=$1 AND active=TRUE ORDER BY created_at LIMIT 1`, user.OrganizationID).
			Scan(&rule.ID, &rule.Name, &rule.StandardHoursPerDay, &rule.OvertimeAfterHours, &rule.OvertimeMultiplier, &rule.WeekendMultiplier, &rule.Currency, &rule.Active)
		if err != nil {
			httpapi.WriteJSON(w, http.StatusOK, DefaultRule())
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, rule)
		return
	}

	var input Rule
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid payroll rule")
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		input.Name = "Default"
	}
	if input.StandardHoursPerDay <= 0 {
		input.StandardHoursPerDay = 8
	}
	if input.OvertimeAfterHours <= 0 {
		input.OvertimeAfterHours = input.StandardHoursPerDay
	}
	if input.OvertimeMultiplier < 1 {
		input.OvertimeMultiplier = 1.5
	}
	if input.WeekendMultiplier < 1 {
		input.WeekendMultiplier = 2
	}
	if strings.TrimSpace(input.Currency) == "" {
		input.Currency = "USD"
	}

	var saved Rule
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO payroll_rules (organization_id, name, standard_hours_per_day, overtime_after_hours, overtime_multiplier, weekend_multiplier, currency, active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,TRUE)
		ON CONFLICT (organization_id, name) DO UPDATE SET
			standard_hours_per_day=EXCLUDED.standard_hours_per_day,
			overtime_after_hours=EXCLUDED.overtime_after_hours,
			overtime_multiplier=EXCLUDED.overtime_multiplier,
			weekend_multiplier=EXCLUDED.weekend_multiplier,
			currency=EXCLUDED.currency,
			active=TRUE,
			updated_at=NOW()
		RETURNING id, name, standard_hours_per_day, overtime_after_hours, overtime_multiplier, weekend_multiplier, currency, active`,
		user.OrganizationID, input.Name, input.StandardHoursPerDay, input.OvertimeAfterHours, input.OvertimeMultiplier, input.WeekendMultiplier, input.Currency,
	).Scan(&saved.ID, &saved.Name, &saved.StandardHoursPerDay, &saved.OvertimeAfterHours, &saved.OvertimeMultiplier, &saved.WeekendMultiplier, &saved.Currency, &saved.Active)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "save_failed", "could not save payroll rule")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'payroll.rule_saved','payroll_rule',$3,$4)`,
		user.OrganizationID, user.ID, saved.ID, map[string]any{"name": saved.Name, "standard_hours": saved.StandardHoursPerDay})
	httpapi.WriteJSON(w, http.StatusOK, saved)
}

// HoursSummary computes worked/regular/overtime hours for the authenticated
// user's recent attendance (or organization-wide for managers via ?scope=org).
func (h Handler) HoursSummary(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	rule := DefaultRule()
	_ = h.DB.QueryRow(r.Context(), `
		SELECT id, name, standard_hours_per_day, overtime_after_hours, overtime_multiplier, weekend_multiplier, currency, active
		FROM payroll_rules WHERE organization_id=$1 AND active=TRUE ORDER BY created_at LIMIT 1`, user.OrganizationID).
		Scan(&rule.ID, &rule.Name, &rule.StandardHoursPerDay, &rule.OvertimeAfterHours, &rule.OvertimeMultiplier, &rule.WeekendMultiplier, &rule.Currency, &rule.Active)

	scopeOrg := r.URL.Query().Get("scope") == "org"
	query := `
		SELECT a.user_id, u.display_name, a.work_date::text, a.check_in_at, a.check_out_at
		FROM attendance_records a JOIN users u ON u.id=a.user_id
		WHERE a.organization_id=$1 AND a.user_id=$2
		ORDER BY a.work_date DESC LIMIT 60`
	args := []any{user.OrganizationID, user.ID}
	if scopeOrg {
		query = `
			SELECT a.user_id, u.display_name, a.work_date::text, a.check_in_at, a.check_out_at
			FROM attendance_records a JOIN users u ON u.id=a.user_id
			WHERE a.organization_id=$1
			ORDER BY a.work_date DESC, u.display_name LIMIT 500`
		args = []any{user.OrganizationID}
	}

	rows, err := h.DB.Query(r.Context(), query, args...)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load attendance for hours")
		return
	}
	defer rows.Close()

	byUser := map[string]*hourSummary{}
	order := make([]string, 0)
	for rows.Next() {
		var userID, displayName, workDate string
		var checkIn, checkOut *time.Time
		if err := rows.Scan(&userID, &displayName, &workDate, &checkIn, &checkOut); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read attendance for hours")
			return
		}
		summary, ok := byUser[userID]
		if !ok {
			summary = &hourSummary{UserID: userID, DisplayName: displayName, Days: make([]DayHours, 0)}
			byUser[userID] = summary
			order = append(order, userID)
		}
		day := SplitDayHours(workDate, WorkedHours(checkIn, checkOut), rule)
		summary.Days = append(summary.Days, day)
		summary.TotalWorked = round2(summary.TotalWorked + day.WorkedHours)
		summary.TotalOT = round2(summary.TotalOT + day.OvertimeHours)
	}

	items := make([]hourSummary, 0, len(order))
	for _, id := range order {
		items = append(items, *byUser[id])
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"rule": rule, "summaries": items})
}
