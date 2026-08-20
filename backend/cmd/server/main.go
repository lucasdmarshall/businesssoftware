package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"name/backend/internal/analytics"
	"name/backend/internal/attendance"
	"name/backend/internal/audit"
	"name/backend/internal/auth"
	"name/backend/internal/calendar"
	"name/backend/internal/config"
	"name/backend/internal/crm"
	"name/backend/internal/dashboard"
	"name/backend/internal/database"
	"name/backend/internal/finance"
	"name/backend/internal/hr"
	"name/backend/internal/httpapi"
	"name/backend/internal/itops"
	"name/backend/internal/leave"
	"name/backend/internal/lookups"
	"name/backend/internal/notifications"
	"name/backend/internal/orgstructure"
	"name/backend/internal/projects"
	"name/backend/internal/rbac"
	"name/backend/internal/reports"
	"name/backend/internal/shifts"
	"name/backend/internal/sync"
	"name/backend/internal/tasks"
	"name/backend/internal/workflow"
	"name/backend/internal/workspace"

	"github.com/jackc/pgx/v5/pgxpool"
)

type healthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Version   string `json:"version"`
	Timestamp string `json:"timestamp"`
	Database  string `json:"database"`
}

type organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

type createOrganizationRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func main() {
	slog.SetDefault(httpapi.NewLogger())
	cfg := config.FromEnvironment()
	var pool *pgxpool.Pool
	if cfg.DatabaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		var err error
		pool, err = database.Open(ctx, cfg.DatabaseURL)
		cancel()
		if err != nil {
			log.Printf("database unavailable: %v", err)
		} else {
			defer pool.Close()
			log.Printf("connected to PostgreSQL")
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler(pool))
	mux.HandleFunc("GET /api/v1/health", healthHandler(pool))
	authHandler := auth.Handler{DB: pool}
	mux.Handle("GET /api/v1/organizations", authHandler.RequirePermission("organization.read", organizationsHandler(pool, authHandler)))
	mux.Handle("POST /api/v1/organizations", authHandler.RequirePermission("organization.manage", organizationsHandler(pool, authHandler)))
	mux.Handle("GET /api/v1/users", authHandler.RequirePermission("users.read", usersHandler(pool, authHandler)))
	mux.Handle("POST /api/v1/users", authHandler.RequirePermission("users.manage", usersHandler(pool, authHandler)))
	mux.Handle("GET /api/v1/departments", authHandler.RequirePermission("organization.read", departmentsHandler(pool, authHandler)))
	mux.Handle("POST /api/v1/departments", authHandler.RequirePermission("organization.manage", departmentsHandler(pool, authHandler)))
	mux.Handle("GET /api/v1/user-departments", authHandler.RequirePermission("organization.read", listUserDepartmentsHandler(pool, authHandler)))
	mux.Handle("POST /api/v1/user-departments", authHandler.RequirePermission("organization.manage", userDepartmentsHandler(pool, authHandler)))
	mux.HandleFunc("GET /api/v1/setup/status", authHandler.SetupStatus)
	mux.HandleFunc("POST /api/v1/setup", authHandler.Setup)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/v1/auth/logout", authHandler.Logout)
	mux.HandleFunc("GET /api/v1/auth/me", authHandler.Me)
	mux.HandleFunc("POST /api/v1/auth/change-password", authHandler.ChangePassword)
	mux.HandleFunc("POST /api/v1/auth/mfa/enroll", authHandler.EnrollMFA)
	mux.HandleFunc("POST /api/v1/auth/mfa/verify", authHandler.VerifyMFA)
	mux.HandleFunc("POST /api/v1/auth/mfa/disable", authHandler.DisableMFA)
	mux.Handle("POST /api/v1/users/{id}/reset-password", authHandler.RequirePermission("users.manage", http.HandlerFunc(authHandler.ResetPassword)))
	mux.Handle("POST /api/v1/users/{id}/offboard", authHandler.RequirePermission("users.manage", http.HandlerFunc(authHandler.OffboardUser)))
	mux.Handle("POST /api/v1/user-roles/unassign", authHandler.RequirePermission("roles.manage", http.HandlerFunc(authHandler.UnassignRole)))
	lookupsHandler := lookups.Handler{DB: pool, Auth: authHandler}
	mux.HandleFunc("GET /api/v1/lookups", lookupsHandler.Catalog)
	mux.HandleFunc("GET /api/v1/lookups/{category}", lookupsHandler.List)
	mux.HandleFunc("POST /api/v1/lookups/{category}", lookupsHandler.Create)
	mux.HandleFunc("PATCH /api/v1/lookup-options/{id}", lookupsHandler.Update)
	mux.HandleFunc("DELETE /api/v1/lookup-options/{id}", lookupsHandler.Delete)
	syncHandler := sync.Handler{DB: pool, Auth: authHandler}
	mux.HandleFunc("POST /api/v1/sync/push", syncHandler.Push)
	mux.HandleFunc("GET /api/v1/sync/pull", syncHandler.Pull)
	mux.HandleFunc("GET /api/v1/sync/conflicts", syncHandler.Conflicts)
	mux.HandleFunc("POST /api/v1/sync/conflicts/resolve", syncHandler.ResolveConflict)
	taskHandler := tasks.Handler{DB: pool, Auth: authHandler, StoragePath: cfg.StoragePath, ClamScanPath: cfg.ClamScanPath, RetentionDays: cfg.RetentionDays, BackupVerified: cfg.BackupVerified}
	reportsHandler := reports.Handler{DB: pool, Auth: authHandler}
	if pool != nil {
		go taskHandler.StartRecurringWorker(context.Background())
		go taskHandler.StartRetentionWorker(context.Background())
		go reportsHandler.StartScheduleWorker(context.Background())
		log.Println("started report schedule worker")
	}
	mux.Handle("GET /api/v1/tasks", authHandler.RequirePermission("tasks.read", http.HandlerFunc(taskHandler.List)))
	mux.Handle("POST /api/v1/tasks", authHandler.RequirePermission("tasks.manage", http.HandlerFunc(taskHandler.Create)))
	mux.Handle("PATCH /api/v1/tasks/{id}", authHandler.RequirePermission("tasks.manage", http.HandlerFunc(taskHandler.Update)))
	mux.Handle("DELETE /api/v1/tasks/{id}", authHandler.RequirePermission("tasks.manage", http.HandlerFunc(taskHandler.Delete)))
	mux.Handle("GET /api/v1/tasks/{id}/comments", authHandler.RequirePermission("tasks.read", http.HandlerFunc(taskHandler.Comments)))
	mux.Handle("POST /api/v1/tasks/{id}/comments", authHandler.RequirePermission("tasks.manage", http.HandlerFunc(taskHandler.Comments)))
	mux.Handle("POST /api/v1/tasks/generate-recurring", authHandler.RequirePermission("tasks.manage", http.HandlerFunc(taskHandler.GenerateRecurring)))
	mux.Handle("GET /api/v1/tasks/{id}/attachments", authHandler.RequirePermission("tasks.read", http.HandlerFunc(taskHandler.Attachments)))
	mux.Handle("POST /api/v1/tasks/{id}/attachments", authHandler.RequirePermission("tasks.manage", http.HandlerFunc(taskHandler.Attachments)))
	mux.Handle("GET /api/v1/tasks/{id}/attachments/{attachmentID}", authHandler.RequirePermission("tasks.read", http.HandlerFunc(taskHandler.DownloadAttachment)))
	attendanceHandler := attendance.Handler{DB: pool, Auth: authHandler}
	mux.Handle("GET /api/v1/attendance", authHandler.RequirePermission("attendance.read", http.HandlerFunc(attendanceHandler.List)))
	mux.Handle("GET /api/v1/attendance/settings", authHandler.RequirePermission("attendance.read", http.HandlerFunc(attendanceHandler.Settings)))
	mux.Handle("POST /api/v1/attendance/settings", authHandler.RequirePermission("attendance.read", http.HandlerFunc(attendanceHandler.Settings)))
	mux.Handle("GET /api/v1/attendance/people", authHandler.RequirePermission("attendance.manage", http.HandlerFunc(attendanceHandler.People)))
	mux.Handle("GET /api/v1/attendance/organization", authHandler.RequirePermission("attendance.manage", http.HandlerFunc(attendanceHandler.ListOrganization)))
	mux.Handle("GET /api/v1/attendance/today", authHandler.RequirePermission("attendance.manage", http.HandlerFunc(attendanceHandler.Today)))
	mux.Handle("GET /api/v1/attendance/corrections", authHandler.RequirePermission("attendance.manage", http.HandlerFunc(attendanceHandler.ListCorrections)))
	mux.Handle("POST /api/v1/attendance/corrections", authHandler.RequirePermission("attendance.manage", http.HandlerFunc(attendanceHandler.Correct)))
	mux.Handle("POST /api/v1/attendance/status", authHandler.RequirePermission("attendance.read", http.HandlerFunc(attendanceHandler.SetStatus)))
	mux.Handle("POST /api/v1/attendance/check-in", authHandler.RequirePermission("attendance.read", http.HandlerFunc(attendanceHandler.Mutate)))
	mux.Handle("POST /api/v1/attendance/check-out", authHandler.RequirePermission("attendance.read", http.HandlerFunc(attendanceHandler.Mutate)))
	leaveHandler := leave.Handler{DB: pool, Auth: authHandler}
	mux.Handle("GET /api/v1/leave", authHandler.RequirePermission("leave.read", http.HandlerFunc(leaveHandler.List)))
	mux.Handle("POST /api/v1/leave", authHandler.RequirePermission("leave.read", http.HandlerFunc(leaveHandler.Create)))
	mux.Handle("POST /api/v1/leave/approve", authHandler.RequirePermission("leave.manage", http.HandlerFunc(leaveHandler.Decide)))
	mux.Handle("POST /api/v1/leave/reject", authHandler.RequirePermission("leave.manage", http.HandlerFunc(leaveHandler.Decide)))
	mux.Handle("POST /api/v1/leave/cancel", authHandler.RequirePermission("leave.read", http.HandlerFunc(leaveHandler.Cancel)))
	mux.Handle("GET /api/v1/leave/balances", authHandler.RequirePermission("leave.read", http.HandlerFunc(leaveHandler.Balances)))
	mux.Handle("POST /api/v1/leave/balances", authHandler.RequirePermission("leave.manage", http.HandlerFunc(leaveHandler.Balances)))
	mux.Handle("POST /api/v1/leave/balances/ensure", authHandler.RequirePermission("leave.manage", http.HandlerFunc(leaveHandler.EnsureYearBalances)))
	mux.Handle("GET /api/v1/leave/policies", authHandler.RequirePermission("leave.read", http.HandlerFunc(leaveHandler.Policies)))
	mux.Handle("POST /api/v1/leave/policies", authHandler.RequirePermission("leave.manage", http.HandlerFunc(leaveHandler.Policies)))
	shiftHandler := shifts.Handler{DB: pool, Auth: authHandler}
	mux.Handle("GET /api/v1/shifts", authHandler.RequirePermission("shifts.read", http.HandlerFunc(shiftHandler.List)))
	mux.Handle("GET /api/v1/shifts/week", authHandler.RequirePermission("shifts.read", http.HandlerFunc(shiftHandler.Week)))
	mux.Handle("POST /api/v1/shifts", authHandler.RequirePermission("shifts.read", http.HandlerFunc(shiftHandler.Create)))
	mux.Handle("POST /api/v1/shifts/update", authHandler.RequirePermission("shifts.read", http.HandlerFunc(shiftHandler.Update)))
	mux.Handle("POST /api/v1/shifts/status", authHandler.RequirePermission("shifts.read", http.HandlerFunc(shiftHandler.SetStatus)))
	rbacHandler := rbac.Handler{DB: pool, Auth: authHandler}
	mux.Handle("GET /api/v1/permissions", authHandler.RequirePermission("roles.manage", http.HandlerFunc(rbacHandler.Permissions)))
	mux.Handle("GET /api/v1/roles", authHandler.RequirePermission("roles.manage", http.HandlerFunc(rbacHandler.Roles)))
	mux.Handle("POST /api/v1/roles", authHandler.RequirePermission("roles.manage", http.HandlerFunc(rbacHandler.CreateRole)))
	mux.Handle("POST /api/v1/user-roles", authHandler.RequirePermission("roles.manage", http.HandlerFunc(rbacHandler.Assign)))
	auditHandler := audit.Handler{DB: pool, Auth: authHandler}
	mux.Handle("GET /api/v1/audit-logs", authHandler.RequirePermission("organization.read", http.HandlerFunc(auditHandler.List)))
	projectHandler := projects.Handler{DB: pool, Auth: authHandler}
	mux.Handle("GET /api/v1/projects", authHandler.RequirePermission("projects.read", http.HandlerFunc(projectHandler.List)))
	mux.Handle("POST /api/v1/projects", authHandler.RequirePermission("projects.manage", http.HandlerFunc(projectHandler.Create)))
	calendarHandler := calendar.Handler{DB: pool, Auth: authHandler}
	mux.Handle("GET /api/v1/calendar/events", authHandler.RequirePermission("calendar.read", http.HandlerFunc(calendarHandler.List)))
	mux.Handle("POST /api/v1/calendar/events", authHandler.RequirePermission("calendar.manage", http.HandlerFunc(calendarHandler.Create)))
	notificationHandler := notifications.Handler{DB: pool, Auth: authHandler}
	mux.HandleFunc("GET /api/v1/notifications", notificationHandler.List)
	mux.HandleFunc("POST /api/v1/notifications/read", notificationHandler.MarkRead)
	orgStructureHandler := orgstructure.Handler{DB: pool, Auth: authHandler}
	mux.Handle("GET /api/v1/job-titles", authHandler.RequirePermission("organization.read", http.HandlerFunc(orgStructureHandler.JobTitles)))
	mux.Handle("POST /api/v1/job-titles", authHandler.RequirePermission("organization.manage", http.HandlerFunc(orgStructureHandler.JobTitles)))
	mux.Handle("POST /api/v1/user-placement", authHandler.RequirePermission("organization.manage", http.HandlerFunc(orgStructureHandler.AssignPlacement)))
	mux.Handle("GET /api/v1/reporting-lines", authHandler.RequirePermission("organization.read", http.HandlerFunc(orgStructureHandler.ReportingLines)))
	mux.Handle("POST /api/v1/reporting-lines", authHandler.RequirePermission("organization.manage", http.HandlerFunc(orgStructureHandler.ReportingLines)))
	mux.Handle("GET /api/v1/teams", authHandler.RequirePermission("organization.read", http.HandlerFunc(orgStructureHandler.Teams)))
	mux.Handle("POST /api/v1/teams", authHandler.RequirePermission("organization.manage", http.HandlerFunc(orgStructureHandler.Teams)))
	mux.Handle("POST /api/v1/user-teams", authHandler.RequirePermission("organization.manage", http.HandlerFunc(orgStructureHandler.AssignTeam)))
	mux.Handle("GET /api/v1/files", authHandler.RequirePermission("organization.read", http.HandlerFunc(orgStructureHandler.Files)))
	dashboardHandler := dashboard.Handler{DB: pool, Auth: authHandler}
	mux.HandleFunc("GET /api/v1/dashboard/summary", dashboardHandler.Summary)
	workspaceHandler := workspace.Handler{DB: pool, Auth: authHandler}
	mux.HandleFunc("GET /api/v1/workspaces/departments", workspaceHandler.List)
	mux.HandleFunc("GET /api/v1/workspaces/departments/{id}", workspaceHandler.Get)
	mux.HandleFunc("GET /api/v1/workspaces/departments/{id}/members", workspaceHandler.Members)
	mux.HandleFunc("GET /api/v1/workspaces/departments/{id}/positions", workspaceHandler.Positions)
	mux.HandleFunc("POST /api/v1/workspaces/departments/{id}/positions", workspaceHandler.Positions)
	mux.HandleFunc("POST /api/v1/workspaces/departments/{id}/positions/reorder", workspaceHandler.ReorderPositions)
	mux.HandleFunc("GET /api/v1/workspaces/departments/{id}/access", workspaceHandler.Access)
	mux.HandleFunc("POST /api/v1/workspaces/departments/{id}/access", workspaceHandler.Access)
	mux.HandleFunc("GET /api/v1/workspaces/departments/{id}/salaries", workspaceHandler.Salaries)
	mux.HandleFunc("POST /api/v1/workspaces/departments/{id}/salaries", workspaceHandler.Salaries)
	mux.HandleFunc("GET /api/v1/workspaces/departments/{id}/bonuses", workspaceHandler.Bonuses)
	mux.HandleFunc("POST /api/v1/workspaces/departments/{id}/bonuses", workspaceHandler.Bonuses)
	mux.HandleFunc("GET /api/v1/workspaces/departments/{id}/finance", workspaceHandler.Finance)
	mux.HandleFunc("POST /api/v1/workspaces/departments/{id}/finance", workspaceHandler.Finance)
	mux.HandleFunc("GET /api/v1/workspaces/departments/{id}/finance/summary", workspaceHandler.FinanceSummary)
	mux.HandleFunc("POST /api/v1/workspaces/departments/{id}/finance/status", workspaceHandler.FinanceStatus)
	financeHandler := finance.Handler{DB: pool, Auth: authHandler}
	mux.Handle("GET /api/v1/finance/overview", authHandler.RequirePermission("finance.read", http.HandlerFunc(financeHandler.Overview)))
	mux.Handle("GET /api/v1/finance/accounts", authHandler.RequirePermission("finance.read", http.HandlerFunc(financeHandler.Accounts)))
	mux.Handle("POST /api/v1/finance/accounts", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.Accounts)))
	mux.Handle("GET /api/v1/finance/tax-codes", authHandler.RequirePermission("finance.read", http.HandlerFunc(financeHandler.TaxCodes)))
	mux.Handle("POST /api/v1/finance/tax-codes", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.TaxCodes)))
	mux.Handle("GET /api/v1/finance/customers", authHandler.RequirePermission("finance.read", http.HandlerFunc(financeHandler.Customers)))
	mux.Handle("POST /api/v1/finance/customers", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.Customers)))
	mux.Handle("GET /api/v1/finance/journals", authHandler.RequirePermission("finance.read", http.HandlerFunc(financeHandler.Journals)))
	mux.Handle("POST /api/v1/finance/journals", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.Journals)))
	mux.Handle("POST /api/v1/finance/journals/{id}/post", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.PostJournal)))
	mux.Handle("GET /api/v1/finance/bills", authHandler.RequirePermission("finance.read", http.HandlerFunc(financeHandler.Bills)))
	mux.Handle("POST /api/v1/finance/bills", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.Bills)))
	mux.Handle("POST /api/v1/finance/bills/status", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.SetBillStatus)))
	mux.Handle("GET /api/v1/finance/payments", authHandler.RequirePermission("finance.read", http.HandlerFunc(financeHandler.Payments)))
	mux.Handle("POST /api/v1/finance/payments", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.Payments)))
	mux.Handle("GET /api/v1/finance/aging/ap", authHandler.RequirePermission("finance.read", http.HandlerFunc(financeHandler.AgingAP)))
	mux.Handle("GET /api/v1/finance/aging/ar", authHandler.RequirePermission("finance.read", http.HandlerFunc(financeHandler.AgingAR)))
	mux.Handle("GET /api/v1/finance/bank-accounts", authHandler.RequirePermission("finance.read", http.HandlerFunc(financeHandler.BankAccounts)))
	mux.Handle("POST /api/v1/finance/bank-accounts", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.BankAccounts)))
	mux.Handle("GET /api/v1/finance/bank-transactions", authHandler.RequirePermission("finance.read", http.HandlerFunc(financeHandler.BankTransactions)))
	mux.Handle("POST /api/v1/finance/bank-transactions", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.BankTransactions)))
	mux.Handle("POST /api/v1/finance/bank-transactions/{id}/match", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.MatchBankTransaction)))
	mux.Handle("POST /api/v1/finance/bank-transactions/{id}/unmatch", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.UnmatchBankTransaction)))
	mux.Handle("POST /api/v1/finance/bank-transactions/{id}/exclude", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.ExcludeBankTransaction)))
	mux.Handle("GET /api/v1/vendors", authHandler.RequirePermission("finance.read", http.HandlerFunc(financeHandler.Vendors)))
	mux.Handle("POST /api/v1/vendors", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.Vendors)))
	mux.Handle("GET /api/v1/expenses", authHandler.RequirePermission("finance.read", http.HandlerFunc(financeHandler.Expenses)))
	mux.Handle("POST /api/v1/expenses", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.Expenses)))
	mux.Handle("POST /api/v1/expenses/{id}/submit", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.Submit)))
	mux.Handle("POST /api/v1/expenses/{id}/pay", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.MarkPaid)))
	mux.Handle("GET /api/v1/invoices", authHandler.RequirePermission("finance.read", http.HandlerFunc(financeHandler.Invoices)))
	mux.Handle("POST /api/v1/invoices", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.Invoices)))
	mux.Handle("POST /api/v1/invoices/status", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.SetInvoiceStatus)))
	mux.Handle("GET /api/v1/budgets", authHandler.RequirePermission("finance.read", http.HandlerFunc(financeHandler.Budgets)))
	mux.Handle("POST /api/v1/budgets", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.Budgets)))
	mux.Handle("GET /api/v1/purchase-requests", authHandler.RequirePermission("finance.read", http.HandlerFunc(financeHandler.PurchaseRequests)))
	mux.Handle("POST /api/v1/purchase-requests", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.PurchaseRequests)))
	mux.Handle("POST /api/v1/purchase-requests/{id}/submit", authHandler.RequirePermission("finance.manage", http.HandlerFunc(financeHandler.SubmitPurchaseRequest)))
	hrHandler := hr.Handler{DB: pool, Auth: authHandler}
	mux.Handle("GET /api/v1/hr/profiles", authHandler.RequirePermission("hr.read", http.HandlerFunc(hrHandler.Profiles)))
	mux.Handle("POST /api/v1/hr/profiles", authHandler.RequirePermission("hr.manage", http.HandlerFunc(hrHandler.UpsertProfile)))
	mux.Handle("GET /api/v1/hr/onboarding", authHandler.RequirePermission("hr.read", http.HandlerFunc(hrHandler.Onboarding)))
	mux.Handle("POST /api/v1/hr/onboarding", authHandler.RequirePermission("hr.manage", http.HandlerFunc(hrHandler.Onboarding)))
	mux.Handle("GET /api/v1/hr/documents", authHandler.RequirePermission("hr.read", http.HandlerFunc(hrHandler.Documents)))
	mux.Handle("POST /api/v1/hr/documents", authHandler.RequirePermission("hr.manage", http.HandlerFunc(hrHandler.Documents)))
	crmHandler := crm.Handler{DB: pool, Auth: authHandler}
	mux.Handle("GET /api/v1/crm/companies", authHandler.RequirePermission("sales.read", http.HandlerFunc(crmHandler.Companies)))
	mux.Handle("POST /api/v1/crm/companies", authHandler.RequirePermission("sales.manage", http.HandlerFunc(crmHandler.Companies)))
	mux.Handle("GET /api/v1/crm/contacts", authHandler.RequirePermission("sales.read", http.HandlerFunc(crmHandler.Contacts)))
	mux.Handle("POST /api/v1/crm/contacts", authHandler.RequirePermission("sales.manage", http.HandlerFunc(crmHandler.Contacts)))
	mux.Handle("GET /api/v1/crm/leads", authHandler.RequirePermission("sales.read", http.HandlerFunc(crmHandler.Leads)))
	mux.Handle("POST /api/v1/crm/leads", authHandler.RequirePermission("sales.manage", http.HandlerFunc(crmHandler.Leads)))
	mux.Handle("POST /api/v1/crm/leads/{id}/convert", authHandler.RequirePermission("sales.manage", http.HandlerFunc(crmHandler.ConvertLead)))
	mux.Handle("GET /api/v1/crm/opportunities", authHandler.RequirePermission("sales.read", http.HandlerFunc(crmHandler.Opportunities)))
	mux.Handle("POST /api/v1/crm/opportunities", authHandler.RequirePermission("sales.manage", http.HandlerFunc(crmHandler.Opportunities)))
	mux.Handle("POST /api/v1/crm/opportunities/{id}/stage", authHandler.RequirePermission("sales.manage", http.HandlerFunc(crmHandler.MoveStage)))
	mux.Handle("GET /api/v1/crm/opportunities/{id}/history", authHandler.RequirePermission("sales.read", http.HandlerFunc(crmHandler.StageHistory)))
	mux.Handle("GET /api/v1/crm/pipeline/summary", authHandler.RequirePermission("sales.read", http.HandlerFunc(crmHandler.PipelineSummary)))
	mux.Handle("GET /api/v1/crm/activities", authHandler.RequirePermission("sales.read", http.HandlerFunc(crmHandler.Activities)))
	mux.Handle("POST /api/v1/crm/activities", authHandler.RequirePermission("sales.manage", http.HandlerFunc(crmHandler.Activities)))
	itopsHandler := itops.Handler{DB: pool, Auth: authHandler}
	mux.Handle("GET /api/v1/itops/tickets", authHandler.RequirePermission("itops.read", http.HandlerFunc(itopsHandler.Tickets)))
	mux.Handle("POST /api/v1/itops/tickets", authHandler.RequirePermission("itops.manage", http.HandlerFunc(itopsHandler.Tickets)))
	mux.Handle("GET /api/v1/itops/assets", authHandler.RequirePermission("itops.read", http.HandlerFunc(itopsHandler.Assets)))
	mux.Handle("POST /api/v1/itops/assets", authHandler.RequirePermission("itops.manage", http.HandlerFunc(itopsHandler.Assets)))
	mux.Handle("GET /api/v1/itops/kb", authHandler.RequirePermission("itops.read", http.HandlerFunc(itopsHandler.Articles)))
	mux.Handle("POST /api/v1/itops/kb", authHandler.RequirePermission("itops.manage", http.HandlerFunc(itopsHandler.Articles)))
	analyticsHandler := analytics.Handler{DB: pool, Auth: authHandler}
	mux.Handle("GET /api/v1/analytics/spending", authHandler.RequirePermission("analytics.read", http.HandlerFunc(analyticsHandler.Spending)))
	mux.Handle("GET /api/v1/analytics/attendance", authHandler.RequirePermission("analytics.read", http.HandlerFunc(analyticsHandler.Attendance)))
	mux.Handle("GET /api/v1/analytics/sales", authHandler.RequirePermission("analytics.read", http.HandlerFunc(analyticsHandler.Sales)))
	mux.Handle("GET /api/v1/reports", authHandler.RequirePermission("reports.read", http.HandlerFunc(reportsHandler.Definitions)))
	mux.Handle("POST /api/v1/reports", authHandler.RequirePermission("reports.manage", http.HandlerFunc(reportsHandler.Definitions)))
	mux.Handle("GET /api/v1/reports/schedules", authHandler.RequirePermission("reports.read", http.HandlerFunc(reportsHandler.Schedules)))
	mux.Handle("POST /api/v1/reports/schedules", authHandler.RequirePermission("reports.manage", http.HandlerFunc(reportsHandler.Schedules)))
	mux.Handle("POST /api/v1/reports/{id}/export", authHandler.RequirePermission("reports.read", http.HandlerFunc(reportsHandler.Export)))
	workflowHandler := workflow.Handler{DB: pool, Auth: authHandler}
	mux.Handle("GET /api/v1/workflow/definitions", authHandler.RequirePermission("workflow.read", http.HandlerFunc(workflowHandler.Definitions)))
	mux.Handle("POST /api/v1/workflow/definitions", authHandler.RequirePermission("workflow.manage", http.HandlerFunc(workflowHandler.CreateDefinition)))
	mux.Handle("GET /api/v1/workflow/instances", authHandler.RequirePermission("workflow.read", http.HandlerFunc(workflowHandler.Instances)))
	mux.Handle("POST /api/v1/workflow/instances", authHandler.RequirePermission("workflow.manage", http.HandlerFunc(workflowHandler.CreateInstance)))
	mux.Handle("GET /api/v1/workflow/instances/{id}", authHandler.RequirePermission("workflow.read", http.HandlerFunc(workflowHandler.Instance)))
	mux.Handle("POST /api/v1/workflow/instances/{id}/approve", authHandler.RequirePermission("workflow.act", http.HandlerFunc(workflowHandler.Approve)))
	mux.Handle("POST /api/v1/workflow/instances/{id}/reject", authHandler.RequirePermission("workflow.act", http.HandlerFunc(workflowHandler.Reject)))
	mux.Handle("POST /api/v1/workflow/instances/{id}/resubmit", authHandler.RequirePermission("workflow.manage", http.HandlerFunc(workflowHandler.Resubmit)))
	mux.Handle("POST /api/v1/workflow/instances/{id}/cancel", authHandler.RequirePermission("workflow.manage", http.HandlerFunc(workflowHandler.Cancel)))
	mux.Handle("POST /api/v1/workflow/instances/{id}/due", authHandler.RequirePermission("workflow.read", http.HandlerFunc(workflowHandler.SetDue)))
	mux.Handle("GET /api/v1/workflow/delegations", authHandler.RequirePermission("workflow.act", http.HandlerFunc(workflowHandler.Delegations)))
	mux.Handle("POST /api/v1/workflow/delegations", authHandler.RequirePermission("workflow.act", http.HandlerFunc(workflowHandler.Delegations)))
	mux.Handle("POST /api/v1/workflow/delegations/{id}/revoke", authHandler.RequirePermission("workflow.act", http.HandlerFunc(workflowHandler.RevokeDelegation)))
	go workflowHandler.StartReminderWorker(context.Background())

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.WithRequestLogging(withJSON(withCORS(mux))),
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("backend listening", "url", "http://localhost:"+cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func healthHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		databaseStatus := "not_configured"
		status := "ok"
		if pool != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			err := pool.Ping(ctx)
			cancel()
			if err == nil {
				databaseStatus = "connected"
			} else {
				databaseStatus = "unavailable"
				status = "degraded"
			}
		}

		writeJSON(w, http.StatusOK, healthResponse{
			Status:    status,
			Service:   "name-backend",
			Version:   "0.1.0-dev",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Database:  databaseStatus,
		})
	}
}

func organizationsHandler(pool *pgxpool.Pool, authHandler auth.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if pool == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database is not configured"})
			return
		}

		switch r.Method {
		case http.MethodGet:
			rows, err := pool.Query(r.Context(), `SELECT id, name, slug, created_at FROM organizations ORDER BY created_at ASC`)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load organizations"})
				return
			}
			defer rows.Close()

			organizations := make([]organization, 0)
			for rows.Next() {
				var item organization
				if err := rows.Scan(&item.ID, &item.Name, &item.Slug, &item.CreatedAt); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read organizations"})
					return
				}
				organizations = append(organizations, item)
			}
			if err := rows.Err(); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read organizations"})
				return
			}
			writeJSON(w, http.StatusOK, organizations)

		case http.MethodPost:
			if _, err := authHandler.Authenticate(r); err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
				return
			}
			var input createOrganizationRequest
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Slug) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and slug are required"})
				return
			}

			var item organization
			err := pool.QueryRow(r.Context(), `
				INSERT INTO organizations (name, slug)
				VALUES ($1, $2)
				RETURNING id, name, slug, created_at`, strings.TrimSpace(input.Name), strings.TrimSpace(input.Slug)).Scan(&item.ID, &item.Name, &item.Slug, &item.CreatedAt)
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "organization could not be created; slug may already exist"})
				return
			}
			writeJSON(w, http.StatusCreated, item)

		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}
}

type userRecord struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
}

type createUserRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

func usersHandler(pool *pgxpool.Pool, authHandler auth.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := authHandler.Authenticate(r)
		if err != nil || pool == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		if r.Method == http.MethodGet {
			rows, err := pool.Query(r.Context(), `SELECT id, email, display_name, status FROM users WHERE organization_id = $1 ORDER BY display_name`, user.OrganizationID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load users"})
				return
			}
			defer rows.Close()
			users := make([]userRecord, 0)
			for rows.Next() {
				var item userRecord
				if err := rows.Scan(&item.ID, &item.Email, &item.DisplayName, &item.Status); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read users"})
					return
				}
				users = append(users, item)
			}
			writeJSON(w, http.StatusOK, users)
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var input createUserRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Email) == "" || strings.TrimSpace(input.DisplayName) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email and display name are required"})
			return
		}
		passwordHash, err := auth.HashPassword(input.Password)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		var created userRecord
		err = pool.QueryRow(r.Context(), `INSERT INTO users (organization_id, email, display_name, password_hash) VALUES ($1, lower($2), $3, $4) RETURNING id, email, display_name, status`, user.OrganizationID, strings.TrimSpace(input.Email), strings.TrimSpace(input.DisplayName), passwordHash).Scan(&created.ID, &created.Email, &created.DisplayName, &created.Status)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "user could not be created; email may already exist"})
			return
		}
		writeJSON(w, http.StatusCreated, created)
	}
}

type departmentRecord struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type createDepartmentRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func departmentsHandler(pool *pgxpool.Pool, authHandler auth.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := authHandler.Authenticate(r)
		if err != nil || pool == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		if r.Method == http.MethodGet {
			rows, err := pool.Query(r.Context(), `SELECT id, name, slug FROM departments WHERE organization_id = $1 ORDER BY name`, user.OrganizationID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load departments"})
				return
			}
			defer rows.Close()
			departments := make([]departmentRecord, 0)
			for rows.Next() {
				var item departmentRecord
				if err := rows.Scan(&item.ID, &item.Name, &item.Slug); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read departments"})
					return
				}
				departments = append(departments, item)
			}
			writeJSON(w, http.StatusOK, departments)
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var input createDepartmentRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Slug) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and slug are required"})
			return
		}
		var created departmentRecord
		err = pool.QueryRow(r.Context(), `INSERT INTO departments (organization_id, name, slug) VALUES ($1, $2, $3) RETURNING id, name, slug`, user.OrganizationID, strings.TrimSpace(input.Name), strings.TrimSpace(input.Slug)).Scan(&created.ID, &created.Name, &created.Slug)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "department could not be created; slug may already exist"})
			return
		}
		_ = workspace.SeedDepartmentPositions(r.Context(), pool, user.OrganizationID, created.ID)
		writeJSON(w, http.StatusCreated, created)
	}
}

func listUserDepartmentsHandler(pool *pgxpool.Pool, authHandler auth.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := authHandler.Authenticate(r)
		if err != nil || pool == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		rows, err := pool.Query(r.Context(), `
			SELECT ud.user_id, u.display_name, ud.department_id, d.name, ud.is_primary, ud.is_head,
			       COALESCE(p.code,''), COALESCE(p.name,'')
			FROM user_departments ud
			JOIN users u ON u.id=ud.user_id
			JOIN departments d ON d.id=ud.department_id
			LEFT JOIN department_positions p ON p.id=ud.position_id
			WHERE u.organization_id=$1 AND d.organization_id=$1
			ORDER BY d.name, u.display_name`, user.OrganizationID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load memberships"})
			return
		}
		defer rows.Close()
		items := make([]map[string]any, 0)
		for rows.Next() {
			var userID, userName, deptID, deptName, posCode, posName string
			var primary, head bool
			if rows.Scan(&userID, &userName, &deptID, &deptName, &primary, &head, &posCode, &posName) != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read memberships"})
				return
			}
			items = append(items, map[string]any{
				"user_id": userID, "user_name": userName,
				"department_id": deptID, "department_name": deptName,
				"is_primary": primary, "is_head": head,
				"position_code": posCode, "position_name": posName,
			})
		}
		writeJSON(w, http.StatusOK, items)
	}
}

func userDepartmentsHandler(pool *pgxpool.Pool, authHandler auth.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := authHandler.Authenticate(r)
		if err != nil || pool == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		var input struct {
			UserID       string `json:"user_id"`
			DepartmentID string `json:"department_id"`
			Primary      bool   `json:"is_primary"`
			IsHead       bool   `json:"is_head"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.UserID == "" || input.DepartmentID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id and department_id are required"})
			return
		}
		result, err := pool.Exec(r.Context(), `
			INSERT INTO user_departments (user_id,department_id,is_primary,is_head)
			SELECT u.id,d.id,$3,$4 FROM users u JOIN departments d ON d.organization_id=u.organization_id
			WHERE u.id=$1 AND d.id=$2 AND u.organization_id=$5
			ON CONFLICT (user_id,department_id) DO UPDATE SET is_primary=EXCLUDED.is_primary, is_head=EXCLUDED.is_head`,
			input.UserID, input.DepartmentID, input.Primary, input.IsHead, user.OrganizationID)
		if err != nil || result.RowsAffected() == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user or department does not belong to this organization"})
			return
		}
		_ = workspace.SeedDepartmentPositions(r.Context(), pool, user.OrganizationID, input.DepartmentID)
		posCode := "employee"
		if input.IsHead {
			posCode = "head"
		}
		_, _ = pool.Exec(r.Context(), `
			UPDATE user_departments SET position_id=p.id, is_head=$3
			FROM department_positions p
			WHERE user_departments.user_id=$1 AND user_departments.department_id=$2
			  AND p.department_id=$2 AND p.code=$4`,
			input.UserID, input.DepartmentID, input.IsHead, posCode)
		writeJSON(w, http.StatusOK, map[string]any{"status": "assigned", "is_head": input.IsHead})
	}
}

func withJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write response: %v", err)
	}
}
