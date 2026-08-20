package finance

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"name/backend/internal/httpapi"
)

type Account struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	AccountType string `json:"account_type"`
	IsSystem    bool   `json:"is_system"`
	IsActive    bool   `json:"is_active"`
}

type TaxCode struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	RatePercent float64 `json:"rate_percent"`
	IsActive    bool    `json:"is_active"`
}

type Customer struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ContactEmail string `json:"contact_email"`
	ContactPhone string `json:"contact_phone"`
	Status       string `json:"status"`
}

type Journal struct {
	ID        string        `json:"id"`
	EntryDate string        `json:"entry_date"`
	Memo      string        `json:"memo"`
	Status    string        `json:"status"`
	Source    string        `json:"source"`
	Lines     []JournalLine `json:"lines,omitempty"`
	DebitSum  float64       `json:"debit_sum"`
	CreditSum float64       `json:"credit_sum"`
}

type JournalLine struct {
	AccountID   string  `json:"account_id"`
	AccountCode string  `json:"account_code,omitempty"`
	AccountName string  `json:"account_name,omitempty"`
	Description string  `json:"description"`
	Debit       float64 `json:"debit"`
	Credit      float64 `json:"credit"`
}

type Bill struct {
	ID          string  `json:"id"`
	VendorID    string  `json:"vendor_id"`
	VendorName  string  `json:"vendor_name"`
	Number      string  `json:"number"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
	TaxAmount   float64 `json:"tax_amount"`
	Currency    string  `json:"currency"`
	Status      string  `json:"status"`
	BillDate    string  `json:"bill_date"`
	DueDate     string  `json:"due_date"`
}

type Payment struct {
	ID           string  `json:"id"`
	Direction    string  `json:"direction"`
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
	Method       string  `json:"method"`
	PaidOn       string  `json:"paid_on"`
	Reference    string  `json:"reference"`
	Memo         string  `json:"memo"`
	VendorID     string  `json:"vendor_id"`
	VendorName   string  `json:"vendor_name"`
	CustomerID   string  `json:"customer_id"`
	CustomerName string  `json:"customer_name"`
	BillID       string  `json:"bill_id"`
	InvoiceID    string  `json:"invoice_id"`
}

type SuiteOverview struct {
	CashBalance   float64 `json:"cash_balance"`
	APOpen        float64 `json:"ap_open"`
	AROpen        float64 `json:"ar_open"`
	ExpensesMonth float64 `json:"expenses_month"`
	BillsOpen     int     `json:"bills_open"`
	InvoicesOpen  int     `json:"invoices_open"`
	Currency      string  `json:"currency"`
}

func (h Handler) ensure(ctxOrg string, r *http.Request) {
	_ = SeedFinanceSuite(r.Context(), h.DB, ctxOrg)
}

func (h Handler) Overview(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	h.ensure(user.OrganizationID, r)
	var out SuiteOverview
	out.Currency = "USD"
	_ = h.DB.QueryRow(r.Context(), `
		SELECT COALESCE(SUM(CASE WHEN direction='in' THEN amount ELSE -amount END),0)
		FROM finance_payments WHERE organization_id=$1`, user.OrganizationID).Scan(&out.CashBalance)
	_ = h.DB.QueryRow(r.Context(), `
		SELECT COALESCE(SUM(amount+tax_amount),0), COUNT(*)
		FROM finance_bills WHERE organization_id=$1 AND status IN ('open','partial')`,
		user.OrganizationID).Scan(&out.APOpen, &out.BillsOpen)
	_ = h.DB.QueryRow(r.Context(), `
		SELECT COALESCE(SUM(amount+COALESCE(tax_amount,0)),0), COUNT(*)
		FROM invoices WHERE organization_id=$1 AND status IN ('sent','overdue')`,
		user.OrganizationID).Scan(&out.AROpen, &out.InvoicesOpen)
	monthStart := time.Now().UTC().Format("2006-01") + "-01"
	_ = h.DB.QueryRow(r.Context(), `
		SELECT COALESCE(SUM(amount),0) FROM expenses
		WHERE organization_id=$1 AND status IN ('submitted','paid') AND created_at>=$2::date`,
		user.OrganizationID, monthStart).Scan(&out.ExpensesMonth)
	httpapi.WriteJSON(w, http.StatusOK, out)
}

func (h Handler) Accounts(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	h.ensure(user.OrganizationID, r)
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT id, code, name, account_type, is_system, is_active
			FROM finance_accounts WHERE organization_id=$1 ORDER BY code`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load accounts")
			return
		}
		defer rows.Close()
		items := make([]Account, 0)
		for rows.Next() {
			var item Account
			if rows.Scan(&item.ID, &item.Code, &item.Name, &item.AccountType, &item.IsSystem, &item.IsActive) == nil {
				items = append(items, item)
			}
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	if !h.Auth.HasPermission(r.Context(), user, "finance.accounts.manage") && !h.Auth.HasPermission(r.Context(), user, "finance.manage") {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "finance.accounts.manage required")
		return
	}
	var input struct {
		Code        string `json:"code"`
		Name        string `json:"name"`
		AccountType string `json:"account_type"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Code) == "" || strings.TrimSpace(input.Name) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "code and name are required")
		return
	}
	switch input.AccountType {
	case "asset", "liability", "equity", "revenue", "expense":
	default:
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_type", "account_type must be asset, liability, equity, revenue, or expense")
		return
	}
	var item Account
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO finance_accounts (organization_id, code, name, account_type)
		VALUES ($1,$2,$3,$4) RETURNING id, code, name, account_type, is_system, is_active`,
		user.OrganizationID, strings.TrimSpace(input.Code), strings.TrimSpace(input.Name), input.AccountType,
	).Scan(&item.ID, &item.Code, &item.Name, &item.AccountType, &item.IsSystem, &item.IsActive)
	if err != nil {
		httpapi.WriteError(w, http.StatusConflict, "conflict", "account code may already exist")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, item)
}

func (h Handler) TaxCodes(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	h.ensure(user.OrganizationID, r)
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT id, code, name, rate_percent, is_active FROM finance_tax_codes
			WHERE organization_id=$1 ORDER BY code`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load tax codes")
			return
		}
		defer rows.Close()
		items := make([]TaxCode, 0)
		for rows.Next() {
			var item TaxCode
			if rows.Scan(&item.ID, &item.Code, &item.Name, &item.RatePercent, &item.IsActive) == nil {
				items = append(items, item)
			}
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	if !h.Auth.HasPermission(r.Context(), user, "finance.accounts.manage") && !h.Auth.HasPermission(r.Context(), user, "finance.manage") {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "finance.accounts.manage required")
		return
	}
	var input struct {
		Code        string  `json:"code"`
		Name        string  `json:"name"`
		RatePercent float64 `json:"rate_percent"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Code) == "" || strings.TrimSpace(input.Name) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "code and name are required")
		return
	}
	var item TaxCode
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO finance_tax_codes (organization_id, code, name, rate_percent)
		VALUES ($1,$2,$3,$4) RETURNING id, code, name, rate_percent, is_active`,
		user.OrganizationID, strings.TrimSpace(input.Code), strings.TrimSpace(input.Name), input.RatePercent,
	).Scan(&item.ID, &item.Code, &item.Name, &item.RatePercent, &item.IsActive)
	if err != nil {
		httpapi.WriteError(w, http.StatusConflict, "conflict", "tax code may already exist")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, item)
}

func (h Handler) Customers(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	h.ensure(user.OrganizationID, r)
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT id, name, contact_email, contact_phone, status FROM finance_customers
			WHERE organization_id=$1 ORDER BY name`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load customers")
			return
		}
		defer rows.Close()
		items := make([]Customer, 0)
		for rows.Next() {
			var item Customer
			if rows.Scan(&item.ID, &item.Name, &item.ContactEmail, &item.ContactPhone, &item.Status) == nil {
				items = append(items, item)
			}
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
	var item Customer
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO finance_customers (organization_id, name, contact_email, contact_phone, created_by)
		VALUES ($1,$2,$3,$4,$5) RETURNING id, name, contact_email, contact_phone, status`,
		user.OrganizationID, strings.TrimSpace(input.Name), strings.TrimSpace(input.ContactEmail), strings.TrimSpace(input.ContactPhone), user.ID,
	).Scan(&item.ID, &item.Name, &item.ContactEmail, &item.ContactPhone, &item.Status)
	if err != nil {
		httpapi.WriteError(w, http.StatusConflict, "conflict", "customer may already exist")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, item)
}

func (h Handler) Journals(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	h.ensure(user.OrganizationID, r)
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT j.id, j.entry_date::text, j.memo, j.status, j.source,
			       COALESCE((SELECT SUM(debit) FROM finance_journal_lines l WHERE l.journal_id=j.id),0),
			       COALESCE((SELECT SUM(credit) FROM finance_journal_lines l WHERE l.journal_id=j.id),0)
			FROM finance_journals j WHERE j.organization_id=$1
			ORDER BY j.entry_date DESC, j.created_at DESC LIMIT 200`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load journals")
			return
		}
		defer rows.Close()
		items := make([]Journal, 0)
		for rows.Next() {
			var item Journal
			if rows.Scan(&item.ID, &item.EntryDate, &item.Memo, &item.Status, &item.Source, &item.DebitSum, &item.CreditSum) == nil {
				items = append(items, item)
			}
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	if !h.Auth.HasPermission(r.Context(), user, "finance.journals.manage") && !h.Auth.HasPermission(r.Context(), user, "finance.manage") {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "finance.journals.manage required")
		return
	}
	var input struct {
		EntryDate string        `json:"entry_date"`
		Memo      string        `json:"memo"`
		Lines     []JournalLine `json:"lines"`
		Post      bool          `json:"post"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || len(input.Lines) < 2 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "entry needs at least two lines")
		return
	}
	if input.EntryDate == "" {
		input.EntryDate = time.Now().UTC().Format("2006-01-02")
	}
	var debitSum, creditSum float64
	for _, line := range input.Lines {
		if line.AccountID == "" || (line.Debit <= 0 && line.Credit <= 0) || (line.Debit > 0 && line.Credit > 0) {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_line", "each line needs an account and either debit or credit")
			return
		}
		debitSum += line.Debit
		creditSum += line.Credit
	}
	if round2(debitSum) != round2(creditSum) {
		httpapi.WriteError(w, http.StatusBadRequest, "unbalanced", "debits must equal credits")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "tx_failed", "could not create journal")
		return
	}
	defer tx.Rollback(r.Context())
	status := "draft"
	var postedAt any
	if input.Post {
		status = "posted"
		postedAt = time.Now().UTC()
	}
	var journalID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO finance_journals (organization_id, entry_date, memo, status, source, created_by, posted_at)
		VALUES ($1,$2::date,$3,$4,'manual',$5,$6) RETURNING id`,
		user.OrganizationID, input.EntryDate, input.Memo, status, user.ID, postedAt,
	).Scan(&journalID)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create journal")
		return
	}
	for _, line := range input.Lines {
		_, err = tx.Exec(r.Context(), `
			INSERT INTO finance_journal_lines (journal_id, organization_id, account_id, description, debit, credit)
			VALUES ($1,$2,$3,$4,$5,$6)`, journalID, user.OrganizationID, line.AccountID, line.Description, line.Debit, line.Credit)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "line_failed", "could not save journal line")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not save journal")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'finance.journal_created','finance_journal',$3,$4)`,
		user.OrganizationID, user.ID, journalID, map[string]any{"status": status, "debit_sum": debitSum})
	httpapi.WriteJSON(w, http.StatusCreated, Journal{ID: journalID, EntryDate: input.EntryDate, Memo: input.Memo, Status: status, Source: "manual", DebitSum: debitSum, CreditSum: creditSum, Lines: input.Lines})
}

func (h Handler) PostJournal(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if !h.Auth.HasPermission(r.Context(), user, "finance.journals.manage") && !h.Auth.HasPermission(r.Context(), user, "finance.manage") {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "finance.journals.manage required")
		return
	}
	id := r.PathValue("id")
	tag, err := h.DB.Exec(r.Context(), `
		UPDATE finance_journals SET status='posted', posted_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND organization_id=$2 AND status='draft'`, id, user.OrganizationID)
	if err != nil || tag.RowsAffected() == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "post_failed", "draft journal not found")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "posted"})
}

func (h Handler) Bills(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	h.ensure(user.OrganizationID, r)
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT b.id, b.vendor_id, v.name, b.number, b.description, b.amount, b.tax_amount, b.currency, b.status,
			       b.bill_date::text, COALESCE(b.due_date::text,'')
			FROM finance_bills b JOIN vendors v ON v.id=b.vendor_id
			WHERE b.organization_id=$1 ORDER BY b.bill_date DESC LIMIT 500`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load bills")
			return
		}
		defer rows.Close()
		items := make([]Bill, 0)
		for rows.Next() {
			var item Bill
			if rows.Scan(&item.ID, &item.VendorID, &item.VendorName, &item.Number, &item.Description, &item.Amount, &item.TaxAmount, &item.Currency, &item.Status, &item.BillDate, &item.DueDate) == nil {
				items = append(items, item)
			}
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		VendorID    string  `json:"vendor_id"`
		Number      string  `json:"number"`
		Description string  `json:"description"`
		Amount      float64 `json:"amount"`
		TaxAmount   float64 `json:"tax_amount"`
		Currency    string  `json:"currency"`
		BillDate    string  `json:"bill_date"`
		DueDate     string  `json:"due_date"`
		TaxCodeID   string  `json:"tax_code_id"`
		Open        bool    `json:"open"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.VendorID == "" || strings.TrimSpace(input.Number) == "" || input.Amount < 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "vendor_id, number, and amount are required")
		return
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	if input.BillDate == "" {
		input.BillDate = time.Now().UTC().Format("2006-01-02")
	}
	status := "draft"
	if input.Open {
		status = "open"
	}
	var due any
	if strings.TrimSpace(input.DueDate) != "" {
		due = input.DueDate
	}
	var taxCode any
	if input.TaxCodeID != "" {
		taxCode = input.TaxCodeID
	}
	var item Bill
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO finance_bills (organization_id, vendor_id, number, description, amount, tax_amount, currency, status, bill_date, due_date, tax_code_id, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::date,$10,$11,$12)
		RETURNING id, vendor_id, number, description, amount, tax_amount, currency, status, bill_date::text, COALESCE(due_date::text,'')`,
		user.OrganizationID, input.VendorID, strings.TrimSpace(input.Number), input.Description, input.Amount, input.TaxAmount, input.Currency, status, input.BillDate, due, taxCode, user.ID,
	).Scan(&item.ID, &item.VendorID, &item.Number, &item.Description, &item.Amount, &item.TaxAmount, &item.Currency, &item.Status, &item.BillDate, &item.DueDate)
	if err != nil {
		httpapi.WriteError(w, http.StatusConflict, "conflict", "bill number may already exist")
		return
	}
	_ = h.DB.QueryRow(r.Context(), `SELECT name FROM vendors WHERE id=$1`, item.VendorID).Scan(&item.VendorName)
	httpapi.WriteJSON(w, http.StatusCreated, item)
}

func (h Handler) SetBillStatus(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var input struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.ID == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "id and status required")
		return
	}
	switch input.Status {
	case "draft", "open", "partial", "paid", "void":
	default:
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_status", "invalid bill status")
		return
	}
	tag, err := h.DB.Exec(r.Context(), `
		UPDATE finance_bills SET status=$1, updated_at=NOW() WHERE id=$2 AND organization_id=$3`,
		input.Status, input.ID, user.OrganizationID)
	if err != nil || tag.RowsAffected() == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "update_failed", "bill not found")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": input.Status})
}

func (h Handler) Payments(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	h.ensure(user.OrganizationID, r)
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT p.id, p.direction, p.amount, p.currency, p.method, p.paid_on::text, p.reference, p.memo,
			       COALESCE(p.vendor_id::text,''), COALESCE(v.name,''),
			       COALESCE(p.customer_id::text,''), COALESCE(c.name,''),
			       COALESCE(p.bill_id::text,''), COALESCE(p.invoice_id::text,'')
			FROM finance_payments p
			LEFT JOIN vendors v ON v.id=p.vendor_id
			LEFT JOIN finance_customers c ON c.id=p.customer_id
			WHERE p.organization_id=$1 ORDER BY p.paid_on DESC, p.created_at DESC LIMIT 500`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load payments")
			return
		}
		defer rows.Close()
		items := make([]Payment, 0)
		for rows.Next() {
			var item Payment
			if rows.Scan(&item.ID, &item.Direction, &item.Amount, &item.Currency, &item.Method, &item.PaidOn, &item.Reference, &item.Memo,
				&item.VendorID, &item.VendorName, &item.CustomerID, &item.CustomerName, &item.BillID, &item.InvoiceID) == nil {
				items = append(items, item)
			}
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	if !h.Auth.HasPermission(r.Context(), user, "finance.pay") && !h.Auth.HasPermission(r.Context(), user, "finance.manage") {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "finance.pay required")
		return
	}
	var input struct {
		Direction  string  `json:"direction"`
		Amount     float64 `json:"amount"`
		Currency   string  `json:"currency"`
		Method     string  `json:"method"`
		PaidOn     string  `json:"paid_on"`
		Reference  string  `json:"reference"`
		Memo       string  `json:"memo"`
		VendorID   string  `json:"vendor_id"`
		CustomerID string  `json:"customer_id"`
		BillID     string  `json:"bill_id"`
		InvoiceID  string  `json:"invoice_id"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || (input.Direction != "in" && input.Direction != "out") || input.Amount <= 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "direction and positive amount are required")
		return
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	if input.Method == "" {
		input.Method = "transfer"
	}
	if input.PaidOn == "" {
		input.PaidOn = time.Now().UTC().Format("2006-01-02")
	}
	var vendor, customer, bill, invoice any
	if input.VendorID != "" {
		vendor = input.VendorID
	}
	if input.CustomerID != "" {
		customer = input.CustomerID
	}
	if input.BillID != "" {
		bill = input.BillID
	}
	if input.InvoiceID != "" {
		invoice = input.InvoiceID
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "tx_failed", "could not record payment")
		return
	}
	defer tx.Rollback(r.Context())
	var id string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO finance_payments (organization_id, direction, amount, currency, method, paid_on, reference, memo, vendor_id, customer_id, bill_id, invoice_id, created_by)
		VALUES ($1,$2,$3,$4,$5,$6::date,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
		user.OrganizationID, input.Direction, input.Amount, input.Currency, input.Method, input.PaidOn, input.Reference, input.Memo, vendor, customer, bill, invoice, user.ID,
	).Scan(&id)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not record payment")
		return
	}
	if input.BillID != "" && input.Direction == "out" {
		_, _ = tx.Exec(r.Context(), `UPDATE finance_bills SET status='paid', updated_at=NOW() WHERE id=$1 AND organization_id=$2`, input.BillID, user.OrganizationID)
	}
	if input.InvoiceID != "" && input.Direction == "in" {
		_, _ = tx.Exec(r.Context(), `UPDATE invoices SET status='paid', updated_at=NOW() WHERE id=$1 AND organization_id=$2`, input.InvoiceID, user.OrganizationID)
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not save payment")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'finance.payment_recorded','finance_payment',$3,$4)`,
		user.OrganizationID, user.ID, id, map[string]any{"direction": input.Direction, "amount": input.Amount})
	httpapi.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "recorded"})
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
