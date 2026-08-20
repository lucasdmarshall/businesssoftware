package crm

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"name/backend/internal/httpapi"
)

var validStages = map[string]bool{
	"prospect": true, "qualified": true, "proposal": true, "won": true, "lost": true,
}

func stageProbability(stage string) float64 {
	switch stage {
	case "prospect":
		return 10
	case "qualified":
		return 35
	case "proposal":
		return 60
	case "won":
		return 100
	case "lost":
		return 0
	default:
		return 10
	}
}

type PipelineSummary struct {
	OpenCount     int           `json:"open_count"`
	OpenValue     float64       `json:"open_value"`
	WeightedValue float64       `json:"weighted_value"`
	WonCount      int           `json:"won_count"`
	WonValue      float64       `json:"won_value"`
	LeadOpen      int           `json:"lead_open"`
	Currency      string        `json:"currency"`
	Stages        []StageBucket `json:"stages"`
}

type StageBucket struct {
	Stage  string  `json:"stage"`
	Count  int     `json:"count"`
	Amount float64 `json:"amount"`
}

type Opportunity struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Stage       string  `json:"stage"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	Probability float64 `json:"probability"`
	CloseDate   string  `json:"close_date"`
	CompanyID   string  `json:"company_id"`
	CompanyName string  `json:"company_name"`
	OwnerName   string  `json:"owner_name"`
	LeadID      string  `json:"lead_id"`
	UpdatedAt   string  `json:"updated_at"`
}

// PipelineSummary returns open/won pipeline metrics and per-stage buckets.
func (h Handler) PipelineSummary(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	out := PipelineSummary{Currency: "USD", Stages: []StageBucket{
		{Stage: "prospect"}, {Stage: "qualified"}, {Stage: "proposal"}, {Stage: "won"}, {Stage: "lost"},
	}}
	rows, err := h.DB.Query(r.Context(), `
		SELECT stage, COUNT(*), COALESCE(SUM(amount),0)
		FROM opportunities WHERE organization_id=$1
		GROUP BY stage`, user.OrganizationID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load pipeline summary")
		return
	}
	defer rows.Close()
	idx := map[string]int{"prospect": 0, "qualified": 1, "proposal": 2, "won": 3, "lost": 4}
	for rows.Next() {
		var stage string
		var count int
		var amount float64
		if rows.Scan(&stage, &count, &amount) != nil {
			continue
		}
		if i, ok := idx[stage]; ok {
			out.Stages[i].Count = count
			out.Stages[i].Amount = amount
		}
		switch stage {
		case "won":
			out.WonCount += count
			out.WonValue += amount
		case "lost":
			// excluded from open
		default:
			out.OpenCount += count
			out.OpenValue += amount
		}
	}
	_ = h.DB.QueryRow(r.Context(), `
		SELECT COALESCE(SUM(amount * probability / 100.0),0)
		FROM opportunities WHERE organization_id=$1 AND stage NOT IN ('won','lost')`,
		user.OrganizationID).Scan(&out.WeightedValue)
	_ = h.DB.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM leads WHERE organization_id=$1 AND status IN ('new','qualified')`,
		user.OrganizationID).Scan(&out.LeadOpen)
	httpapi.WriteJSON(w, http.StatusOK, out)
}

// MoveStage advances an opportunity stage, records history, and logs an activity.
func (h Handler) MoveStage(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	oppID := r.PathValue("id")
	var input struct {
		Stage     string   `json:"stage"`
		Note      string   `json:"note"`
		CloseDate string   `json:"close_date"`
		Amount    *float64 `json:"amount"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || !validStages[input.Stage] {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "valid stage is required")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "tx_failed", "could not move stage")
		return
	}
	defer tx.Rollback(r.Context())

	var fromStage string
	var name string
	if err := tx.QueryRow(r.Context(), `
		SELECT stage, name FROM opportunities WHERE id=$1 AND organization_id=$2 FOR UPDATE`,
		oppID, user.OrganizationID).Scan(&fromStage, &name); err != nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "opportunity not found")
		return
	}
	if fromStage == input.Stage {
		httpapi.WriteJSON(w, http.StatusOK, map[string]string{"stage": input.Stage, "unchanged": "true"})
		return
	}
	prob := stageProbability(input.Stage)
	var closeDate any
	if strings.TrimSpace(input.CloseDate) != "" {
		closeDate = input.CloseDate
	} else if input.Stage == "won" || input.Stage == "lost" {
		closeDate = time.Now().UTC().Format("2006-01-02")
	}
	if input.Amount != nil {
		_, err = tx.Exec(r.Context(), `
			UPDATE opportunities SET stage=$1, probability=$2, close_date=COALESCE($3::date, close_date),
			       amount=$4, updated_at=NOW()
			WHERE id=$5 AND organization_id=$6`,
			input.Stage, prob, closeDate, *input.Amount, oppID, user.OrganizationID)
	} else {
		_, err = tx.Exec(r.Context(), `
			UPDATE opportunities SET stage=$1, probability=$2, close_date=COALESCE($3::date, close_date), updated_at=NOW()
			WHERE id=$4 AND organization_id=$5`,
			input.Stage, prob, closeDate, oppID, user.OrganizationID)
	}
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "update_failed", "could not update stage")
		return
	}
	_, err = tx.Exec(r.Context(), `
		INSERT INTO opportunity_stage_history (organization_id, opportunity_id, from_stage, to_stage, note, changed_by)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		user.OrganizationID, oppID, fromStage, input.Stage, strings.TrimSpace(input.Note), user.ID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "history_failed", "could not record stage history")
		return
	}
	note := strings.TrimSpace(input.Note)
	if note == "" {
		note = "Stage moved from " + fromStage + " to " + input.Stage
	}
	_, _ = tx.Exec(r.Context(), `
		INSERT INTO crm_activities (organization_id, entity_type, entity_id, kind, note, created_by)
		VALUES ($1,'opportunity',$2,'note',$3,$4)`,
		user.OrganizationID, oppID, note, user.ID)
	if err := tx.Commit(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not save stage move")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'crm.stage_moved','opportunity',$3,$4)`,
		user.OrganizationID, user.ID, oppID, map[string]any{"from": fromStage, "to": input.Stage, "name": name})
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"stage": input.Stage, "probability": prob})
}

// ConvertLead turns a lead into an opportunity (and optional company/contact).
func (h Handler) ConvertLead(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	leadID := r.PathValue("id")
	var input struct {
		CompanyID   string  `json:"company_id"`
		CompanyName string  `json:"company_name"`
		Amount      float64 `json:"amount"`
		Currency    string  `json:"currency"`
		CloseDate   string  `json:"close_date"`
		Stage       string  `json:"stage"`
	}
	_ = json.NewDecoder(r.Body).Decode(&input)
	if input.Stage == "" {
		input.Stage = "prospect"
	}
	if !validStages[input.Stage] || input.Stage == "won" || input.Stage == "lost" {
		input.Stage = "prospect"
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "tx_failed", "could not convert lead")
		return
	}
	defer tx.Rollback(r.Context())

	var name, email, status, existingCompany string
	err = tx.QueryRow(r.Context(), `
		SELECT name, contact_email, status, COALESCE(company_id::text,'')
		FROM leads WHERE id=$1 AND organization_id=$2 FOR UPDATE`,
		leadID, user.OrganizationID).Scan(&name, &email, &status, &existingCompany)
	if err != nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "lead not found")
		return
	}
	if status == "converted" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_state", "lead already converted")
		return
	}
	if status == "lost" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_state", "lost leads cannot be converted")
		return
	}

	var resolvedCompany any
	if strings.TrimSpace(input.CompanyID) != "" {
		resolvedCompany = input.CompanyID
	} else if existingCompany != "" {
		resolvedCompany = existingCompany
	} else if strings.TrimSpace(input.CompanyName) != "" {
		var newCompanyID string
		err = tx.QueryRow(r.Context(), `
			INSERT INTO companies (organization_id, name, created_by)
			VALUES ($1,$2,$3)
			ON CONFLICT (organization_id, name) DO UPDATE SET name=EXCLUDED.name
			RETURNING id`, user.OrganizationID, strings.TrimSpace(input.CompanyName), user.ID).Scan(&newCompanyID)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "company_failed", "could not create company")
			return
		}
		resolvedCompany = newCompanyID
	}

	var contactID any
	if email != "" || name != "" {
		var cid string
		err = tx.QueryRow(r.Context(), `
			INSERT INTO contacts (organization_id, company_id, name, email)
			VALUES ($1,$2,$3,$4) RETURNING id`,
			user.OrganizationID, resolvedCompany, name, email).Scan(&cid)
		if err == nil {
			contactID = cid
		}
	}

	var closeDate any
	if strings.TrimSpace(input.CloseDate) != "" {
		closeDate = input.CloseDate
	}
	prob := stageProbability(input.Stage)
	var oppID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO opportunities (organization_id, company_id, contact_id, lead_id, name, stage, amount, currency, probability, owner_id, close_date)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		user.OrganizationID, resolvedCompany, contactID, leadID, name+" deal", input.Stage, input.Amount, input.Currency, prob, user.ID, closeDate,
	).Scan(&oppID)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create opportunity")
		return
	}
	_, _ = tx.Exec(r.Context(), `
		INSERT INTO opportunity_stage_history (organization_id, opportunity_id, from_stage, to_stage, note, changed_by)
		VALUES ($1,$2,NULL,$3,'Converted from lead',$4)`,
		user.OrganizationID, oppID, input.Stage, user.ID)
	_, err = tx.Exec(r.Context(), `
		UPDATE leads SET status='converted', converted_opportunity_id=$1, converted_at=NOW(), company_id=COALESCE($2::uuid, company_id)
		WHERE id=$3 AND organization_id=$4`,
		oppID, resolvedCompany, leadID, user.OrganizationID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "update_failed", "could not mark lead converted")
		return
	}
	_, _ = tx.Exec(r.Context(), `
		INSERT INTO crm_activities (organization_id, entity_type, entity_id, kind, note, created_by)
		VALUES ($1,'lead',$2,'note','Converted to opportunity',$3),
		       ($1,'opportunity',$4,'note','Created from lead conversion',$3)`,
		user.OrganizationID, leadID, user.ID, oppID)
	if err := tx.Commit(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not convert lead")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'crm.lead_converted','lead',$3,$4)`,
		user.OrganizationID, user.ID, leadID, map[string]any{"opportunity_id": oppID})
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"status": "converted", "opportunity_id": oppID})
}

// StageHistory lists stage changes for an opportunity.
func (h Handler) StageHistory(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	oppID := r.PathValue("id")
	rows, err := h.DB.Query(r.Context(), `
		SELECT h.id, COALESCE(h.from_stage,''), h.to_stage, h.note, COALESCE(u.display_name,''), h.changed_at::text
		FROM opportunity_stage_history h
		LEFT JOIN users u ON u.id=h.changed_by
		WHERE h.organization_id=$1 AND h.opportunity_id=$2
		ORDER BY h.changed_at DESC LIMIT 100`, user.OrganizationID, oppID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load stage history")
		return
	}
	defer rows.Close()
	type item struct {
		ID        string `json:"id"`
		FromStage string `json:"from_stage"`
		ToStage   string `json:"to_stage"`
		Note      string `json:"note"`
		ChangedBy string `json:"changed_by"`
		ChangedAt string `json:"changed_at"`
	}
	items := make([]item, 0)
	for rows.Next() {
		var row item
		if rows.Scan(&row.ID, &row.FromStage, &row.ToStage, &row.Note, &row.ChangedBy, &row.ChangedAt) == nil {
			items = append(items, row)
		}
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}
