package finance

import (
	"encoding/json"
	"net/http"
	"strings"

	"name/backend/internal/httpapi"
	"name/backend/internal/workflow"
)

type Invoice struct {
	ID             string  `json:"id"`
	InvoiceNumber  string  `json:"invoice_number"`
	Description    string  `json:"description"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
	DueDate        string  `json:"due_date"`
	Status         string  `json:"status"`
	VendorID       string  `json:"vendor_id"`
	VendorName     string  `json:"vendor_name"`
	CreatorName    string  `json:"creator_name"`
	ApprovalStatus string  `json:"approval_status"`
	CreatedAt      string  `json:"created_at"`
}

type Budget struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	DepartmentID   string  `json:"department_id"`
	DepartmentName string  `json:"department_name"`
	PeriodStart    string  `json:"period_start"`
	PeriodEnd      string  `json:"period_end"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
	Committed      float64 `json:"committed"`
	Remaining      float64 `json:"remaining"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
}

type PurchaseRequest struct {
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
	Status         string  `json:"status"`
	VendorID       string  `json:"vendor_id"`
	VendorName     string  `json:"vendor_name"`
	RequesterName  string  `json:"requester_name"`
	ApprovalStatus string  `json:"approval_status"`
	CreatedAt      string  `json:"created_at"`
}

// Invoices lists (GET) or creates draft invoices (POST).
func (h Handler) Invoices(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT i.id, i.invoice_number, i.description, i.amount, i.currency, COALESCE(i.due_date::text,''), i.status,
			       COALESCE(i.vendor_id::text,''), COALESCE(v.name,''), u.display_name, COALESCE(w.status,''), i.created_at::text
			FROM invoices i
			JOIN users u ON u.id=i.created_by
			LEFT JOIN vendors v ON v.id=i.vendor_id
			LEFT JOIN workflow_instances w ON w.id=i.workflow_instance_id
			WHERE i.organization_id=$1 ORDER BY i.created_at DESC LIMIT 500`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load invoices")
			return
		}
		defer rows.Close()
		items := make([]Invoice, 0)
		for rows.Next() {
			var item Invoice
			if err := rows.Scan(&item.ID, &item.InvoiceNumber, &item.Description, &item.Amount, &item.Currency, &item.DueDate, &item.Status, &item.VendorID, &item.VendorName, &item.CreatorName, &item.ApprovalStatus, &item.CreatedAt); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read invoices")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}

	var input struct {
		InvoiceNumber string  `json:"invoice_number"`
		Description   string  `json:"description"`
		Amount        float64 `json:"amount"`
		Currency      string  `json:"currency"`
		DueDate       string  `json:"due_date"`
		VendorID      string  `json:"vendor_id"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.InvoiceNumber) == "" || input.Amount < 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invoice_number and a non-negative amount are required")
		return
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	var vendorID, dueDate any
	if strings.TrimSpace(input.VendorID) != "" {
		vendorID = input.VendorID
	}
	if strings.TrimSpace(input.DueDate) != "" {
		dueDate = input.DueDate
	}
	var created Invoice
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO invoices (organization_id, vendor_id, created_by, invoice_number, description, amount, currency, due_date)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8
		WHERE $2::uuid IS NULL OR EXISTS (SELECT 1 FROM vendors v WHERE v.id=$2 AND v.organization_id=$1)
		RETURNING id, invoice_number, description, amount, currency, COALESCE(due_date::text,''), status, COALESCE(vendor_id::text,''), created_at::text`,
		user.OrganizationID, vendorID, user.ID, strings.TrimSpace(input.InvoiceNumber), strings.TrimSpace(input.Description), input.Amount, input.Currency, dueDate,
	).Scan(&created.ID, &created.InvoiceNumber, &created.Description, &created.Amount, &created.Currency, &created.DueDate, &created.Status, &created.VendorID, &created.CreatedAt)
	if err != nil {
		httpapi.WriteError(w, http.StatusConflict, "create_failed", "could not create invoice (number may already exist)")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'invoice.created','invoice',$3,$4)`,
		user.OrganizationID, user.ID, created.ID, map[string]any{"invoice_number": created.InvoiceNumber, "amount": created.Amount})
	httpapi.WriteJSON(w, http.StatusCreated, created)
}

// SubmitInvoice routes a draft invoice into an invoice approval workflow when configured.
func (h Handler) SubmitInvoice(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	invoiceID := r.PathValue("id")
	var number string
	var amount float64
	if err := h.DB.QueryRow(r.Context(), `SELECT invoice_number, amount FROM invoices WHERE id=$1 AND organization_id=$2 AND status='draft'`, invoiceID, user.OrganizationID).Scan(&number, &amount); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_state", "invoice not found or not a draft")
		return
	}
	definitionID := workflow.FindDefinitionByEntity(r.Context(), h.DB, user.OrganizationID, "invoice")
	var instanceID any
	if definitionID != "" {
		id, err := workflow.Start(r.Context(), h.DB, user.OrganizationID, definitionID, "Invoice: "+number, "invoice", invoiceID, &amount, user.ID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "workflow_failed", "could not start approval")
			return
		}
		instanceID = id
	}
	if _, err := h.DB.Exec(r.Context(), `UPDATE invoices SET status='submitted', workflow_instance_id=$1, updated_at=NOW() WHERE id=$2 AND organization_id=$3`, instanceID, invoiceID, user.OrganizationID); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "update_failed", "could not submit invoice")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id) VALUES ($1,$2,'invoice.submitted','invoice',$3)`, user.OrganizationID, user.ID, invoiceID)
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"status": "submitted", "routed": definitionID != ""})
}

// PayInvoice marks an approved (or unrouted) submitted invoice as paid.
func (h Handler) PayInvoice(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	invoiceID := r.PathValue("id")
	result, err := h.DB.Exec(r.Context(), `
		UPDATE invoices i SET status='paid', updated_at=NOW()
		WHERE i.id=$1 AND i.organization_id=$2 AND i.status='submitted'
		  AND (i.workflow_instance_id IS NULL OR EXISTS (SELECT 1 FROM workflow_instances w WHERE w.id=i.workflow_instance_id AND w.status='approved'))`,
		invoiceID, user.OrganizationID)
	if err != nil || result.RowsAffected() == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_state", "invoice is not an approved, submitted invoice")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id) VALUES ($1,$2,'invoice.paid','invoice',$3)`, user.OrganizationID, user.ID, invoiceID)
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "paid"})
}

// Budgets lists (GET) or creates (POST) organization budgets with committed spend from submitted/paid expenses and purchase requests overlapping the period.
func (h Handler) Budgets(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT b.id, b.name, COALESCE(b.department_id::text,''), COALESCE(d.name,''), b.period_start::text, b.period_end::text,
			       b.amount, b.currency, b.status, b.created_at::text,
			       COALESCE((
			         SELECT SUM(x.amount) FROM (
			           SELECT e.amount FROM expenses e
			           WHERE e.organization_id=b.organization_id AND e.status IN ('submitted','paid')
			             AND e.created_at::date BETWEEN b.period_start AND b.period_end
			           UNION ALL
			           SELECT p.amount FROM purchase_requests p
			           WHERE p.organization_id=b.organization_id AND p.status IN ('submitted','ordered')
			             AND p.created_at::date BETWEEN b.period_start AND b.period_end
			         ) x
			       ), 0) AS committed
			FROM budgets b
			LEFT JOIN departments d ON d.id=b.department_id
			WHERE b.organization_id=$1
			ORDER BY b.period_start DESC, b.name LIMIT 500`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load budgets")
			return
		}
		defer rows.Close()
		items := make([]Budget, 0)
		for rows.Next() {
			var item Budget
			if err := rows.Scan(&item.ID, &item.Name, &item.DepartmentID, &item.DepartmentName, &item.PeriodStart, &item.PeriodEnd, &item.Amount, &item.Currency, &item.Status, &item.CreatedAt, &item.Committed); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read budgets")
				return
			}
			item.Remaining = Remaining(item.Amount, item.Committed)
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}

	var input struct {
		Name         string  `json:"name"`
		DepartmentID string  `json:"department_id"`
		PeriodStart  string  `json:"period_start"`
		PeriodEnd    string  `json:"period_end"`
		Amount       float64 `json:"amount"`
		Currency     string  `json:"currency"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" || input.PeriodStart == "" || input.PeriodEnd == "" || input.Amount < 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "name, period, and a non-negative amount are required")
		return
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	var departmentID any
	if strings.TrimSpace(input.DepartmentID) != "" {
		departmentID = input.DepartmentID
	}
	var created Budget
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO budgets (organization_id, name, department_id, period_start, period_end, amount, currency, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, name, COALESCE(department_id::text,''), period_start::text, period_end::text, amount, currency, status, created_at::text`,
		user.OrganizationID, strings.TrimSpace(input.Name), departmentID, input.PeriodStart, input.PeriodEnd, input.Amount, input.Currency, user.ID,
	).Scan(&created.ID, &created.Name, &created.DepartmentID, &created.PeriodStart, &created.PeriodEnd, &created.Amount, &created.Currency, &created.Status, &created.CreatedAt)
	if err != nil {
		httpapi.WriteError(w, http.StatusConflict, "create_failed", "could not create budget")
		return
	}
	created.Committed = 0
	created.Remaining = created.Amount
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'budget.created','budget',$3,$4)`,
		user.OrganizationID, user.ID, created.ID, map[string]any{"name": created.Name, "amount": created.Amount})
	httpapi.WriteJSON(w, http.StatusCreated, created)
}

// PurchaseRequests lists (GET) or creates draft purchase requests (POST).
func (h Handler) PurchaseRequests(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT p.id, p.title, p.description, p.amount, p.currency, p.status,
			       COALESCE(p.vendor_id::text,''), COALESCE(v.name,''), u.display_name, COALESCE(w.status,''), p.created_at::text
			FROM purchase_requests p
			JOIN users u ON u.id=p.requested_by
			LEFT JOIN vendors v ON v.id=p.vendor_id
			LEFT JOIN workflow_instances w ON w.id=p.workflow_instance_id
			WHERE p.organization_id=$1 ORDER BY p.created_at DESC LIMIT 500`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load purchase requests")
			return
		}
		defer rows.Close()
		items := make([]PurchaseRequest, 0)
		for rows.Next() {
			var item PurchaseRequest
			if err := rows.Scan(&item.ID, &item.Title, &item.Description, &item.Amount, &item.Currency, &item.Status, &item.VendorID, &item.VendorName, &item.RequesterName, &item.ApprovalStatus, &item.CreatedAt); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read purchase requests")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}

	var input struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Amount      float64 `json:"amount"`
		Currency    string  `json:"currency"`
		VendorID    string  `json:"vendor_id"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Title) == "" || input.Amount <= 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "title and a positive amount are required")
		return
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	var vendorID any
	if strings.TrimSpace(input.VendorID) != "" {
		vendorID = input.VendorID
	}
	var created PurchaseRequest
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO purchase_requests (organization_id, requested_by, vendor_id, title, description, amount, currency)
		SELECT $1,$2,$3,$4,$5,$6,$7
		WHERE $3::uuid IS NULL OR EXISTS (SELECT 1 FROM vendors v WHERE v.id=$3 AND v.organization_id=$1)
		RETURNING id, title, description, amount, currency, status, COALESCE(vendor_id::text,''), created_at::text`,
		user.OrganizationID, user.ID, vendorID, strings.TrimSpace(input.Title), strings.TrimSpace(input.Description), input.Amount, input.Currency,
	).Scan(&created.ID, &created.Title, &created.Description, &created.Amount, &created.Currency, &created.Status, &created.VendorID, &created.CreatedAt)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create purchase request")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'purchase_request.created','purchase_request',$3,$4)`,
		user.OrganizationID, user.ID, created.ID, map[string]any{"amount": created.Amount})
	httpapi.WriteJSON(w, http.StatusCreated, created)
}

// SubmitPurchaseRequest routes a draft PR into the purchase_request workflow when configured.
func (h Handler) SubmitPurchaseRequest(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	id := r.PathValue("id")
	var title string
	var amount float64
	if err := h.DB.QueryRow(r.Context(), `SELECT title, amount FROM purchase_requests WHERE id=$1 AND organization_id=$2 AND status='draft'`, id, user.OrganizationID).Scan(&title, &amount); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_state", "purchase request not found or not a draft")
		return
	}
	definitionID := workflow.FindDefinitionByEntity(r.Context(), h.DB, user.OrganizationID, "purchase_request")
	var instanceID any
	if definitionID != "" {
		wid, err := workflow.Start(r.Context(), h.DB, user.OrganizationID, definitionID, "Purchase: "+title, "purchase_request", id, &amount, user.ID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "workflow_failed", "could not start approval")
			return
		}
		instanceID = wid
	}
	if _, err := h.DB.Exec(r.Context(), `UPDATE purchase_requests SET status='submitted', workflow_instance_id=$1, updated_at=NOW() WHERE id=$2 AND organization_id=$3`, instanceID, id, user.OrganizationID); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "update_failed", "could not submit purchase request")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id) VALUES ($1,$2,'purchase_request.submitted','purchase_request',$3)`, user.OrganizationID, user.ID, id)
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"status": "submitted", "routed": definitionID != ""})
}

// OrderPurchaseRequest marks an approved (or unrouted) submitted PR as ordered.
func (h Handler) OrderPurchaseRequest(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	id := r.PathValue("id")
	result, err := h.DB.Exec(r.Context(), `
		UPDATE purchase_requests p SET status='ordered', updated_at=NOW()
		WHERE p.id=$1 AND p.organization_id=$2 AND p.status='submitted'
		  AND (p.workflow_instance_id IS NULL OR EXISTS (SELECT 1 FROM workflow_instances w WHERE w.id=p.workflow_instance_id AND w.status='approved'))`,
		id, user.OrganizationID)
	if err != nil || result.RowsAffected() == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_state", "purchase request is not approved and submitted")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id) VALUES ($1,$2,'purchase_request.ordered','purchase_request',$3)`, user.OrganizationID, user.ID, id)
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "ordered"})
}
