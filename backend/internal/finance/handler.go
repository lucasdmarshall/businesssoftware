package finance

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"name/backend/internal/auth"
	"name/backend/internal/httpapi"
	"name/backend/internal/workflow"
)

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}

type Vendor struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ContactEmail string `json:"contact_email"`
	ContactPhone string `json:"contact_phone"`
	Status       string `json:"status"`
}

type Expense struct {
	ID             string  `json:"id"`
	Description    string  `json:"description"`
	Category       string  `json:"category"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
	Status         string  `json:"status"`
	VendorID       string  `json:"vendor_id"`
	VendorName     string  `json:"vendor_name"`
	SubmitterName  string  `json:"submitter_name"`
	ApprovalStatus string  `json:"approval_status"`
	CreatedAt      string  `json:"created_at"`
}

// Vendors lists (GET) or creates (POST) vendors.
func (h Handler) Vendors(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `SELECT id, name, contact_email, contact_phone, status FROM vendors WHERE organization_id=$1 ORDER BY name`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load vendors")
			return
		}
		defer rows.Close()
		items := make([]Vendor, 0)
		for rows.Next() {
			var item Vendor
			if err := rows.Scan(&item.ID, &item.Name, &item.ContactEmail, &item.ContactPhone, &item.Status); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read vendors")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		Name         string `json:"name"`
		ContactEmail string `json:"contact_email"`
		ContactPhone string `json:"contact_phone"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	var created Vendor
	if err := h.DB.QueryRow(r.Context(), `INSERT INTO vendors (organization_id, name, contact_email, contact_phone, created_by) VALUES ($1,$2,$3,$4,$5) RETURNING id, name, contact_email, contact_phone, status`, user.OrganizationID, strings.TrimSpace(input.Name), strings.TrimSpace(input.ContactEmail), strings.TrimSpace(input.ContactPhone), user.ID).Scan(&created.ID, &created.Name, &created.ContactEmail, &created.ContactPhone, &created.Status); err != nil {
		httpapi.WriteError(w, http.StatusConflict, "conflict", "vendor may already exist")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, created)
}

// Expenses lists (GET) or creates (POST, as draft) expenses. The approval state
// is read live from the linked workflow instance.
func (h Handler) Expenses(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT e.id, e.description, e.category, e.amount, e.currency, e.status, COALESCE(e.vendor_id::text,''), COALESCE(v.name,''), u.display_name, COALESCE(i.status,''), e.created_at::text
			FROM expenses e
			JOIN users u ON u.id=e.submitted_by
			LEFT JOIN vendors v ON v.id=e.vendor_id
			LEFT JOIN workflow_instances i ON i.id=e.workflow_instance_id
			WHERE e.organization_id=$1 ORDER BY e.created_at DESC LIMIT 500`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load expenses")
			return
		}
		defer rows.Close()
		items := make([]Expense, 0)
		for rows.Next() {
			var item Expense
			if err := rows.Scan(&item.ID, &item.Description, &item.Category, &item.Amount, &item.Currency, &item.Status, &item.VendorID, &item.VendorName, &item.SubmitterName, &item.ApprovalStatus, &item.CreatedAt); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read expenses")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		Description string  `json:"description"`
		Category    string  `json:"category"`
		Amount      float64 `json:"amount"`
		Currency    string  `json:"currency"`
		VendorID    string  `json:"vendor_id"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Description) == "" || input.Amount <= 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "description and a positive amount are required")
		return
	}
	if input.Category == "" {
		input.Category = "general"
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	var vendorID any
	if strings.TrimSpace(input.VendorID) != "" {
		vendorID = input.VendorID
	}
	var created Expense
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO expenses (organization_id, submitted_by, vendor_id, category, description, amount, currency)
		SELECT $1,$2,$3,$4,$5,$6,$7
		WHERE $3::uuid IS NULL OR EXISTS (SELECT 1 FROM vendors v WHERE v.id=$3 AND v.organization_id=$1)
		RETURNING id, description, category, amount, currency, status, COALESCE(vendor_id::text,''), created_at::text`,
		user.OrganizationID, user.ID, vendorID, input.Category, strings.TrimSpace(input.Description), input.Amount, input.Currency).
		Scan(&created.ID, &created.Description, &created.Category, &created.Amount, &created.Currency, &created.Status, &created.VendorID, &created.CreatedAt)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create expense")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'expense.created','expense',$3,$4)`, user.OrganizationID, user.ID, created.ID, map[string]any{"amount": created.Amount})
	httpapi.WriteJSON(w, http.StatusCreated, created)
}

// Submit routes a draft expense into the finance approval workflow. If the
// organization has no expense workflow, the expense simply moves to submitted
// for manual finance handling.
func (h Handler) Submit(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	expenseID := r.PathValue("id")
	var description string
	var amount float64
	if err := h.DB.QueryRow(r.Context(), `SELECT description, amount FROM expenses WHERE id=$1 AND organization_id=$2 AND status='draft'`, expenseID, user.OrganizationID).Scan(&description, &amount); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_state", "expense not found or not a draft")
		return
	}
	definitionID := workflow.FindDefinitionByEntity(r.Context(), h.DB, user.OrganizationID, "expense")
	var instanceID any
	if definitionID != "" {
		id, err := workflow.Start(r.Context(), h.DB, user.OrganizationID, definitionID, "Expense: "+description, "expense", expenseID, &amount, user.ID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "workflow_failed", "could not start approval")
			return
		}
		instanceID = id
	}
	if _, err := h.DB.Exec(r.Context(), `UPDATE expenses SET status='submitted', workflow_instance_id=$1, updated_at=NOW() WHERE id=$2 AND organization_id=$3`, instanceID, expenseID, user.OrganizationID); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "update_failed", "could not submit expense")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id) VALUES ($1,$2,'expense.submitted','expense',$3)`, user.OrganizationID, user.ID, expenseID)
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"status": "submitted", "routed": definitionID != ""})
}

// MarkPaid closes out an approved expense. Requires finance.manage and either an
// approved workflow or no workflow routing.
func (h Handler) MarkPaid(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	expenseID := r.PathValue("id")
	result, err := h.DB.Exec(r.Context(), `
		UPDATE expenses e SET status='paid', updated_at=NOW()
		WHERE e.id=$1 AND e.organization_id=$2 AND e.status='submitted'
		  AND (e.workflow_instance_id IS NULL OR EXISTS (SELECT 1 FROM workflow_instances i WHERE i.id=e.workflow_instance_id AND i.status='approved'))`,
		expenseID, user.OrganizationID)
	if err != nil || result.RowsAffected() == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_state", "expense is not an approved, submitted expense")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id) VALUES ($1,$2,'expense.paid','expense',$3)`, user.OrganizationID, user.ID, expenseID)
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "paid"})
}

// --- Invoices ---

type Invoice struct {
	ID           string  `json:"id"`
	Number       string  `json:"number"`
	CustomerName string  `json:"customer_name"`
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
	Status       string  `json:"status"`
	CreatedAt    string  `json:"created_at"`
}

// Invoices lists (GET) or creates (POST) customer invoices.
func (h Handler) Invoices(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `SELECT id, number, customer_name, amount, currency, status, created_at::text FROM invoices WHERE organization_id=$1 ORDER BY created_at DESC LIMIT 500`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load invoices")
			return
		}
		defer rows.Close()
		items := make([]Invoice, 0)
		for rows.Next() {
			var item Invoice
			if err := rows.Scan(&item.ID, &item.Number, &item.CustomerName, &item.Amount, &item.Currency, &item.Status, &item.CreatedAt); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read invoices")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		Number       string  `json:"number"`
		CustomerName string  `json:"customer_name"`
		Amount       float64 `json:"amount"`
		Currency     string  `json:"currency"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Number) == "" || strings.TrimSpace(input.CustomerName) == "" || input.Amount < 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "number, customer_name, and amount are required")
		return
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	var created Invoice
	if err := h.DB.QueryRow(r.Context(), `INSERT INTO invoices (organization_id, number, customer_name, amount, currency, created_by) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, number, customer_name, amount, currency, status, created_at::text`, user.OrganizationID, strings.TrimSpace(input.Number), strings.TrimSpace(input.CustomerName), input.Amount, input.Currency, user.ID).Scan(&created.ID, &created.Number, &created.CustomerName, &created.Amount, &created.Currency, &created.Status, &created.CreatedAt); err != nil {
		httpapi.WriteError(w, http.StatusConflict, "conflict", "invoice number may already exist")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, created)
}

// SetInvoiceStatus moves an invoice to sent/paid/overdue/cancelled.
func (h Handler) SetInvoiceStatus(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var input struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	valid := map[string]bool{"draft": true, "sent": true, "paid": true, "overdue": true, "cancelled": true}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.ID == "" || !valid[input.Status] {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "id and a valid status are required")
		return
	}
	result, err := h.DB.Exec(r.Context(), `UPDATE invoices SET status=$1, updated_at=NOW() WHERE id=$2 AND organization_id=$3`, input.Status, input.ID, user.OrganizationID)
	if err != nil || result.RowsAffected() == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_state", "invoice not found")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": input.Status})
}

// --- Budgets ---

type Budget struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	PeriodStart string  `json:"period_start"`
	PeriodEnd   string  `json:"period_end"`
	Spent       float64 `json:"spent"`
}

// Budgets lists (GET) or creates (POST) department budgets. Spent is derived
// from paid expenses within the budget period.
func (h Handler) Budgets(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT b.id, b.name, b.amount, b.currency, b.period_start::text, b.period_end::text,
			       COALESCE((SELECT SUM(e.amount) FROM expenses e WHERE e.organization_id=b.organization_id AND e.status='paid' AND e.created_at::date BETWEEN b.period_start AND b.period_end), 0)
			FROM budgets b WHERE b.organization_id=$1 ORDER BY b.period_start DESC`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load budgets")
			return
		}
		defer rows.Close()
		items := make([]Budget, 0)
		for rows.Next() {
			var item Budget
			if err := rows.Scan(&item.ID, &item.Name, &item.Amount, &item.Currency, &item.PeriodStart, &item.PeriodEnd, &item.Spent); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read budgets")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		Name        string  `json:"name"`
		Amount      float64 `json:"amount"`
		Currency    string  `json:"currency"`
		PeriodStart string  `json:"period_start"`
		PeriodEnd   string  `json:"period_end"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" || input.PeriodStart == "" || input.PeriodEnd == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "name, period_start, and period_end are required")
		return
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	var id string
	if err := h.DB.QueryRow(r.Context(), `INSERT INTO budgets (organization_id, name, amount, currency, period_start, period_end, created_by) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, user.OrganizationID, strings.TrimSpace(input.Name), input.Amount, input.Currency, input.PeriodStart, input.PeriodEnd, user.ID).Scan(&id); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create budget")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// --- Purchase requests ---

type PurchaseRequest struct {
	ID             string  `json:"id"`
	Item           string  `json:"item"`
	Justification  string  `json:"justification"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
	Status         string  `json:"status"`
	RequesterName  string  `json:"requester_name"`
	ApprovalStatus string  `json:"approval_status"`
	CreatedAt      string  `json:"created_at"`
}

// PurchaseRequests lists (GET) or creates (POST, as draft) purchase requests.
func (h Handler) PurchaseRequests(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT p.id, p.item, p.justification, p.amount, p.currency, p.status, u.display_name, COALESCE(i.status,''), p.created_at::text
			FROM purchase_requests p JOIN users u ON u.id=p.requested_by
			LEFT JOIN workflow_instances i ON i.id=p.workflow_instance_id
			WHERE p.organization_id=$1 ORDER BY p.created_at DESC LIMIT 500`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load purchase requests")
			return
		}
		defer rows.Close()
		items := make([]PurchaseRequest, 0)
		for rows.Next() {
			var item PurchaseRequest
			if err := rows.Scan(&item.ID, &item.Item, &item.Justification, &item.Amount, &item.Currency, &item.Status, &item.RequesterName, &item.ApprovalStatus, &item.CreatedAt); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read purchase requests")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		Item          string  `json:"item"`
		Justification string  `json:"justification"`
		Amount        float64 `json:"amount"`
		Currency      string  `json:"currency"`
		VendorID      string  `json:"vendor_id"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Item) == "" || input.Amount <= 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "item and a positive amount are required")
		return
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	var vendorID any
	if strings.TrimSpace(input.VendorID) != "" {
		vendorID = input.VendorID
	}
	var id string
	if err := h.DB.QueryRow(r.Context(), `INSERT INTO purchase_requests (organization_id, requested_by, vendor_id, item, justification, amount, currency) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, user.OrganizationID, user.ID, vendorID, strings.TrimSpace(input.Item), strings.TrimSpace(input.Justification), input.Amount, input.Currency).Scan(&id); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create purchase request")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// SubmitPurchaseRequest routes a draft purchase request through the approval
// workflow, mirroring the expense flow.
func (h Handler) SubmitPurchaseRequest(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	prID := r.PathValue("id")
	var item string
	var amount float64
	if err := h.DB.QueryRow(r.Context(), `SELECT item, amount FROM purchase_requests WHERE id=$1 AND organization_id=$2 AND status='draft'`, prID, user.OrganizationID).Scan(&item, &amount); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_state", "purchase request not found or not a draft")
		return
	}
	definitionID := workflow.FindDefinitionByEntity(r.Context(), h.DB, user.OrganizationID, "purchase_request")
	var instanceID any
	if definitionID != "" {
		id, err := workflow.Start(r.Context(), h.DB, user.OrganizationID, definitionID, "Purchase: "+item, "purchase_request", prID, &amount, user.ID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "workflow_failed", "could not start approval")
			return
		}
		instanceID = id
	}
	if _, err := h.DB.Exec(r.Context(), `UPDATE purchase_requests SET status='submitted', workflow_instance_id=$1, updated_at=NOW() WHERE id=$2 AND organization_id=$3`, instanceID, prID, user.OrganizationID); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "update_failed", "could not submit purchase request")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"status": "submitted", "routed": definitionID != ""})
}
