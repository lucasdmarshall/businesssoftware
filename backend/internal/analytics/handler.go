package analytics

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"name/backend/internal/auth"
	"name/backend/internal/httpapi"
)

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}

type CategoryTotal struct {
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
	Count    int     `json:"count"`
}

type SpendingSummary struct {
	Period       string          `json:"period"`
	From         string          `json:"from"`
	To           string          `json:"to"`
	TotalSpent   float64         `json:"total_spent"`
	ExpenseCount int             `json:"expense_count"`
	PaidCount    int             `json:"paid_count"`
	ByCategory   []CategoryTotal `json:"by_category"`
}

type AttendanceSummary struct {
	Period        string  `json:"period"`
	From          string  `json:"from"`
	To            string  `json:"to"`
	PresentDays   int     `json:"present_days"`
	RemoteDays    int     `json:"remote_days"`
	LeaveDays     int     `json:"leave_days"`
	AbsentDays    int     `json:"absent_days"`
	CheckedInDays int     `json:"checked_in_days"`
	CompletedDays int     `json:"completed_days"`
	TotalHours    float64 `json:"total_hours"`
	AverageHours  float64 `json:"average_hours"`
}

type SalesSummary struct {
	Period           string  `json:"period"`
	From             string  `json:"from"`
	To               string  `json:"to"`
	ModuleAvailable  bool    `json:"module_available"`
	Message          string  `json:"message"`
	LeadCount        int     `json:"lead_count"`
	OpportunityCount int     `json:"opportunity_count"`
	PipelineValue    float64 `json:"pipeline_value"`
	WonValue         float64 `json:"won_value"`
}

func (h Handler) periodFromRequest(r *http.Request) (period, from, to string) {
	period = r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}
	from = r.URL.Query().Get("from")
	to = r.URL.Query().Get("to")
	if from == "" || to == "" {
		from, to = PeriodBounds(time.Now().UTC(), period)
	}
	return period, from, to
}

// Spending summarizes submitted and paid expenses in a date window.
func (h Handler) Spending(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	period, from, to := h.periodFromRequest(r)
	summary := SpendingSummary{Period: period, From: from, To: to, ByCategory: []CategoryTotal{}}

	_ = h.DB.QueryRow(r.Context(), `
		SELECT COALESCE(SUM(amount),0), COUNT(*), COUNT(*) FILTER (WHERE status='paid')
		FROM expenses
		WHERE organization_id=$1 AND status IN ('submitted','paid')
		  AND created_at::date BETWEEN $2::date AND $3::date`,
		user.OrganizationID, from, to).Scan(&summary.TotalSpent, &summary.ExpenseCount, &summary.PaidCount)

	rows, err := h.DB.Query(r.Context(), `
		SELECT category, COALESCE(SUM(amount),0), COUNT(*)
		FROM expenses
		WHERE organization_id=$1 AND status IN ('submitted','paid')
		  AND created_at::date BETWEEN $2::date AND $3::date
		GROUP BY category ORDER BY SUM(amount) DESC`, user.OrganizationID, from, to)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var item CategoryTotal
			if rows.Scan(&item.Category, &item.Amount, &item.Count) == nil {
				summary.ByCategory = append(summary.ByCategory, item)
			}
		}
	}
	httpapi.WriteJSON(w, http.StatusOK, summary)
}

// Attendance summarizes attendance_records in a date window for the organization.
func (h Handler) Attendance(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	period, from, to := h.periodFromRequest(r)
	summary := AttendanceSummary{Period: period, From: from, To: to}

	_ = h.DB.QueryRow(r.Context(), `
		SELECT
			COUNT(*) FILTER (WHERE status='present'),
			COUNT(*) FILTER (WHERE status='remote'),
			COUNT(*) FILTER (WHERE status='leave'),
			COUNT(*) FILTER (WHERE status='absent'),
			COUNT(*) FILTER (WHERE check_in_at IS NOT NULL),
			COUNT(*) FILTER (WHERE check_in_at IS NOT NULL AND check_out_at IS NOT NULL),
			COALESCE(SUM(EXTRACT(EPOCH FROM (check_out_at - check_in_at)) / 3600.0)
			         FILTER (WHERE check_in_at IS NOT NULL AND check_out_at IS NOT NULL AND check_out_at > check_in_at), 0)
		FROM attendance_records
		WHERE organization_id=$1 AND work_date BETWEEN $2::date AND $3::date`,
		user.OrganizationID, from, to).Scan(
		&summary.PresentDays, &summary.RemoteDays, &summary.LeaveDays, &summary.AbsentDays,
		&summary.CheckedInDays, &summary.CompletedDays, &summary.TotalHours,
	)
	summary.TotalHours = float64(int(summary.TotalHours*100+0.5)) / 100
	summary.AverageHours = AverageHours(summary.TotalHours, summary.CompletedDays)
	httpapi.WriteJSON(w, http.StatusOK, summary)
}

// Sales returns a placeholder summary until the CRM module lands. The shape is
// stable so dashboards and scheduled reports can wire against it now.
func (h Handler) Sales(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	_ = user.OrganizationID
	period, from, to := h.periodFromRequest(r)
	httpapi.WriteJSON(w, http.StatusOK, SalesSummary{
		Period:          period,
		From:            from,
		To:              to,
		ModuleAvailable: false,
		Message:         "Sales/CRM module is not installed yet. Summary shape is reserved for Phase 7 CRM data.",
	})
}
