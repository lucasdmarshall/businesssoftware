package finance

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"name/backend/internal/httpapi"
)

type BankAccount struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Currency            string `json:"currency"`
	AccountNumberMasked string `json:"account_number_masked"`
	GLAccountID         string `json:"gl_account_id"`
	IsActive            bool   `json:"is_active"`
	UnmatchedCount      int    `json:"unmatched_count"`
}

type BankTransaction struct {
	ID            string  `json:"id"`
	BankAccountID string  `json:"bank_account_id"`
	TxnDate       string  `json:"txn_date"`
	Amount        float64 `json:"amount"`
	Direction     string  `json:"direction"`
	Description   string  `json:"description"`
	Reference     string  `json:"reference"`
	Status        string  `json:"status"`
	PaymentID     string  `json:"payment_id"`
	JournalID     string  `json:"journal_id"`
	MatchID       string  `json:"match_id"`
}

// BankAccounts lists (GET) or creates (POST) bank accounts used for reconciliation.
func (h Handler) BankAccounts(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	h.ensure(user.OrganizationID, r)
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT a.id, a.name, a.currency, a.account_number_masked, COALESCE(a.gl_account_id::text,''), a.is_active,
			       COALESCE((SELECT COUNT(*) FROM finance_bank_transactions t
			                 WHERE t.bank_account_id=a.id AND t.status='unmatched'), 0)
			FROM finance_bank_accounts a
			WHERE a.organization_id=$1
			ORDER BY a.name`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load bank accounts")
			return
		}
		defer rows.Close()
		items := make([]BankAccount, 0)
		for rows.Next() {
			var item BankAccount
			if rows.Scan(&item.ID, &item.Name, &item.Currency, &item.AccountNumberMasked, &item.GLAccountID, &item.IsActive, &item.UnmatchedCount) == nil {
				items = append(items, item)
			}
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	if !h.Auth.HasPermission(r.Context(), user, "finance.manage") && !h.Auth.HasPermission(r.Context(), user, "finance.pay") {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "finance.manage required")
		return
	}
	var input struct {
		Name                string `json:"name"`
		Currency            string `json:"currency"`
		AccountNumberMasked string `json:"account_number_masked"`
		GLAccountID         string `json:"gl_account_id"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	var gl any
	if strings.TrimSpace(input.GLAccountID) != "" {
		gl = input.GLAccountID
	}
	var item BankAccount
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO finance_bank_accounts (organization_id, name, currency, account_number_masked, gl_account_id, created_by)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id, name, currency, account_number_masked, COALESCE(gl_account_id::text,''), is_active`,
		user.OrganizationID, strings.TrimSpace(input.Name), input.Currency, strings.TrimSpace(input.AccountNumberMasked), gl, user.ID,
	).Scan(&item.ID, &item.Name, &item.Currency, &item.AccountNumberMasked, &item.GLAccountID, &item.IsActive)
	if err != nil {
		httpapi.WriteError(w, http.StatusConflict, "conflict", "bank account name may already exist")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, item)
}

// BankTransactions lists (GET) or imports (POST) statement lines for a bank account.
func (h Handler) BankTransactions(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	h.ensure(user.OrganizationID, r)
	if r.Method == http.MethodGet {
		accountID := r.URL.Query().Get("bank_account_id")
		status := r.URL.Query().Get("status")
		query := `
			SELECT t.id, t.bank_account_id, t.txn_date::text, t.amount, t.direction, t.description, t.reference, t.status,
			       COALESCE(m.id::text,''), COALESCE(m.payment_id::text,''), COALESCE(m.journal_id::text,'')
			FROM finance_bank_transactions t
			LEFT JOIN finance_bank_matches m ON m.bank_transaction_id=t.id
			WHERE t.organization_id=$1`
		args := []any{user.OrganizationID}
		argN := 2
		if accountID != "" {
			query += ` AND t.bank_account_id=$` + strconv.Itoa(argN)
			args = append(args, accountID)
			argN++
		}
		if status != "" {
			query += ` AND t.status=$` + strconv.Itoa(argN)
			args = append(args, status)
			argN++
		}
		query += ` ORDER BY t.txn_date DESC, t.created_at DESC LIMIT 500`
		rows, err := h.DB.Query(r.Context(), query, args...)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load bank transactions")
			return
		}
		defer rows.Close()
		items := make([]BankTransaction, 0)
		for rows.Next() {
			var item BankTransaction
			if rows.Scan(&item.ID, &item.BankAccountID, &item.TxnDate, &item.Amount, &item.Direction, &item.Description, &item.Reference, &item.Status, &item.MatchID, &item.PaymentID, &item.JournalID) == nil {
				items = append(items, item)
			}
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	if !h.Auth.HasPermission(r.Context(), user, "finance.manage") && !h.Auth.HasPermission(r.Context(), user, "finance.pay") {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "finance.manage required")
		return
	}
	var input struct {
		BankAccountID string  `json:"bank_account_id"`
		TxnDate       string  `json:"txn_date"`
		Amount        float64 `json:"amount"`
		Direction     string  `json:"direction"`
		Description   string  `json:"description"`
		Reference     string  `json:"reference"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.BankAccountID == "" || input.Amount <= 0 || (input.Direction != "in" && input.Direction != "out") {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "bank_account_id, positive amount, and direction in|out are required")
		return
	}
	if input.TxnDate == "" {
		input.TxnDate = time.Now().UTC().Format("2006-01-02")
	}
	var exists bool
	if err := h.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM finance_bank_accounts WHERE id=$1 AND organization_id=$2)`, input.BankAccountID, user.OrganizationID).Scan(&exists); err != nil || !exists {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_account", "bank account not found")
		return
	}
	var item BankTransaction
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO finance_bank_transactions (organization_id, bank_account_id, txn_date, amount, direction, description, reference, created_by)
		VALUES ($1,$2,$3::date,$4,$5,$6,$7,$8)
		RETURNING id, bank_account_id, txn_date::text, amount, direction, description, reference, status`,
		user.OrganizationID, input.BankAccountID, input.TxnDate, input.Amount, input.Direction, strings.TrimSpace(input.Description), strings.TrimSpace(input.Reference), user.ID,
	).Scan(&item.ID, &item.BankAccountID, &item.TxnDate, &item.Amount, &item.Direction, &item.Description, &item.Reference, &item.Status)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not add bank transaction")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, item)
}

// MatchBankTransaction links an unmatched statement line to a payment and/or journal.
func (h Handler) MatchBankTransaction(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if !h.Auth.HasPermission(r.Context(), user, "finance.manage") && !h.Auth.HasPermission(r.Context(), user, "finance.pay") {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "finance.manage required")
		return
	}
	txnID := r.PathValue("id")
	var input struct {
		PaymentID string `json:"payment_id"`
		JournalID string `json:"journal_id"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || (strings.TrimSpace(input.PaymentID) == "" && strings.TrimSpace(input.JournalID) == "") {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "payment_id and/or journal_id required")
		return
	}
	var status string
	if err := h.DB.QueryRow(r.Context(), `SELECT status FROM finance_bank_transactions WHERE id=$1 AND organization_id=$2`, txnID, user.OrganizationID).Scan(&status); err != nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "bank transaction not found")
		return
	}
	if status != "unmatched" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_state", "transaction is not unmatched")
		return
	}
	var payment, journal any
	if strings.TrimSpace(input.PaymentID) != "" {
		var ok bool
		if err := h.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM finance_payments WHERE id=$1 AND organization_id=$2)`, input.PaymentID, user.OrganizationID).Scan(&ok); err != nil || !ok {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_payment", "payment not found")
			return
		}
		payment = input.PaymentID
	}
	if strings.TrimSpace(input.JournalID) != "" {
		var ok bool
		if err := h.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM finance_journals WHERE id=$1 AND organization_id=$2)`, input.JournalID, user.OrganizationID).Scan(&ok); err != nil || !ok {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_journal", "journal not found")
			return
		}
		journal = input.JournalID
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "tx_failed", "could not match transaction")
		return
	}
	defer tx.Rollback(r.Context())
	var matchID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO finance_bank_matches (organization_id, bank_transaction_id, payment_id, journal_id, matched_by)
		VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		user.OrganizationID, txnID, payment, journal, user.ID,
	).Scan(&matchID)
	if err != nil {
		httpapi.WriteError(w, http.StatusConflict, "match_failed", "could not create match (already matched?)")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE finance_bank_transactions SET status='matched' WHERE id=$1 AND organization_id=$2`, txnID, user.OrganizationID); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "update_failed", "could not update transaction status")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not save match")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'finance.bank_matched','finance_bank_transaction',$3,$4)`,
		user.OrganizationID, user.ID, txnID, map[string]any{"match_id": matchID, "payment_id": input.PaymentID, "journal_id": input.JournalID})
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"status": "matched", "match_id": matchID})
}

// UnmatchBankTransaction clears a match and returns the line to unmatched.
func (h Handler) UnmatchBankTransaction(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if !h.Auth.HasPermission(r.Context(), user, "finance.manage") && !h.Auth.HasPermission(r.Context(), user, "finance.pay") {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "finance.manage required")
		return
	}
	txnID := r.PathValue("id")
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "tx_failed", "could not unmatch")
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), `DELETE FROM finance_bank_matches WHERE bank_transaction_id=$1 AND organization_id=$2`, txnID, user.OrganizationID)
	if err != nil || tag.RowsAffected() == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_state", "no match to clear")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE finance_bank_transactions SET status='unmatched' WHERE id=$1 AND organization_id=$2 AND status='matched'`, txnID, user.OrganizationID); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "update_failed", "could not update transaction")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not unmatch")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "unmatched"})
}

// ExcludeBankTransaction marks a statement line as excluded from matching.
func (h Handler) ExcludeBankTransaction(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if !h.Auth.HasPermission(r.Context(), user, "finance.manage") && !h.Auth.HasPermission(r.Context(), user, "finance.pay") {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "finance.manage required")
		return
	}
	txnID := r.PathValue("id")
	tag, err := h.DB.Exec(r.Context(), `
		UPDATE finance_bank_transactions SET status='excluded'
		WHERE id=$1 AND organization_id=$2 AND status='unmatched'`, txnID, user.OrganizationID)
	if err != nil || tag.RowsAffected() == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_state", "only unmatched transactions can be excluded")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "excluded"})
}
