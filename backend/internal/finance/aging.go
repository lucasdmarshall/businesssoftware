package finance

import (
	"net/http"
	"time"

	"name/backend/internal/httpapi"
)

// Aging buckets: current (not yet due or 0–30 days past due), then 31–60, 61–90, 90+.
const (
	agingCurrent = "0_30"
	aging3160    = "31_60"
	aging6190    = "61_90"
	aging90Plus  = "90_plus"
)

type AgingBucket struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Count    int     `json:"count"`
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

type AgingRow struct {
	ID          string  `json:"id"`
	Number      string  `json:"number"`
	PartyName   string  `json:"party_name"`
	DueDate     string  `json:"due_date"`
	DaysPastDue int     `json:"days_past_due"`
	Bucket      string  `json:"bucket"`
	OpenAmount  float64 `json:"open_amount"`
	Currency    string  `json:"currency"`
	Status      string  `json:"status"`
}

type AgingReport struct {
	AsOf     string        `json:"as_of"`
	Side     string        `json:"side"`
	Currency string        `json:"currency"`
	Total    float64       `json:"total"`
	Buckets  []AgingBucket `json:"buckets"`
	Rows     []AgingRow    `json:"rows"`
}

func agingBucketFor(daysPastDue int) string {
	if daysPastDue <= 30 {
		return agingCurrent
	}
	if daysPastDue <= 60 {
		return aging3160
	}
	if daysPastDue <= 90 {
		return aging6190
	}
	return aging90Plus
}

func agingBucketLabel(key string) string {
	switch key {
	case agingCurrent:
		return "0–30 days"
	case aging3160:
		return "31–60 days"
	case aging6190:
		return "61–90 days"
	case aging90Plus:
		return "90+ days"
	default:
		return key
	}
}

func emptyAgingBuckets(currency string) []AgingBucket {
	keys := []string{agingCurrent, aging3160, aging6190, aging90Plus}
	out := make([]AgingBucket, 0, len(keys))
	for _, key := range keys {
		out = append(out, AgingBucket{Key: key, Label: agingBucketLabel(key), Currency: currency})
	}
	return out
}

func parseAsOf(raw string) (time.Time, string, bool) {
	if raw == "" {
		now := time.Now().UTC()
		asOf := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return asOf, asOf.Format("2006-01-02"), true
	}
	asOf, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, "", false
	}
	return asOf, asOf.Format("2006-01-02"), true
}

func daysPastDue(asOf, due time.Time) int {
	if due.After(asOf) {
		return 0
	}
	return int(asOf.Sub(due).Hours() / 24)
}

// AgingAP reports open/partial vendor bills by days past due.
func (h Handler) AgingAP(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	h.ensure(user.OrganizationID, r)
	asOf, asOfStr, ok := parseAsOf(r.URL.Query().Get("as_of"))
	if !ok {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_as_of", "as_of must be YYYY-MM-DD")
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT b.id, b.number, v.name,
		       COALESCE(b.due_date, b.bill_date)::text,
		       (b.amount + b.tax_amount) - COALESCE((
		         SELECT SUM(p.amount) FROM finance_payments p
		         WHERE p.bill_id=b.id AND p.organization_id=b.organization_id AND p.direction='out'
		       ), 0),
		       b.currency, b.status
		FROM finance_bills b
		JOIN vendors v ON v.id=b.vendor_id
		WHERE b.organization_id=$1 AND b.status IN ('open','partial')
		ORDER BY COALESCE(b.due_date, b.bill_date), b.number`, user.OrganizationID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load AP aging")
		return
	}
	defer rows.Close()
	report := buildAgingReport("ap", asOf, asOfStr, rows)
	httpapi.WriteJSON(w, http.StatusOK, report)
}

// AgingAR reports sent/overdue customer invoices by days past due.
func (h Handler) AgingAR(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	h.ensure(user.OrganizationID, r)
	asOf, asOfStr, ok := parseAsOf(r.URL.Query().Get("as_of"))
	if !ok {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_as_of", "as_of must be YYYY-MM-DD")
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT i.id, i.number, i.customer_name,
		       COALESCE(i.due_date, COALESCE(i.issued_date, i.created_at::date))::text,
		       (i.amount + COALESCE(i.tax_amount,0)) - COALESCE((
		         SELECT SUM(p.amount) FROM finance_payments p
		         WHERE p.invoice_id=i.id AND p.organization_id=i.organization_id AND p.direction='in'
		       ), 0),
		       i.currency, i.status
		FROM invoices i
		WHERE i.organization_id=$1 AND i.status IN ('sent','overdue')
		ORDER BY COALESCE(i.due_date, COALESCE(i.issued_date, i.created_at::date)), i.number`, user.OrganizationID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load AR aging")
		return
	}
	defer rows.Close()
	report := buildAgingReport("ar", asOf, asOfStr, rows)
	httpapi.WriteJSON(w, http.StatusOK, report)
}

type agingScanner interface {
	Next() bool
	Scan(dest ...any) error
}

func buildAgingReport(side string, asOf time.Time, asOfStr string, rows agingScanner) AgingReport {
	currency := "USD"
	buckets := emptyAgingBuckets(currency)
	bucketIdx := map[string]int{agingCurrent: 0, aging3160: 1, aging6190: 2, aging90Plus: 3}
	detail := make([]AgingRow, 0)
	var total float64
	for rows.Next() {
		var row AgingRow
		var dueRaw string
		if err := rows.Scan(&row.ID, &row.Number, &row.PartyName, &dueRaw, &row.OpenAmount, &row.Currency, &row.Status); err != nil {
			continue
		}
		if row.OpenAmount <= 0 {
			continue
		}
		due, err := time.Parse("2006-01-02", dueRaw)
		if err != nil {
			due = asOf
		}
		row.DueDate = dueRaw
		row.DaysPastDue = daysPastDue(asOf, due)
		row.Bucket = agingBucketFor(row.DaysPastDue)
		if row.Currency != "" {
			currency = row.Currency
		}
		detail = append(detail, row)
		total += row.OpenAmount
		if idx, ok := bucketIdx[row.Bucket]; ok {
			buckets[idx].Count++
			buckets[idx].Amount += row.OpenAmount
			buckets[idx].Currency = currency
		}
	}
	for i := range buckets {
		buckets[i].Currency = currency
	}
	return AgingReport{
		AsOf:     asOfStr,
		Side:     side,
		Currency: currency,
		Total:    round2(total),
		Buckets:  buckets,
		Rows:     detail,
	}
}
