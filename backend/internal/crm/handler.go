package crm

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"name/backend/internal/auth"
	"name/backend/internal/httpapi"
)

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}

// Companies lists (GET) or creates (POST) companies.
func (h Handler) Companies(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `SELECT id, name, industry, website FROM companies WHERE organization_id=$1 ORDER BY name`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load companies")
			return
		}
		defer rows.Close()
		type company struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Industry string `json:"industry"`
			Website  string `json:"website"`
		}
		items := make([]company, 0)
		for rows.Next() {
			var item company
			if err := rows.Scan(&item.ID, &item.Name, &item.Industry, &item.Website); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read companies")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		Name     string `json:"name"`
		Industry string `json:"industry"`
		Website  string `json:"website"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	var id string
	if err := h.DB.QueryRow(r.Context(), `INSERT INTO companies (organization_id, name, industry, website, created_by) VALUES ($1,$2,$3,$4,$5) RETURNING id`, user.OrganizationID, strings.TrimSpace(input.Name), strings.TrimSpace(input.Industry), strings.TrimSpace(input.Website), user.ID).Scan(&id); err != nil {
		httpapi.WriteError(w, http.StatusConflict, "conflict", "company may already exist")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// Contacts lists (GET) or creates (POST) contacts.
func (h Handler) Contacts(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `SELECT c.id, c.name, c.email, c.phone, c.title, COALESCE(c.company_id::text,''), COALESCE(co.name,'') FROM contacts c LEFT JOIN companies co ON co.id=c.company_id WHERE c.organization_id=$1 ORDER BY c.name`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load contacts")
			return
		}
		defer rows.Close()
		type contact struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Email       string `json:"email"`
			Phone       string `json:"phone"`
			Title       string `json:"title"`
			CompanyID   string `json:"company_id"`
			CompanyName string `json:"company_name"`
		}
		items := make([]contact, 0)
		for rows.Next() {
			var item contact
			if err := rows.Scan(&item.ID, &item.Name, &item.Email, &item.Phone, &item.Title, &item.CompanyID, &item.CompanyName); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read contacts")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		Name      string `json:"name"`
		Email     string `json:"email"`
		Phone     string `json:"phone"`
		Title     string `json:"title"`
		CompanyID string `json:"company_id"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	var companyID any
	if strings.TrimSpace(input.CompanyID) != "" {
		companyID = input.CompanyID
	}
	var id string
	if err := h.DB.QueryRow(r.Context(), `
		INSERT INTO contacts (organization_id, company_id, name, email, phone, title)
		SELECT $1,$2,$3,$4,$5,$6 WHERE $2::uuid IS NULL OR EXISTS (SELECT 1 FROM companies co WHERE co.id=$2 AND co.organization_id=$1)
		RETURNING id`, user.OrganizationID, companyID, strings.TrimSpace(input.Name), strings.TrimSpace(input.Email), strings.TrimSpace(input.Phone), strings.TrimSpace(input.Title)).Scan(&id); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create contact")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// Leads lists (GET) or creates (POST) leads; POST with id+status updates status.
func (h Handler) Leads(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `SELECT l.id, l.name, l.contact_email, l.source, l.status, COALESCE(o.display_name,''), COALESCE(l.company_id::text,''), COALESCE(l.converted_opportunity_id::text,'') FROM leads l LEFT JOIN users o ON o.id=l.owner_id WHERE l.organization_id=$1 ORDER BY l.created_at DESC LIMIT 500`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load leads")
			return
		}
		defer rows.Close()
		type lead struct {
			ID                     string `json:"id"`
			Name                   string `json:"name"`
			Email                  string `json:"contact_email"`
			Source                 string `json:"source"`
			Status                 string `json:"status"`
			OwnerName              string `json:"owner_name"`
			CompanyID              string `json:"company_id"`
			ConvertedOpportunityID string `json:"converted_opportunity_id"`
		}
		items := make([]lead, 0)
		for rows.Next() {
			var item lead
			if err := rows.Scan(&item.ID, &item.Name, &item.Email, &item.Source, &item.Status, &item.OwnerName, &item.CompanyID, &item.ConvertedOpportunityID); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read leads")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Email     string `json:"contact_email"`
		Source    string `json:"source"`
		Status    string `json:"status"`
		CompanyID string `json:"company_id"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	if input.ID != "" {
		valid := map[string]bool{"new": true, "qualified": true, "converted": true, "lost": true}
		if !valid[input.Status] {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid status")
			return
		}
		if _, err := h.DB.Exec(r.Context(), `UPDATE leads SET status=$1 WHERE id=$2 AND organization_id=$3`, input.Status, input.ID, user.OrganizationID); err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "update_failed", "could not update lead")
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": input.Status})
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if input.Source == "" {
		input.Source = "other"
	}
	var companyID any
	if strings.TrimSpace(input.CompanyID) != "" {
		companyID = input.CompanyID
	}
	var id string
	if err := h.DB.QueryRow(r.Context(), `INSERT INTO leads (organization_id, name, contact_email, source, owner_id, company_id) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, user.OrganizationID, strings.TrimSpace(input.Name), strings.TrimSpace(input.Email), input.Source, user.ID, companyID).Scan(&id); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create lead")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// Opportunities lists (GET) or creates (POST) opportunities; POST with id+stage
// advances the pipeline stage (also writes history when table exists).
func (h Handler) Opportunities(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		stageFilter := r.URL.Query().Get("stage")
		query := `
			SELECT o.id, o.name, o.stage, o.amount, o.currency, COALESCE(o.probability,10), COALESCE(o.close_date::text,''),
			       COALESCE(o.company_id::text,''), COALESCE(co.name,''), COALESCE(u.display_name,''),
			       COALESCE(o.lead_id::text,''), o.updated_at::text
			FROM opportunities o
			LEFT JOIN companies co ON co.id=o.company_id
			LEFT JOIN users u ON u.id=o.owner_id
			WHERE o.organization_id=$1`
		args := []any{user.OrganizationID}
		if stageFilter != "" {
			query += ` AND o.stage=$2`
			args = append(args, stageFilter)
		}
		query += ` ORDER BY o.updated_at DESC LIMIT 500`
		rows, err := h.DB.Query(r.Context(), query, args...)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load opportunities")
			return
		}
		defer rows.Close()
		items := make([]Opportunity, 0)
		for rows.Next() {
			var item Opportunity
			if err := rows.Scan(&item.ID, &item.Name, &item.Stage, &item.Amount, &item.Currency, &item.Probability, &item.CloseDate,
				&item.CompanyID, &item.CompanyName, &item.OwnerName, &item.LeadID, &item.UpdatedAt); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read opportunities")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		Stage     string  `json:"stage"`
		Amount    float64 `json:"amount"`
		Currency  string  `json:"currency"`
		CompanyID string  `json:"company_id"`
		CloseDate string  `json:"close_date"`
		Note      string  `json:"note"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	if input.ID != "" {
		// Prefer dedicated MoveStage route; keep POST update for backward compatibility.
		if !validStages[input.Stage] {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid stage")
			return
		}
		var fromStage string
		if err := h.DB.QueryRow(r.Context(), `SELECT stage FROM opportunities WHERE id=$1 AND organization_id=$2`, input.ID, user.OrganizationID).Scan(&fromStage); err != nil {
			httpapi.WriteError(w, http.StatusNotFound, "not_found", "opportunity not found")
			return
		}
		prob := stageProbability(input.Stage)
		var closeDate any
		if strings.TrimSpace(input.CloseDate) != "" {
			closeDate = input.CloseDate
		} else if input.Stage == "won" || input.Stage == "lost" {
			closeDate = "now"
		}
		tx, err := h.DB.Begin(r.Context())
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "tx_failed", "could not update stage")
			return
		}
		defer tx.Rollback(r.Context())
		if closeDate == "now" {
			_, err = tx.Exec(r.Context(), `UPDATE opportunities SET stage=$1, probability=$2, close_date=CURRENT_DATE, updated_at=NOW() WHERE id=$3 AND organization_id=$4`, input.Stage, prob, input.ID, user.OrganizationID)
		} else {
			_, err = tx.Exec(r.Context(), `UPDATE opportunities SET stage=$1, probability=$2, close_date=COALESCE($3::date, close_date), updated_at=NOW() WHERE id=$4 AND organization_id=$5`, input.Stage, prob, closeDate, input.ID, user.OrganizationID)
		}
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "update_failed", "could not update opportunity")
			return
		}
		if fromStage != input.Stage {
			_, _ = tx.Exec(r.Context(), `INSERT INTO opportunity_stage_history (organization_id, opportunity_id, from_stage, to_stage, note, changed_by) VALUES ($1,$2,$3,$4,$5,$6)`,
				user.OrganizationID, input.ID, fromStage, input.Stage, strings.TrimSpace(input.Note), user.ID)
			note := strings.TrimSpace(input.Note)
			if note == "" {
				note = "Stage moved from " + fromStage + " to " + input.Stage
			}
			_, _ = tx.Exec(r.Context(), `INSERT INTO crm_activities (organization_id, entity_type, entity_id, kind, note, created_by) VALUES ($1,'opportunity',$2,'note',$3,$4)`,
				user.OrganizationID, input.ID, note, user.ID)
		}
		if err := tx.Commit(r.Context()); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not save stage")
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, map[string]any{"stage": input.Stage, "probability": prob})
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	stage := input.Stage
	if stage == "" {
		stage = "prospect"
	}
	if !validStages[stage] {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid stage")
		return
	}
	var companyID, closeDate any
	if strings.TrimSpace(input.CompanyID) != "" {
		companyID = input.CompanyID
	}
	if strings.TrimSpace(input.CloseDate) != "" {
		closeDate = input.CloseDate
	}
	prob := stageProbability(stage)
	var id string
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO opportunities (organization_id, company_id, name, stage, amount, currency, probability, owner_id, close_date)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		user.OrganizationID, companyID, strings.TrimSpace(input.Name), stage, input.Amount, input.Currency, prob, user.ID, closeDate).Scan(&id)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create opportunity")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO opportunity_stage_history (organization_id, opportunity_id, from_stage, to_stage, note, changed_by) VALUES ($1,$2,NULL,$3,'Created',$4)`,
		user.OrganizationID, id, stage, user.ID)
	httpapi.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "stage": stage, "probability": prob})
}

// Activities lists (GET, filtered by entity) or logs (POST) a CRM activity.
func (h Handler) Activities(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `SELECT a.id, a.entity_type, a.entity_id::text, a.kind, a.note, COALESCE(u.display_name,''), a.created_at::text FROM crm_activities a LEFT JOIN users u ON u.id=a.created_by WHERE a.organization_id=$1 AND ($2='' OR a.entity_id=$2::uuid) ORDER BY a.created_at DESC LIMIT 200`, user.OrganizationID, r.URL.Query().Get("entity_id"))
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load activities")
			return
		}
		defer rows.Close()
		type activity struct {
			ID         string `json:"id"`
			EntityType string `json:"entity_type"`
			EntityID   string `json:"entity_id"`
			Kind       string `json:"kind"`
			Note       string `json:"note"`
			AuthorName string `json:"author_name"`
			CreatedAt  string `json:"created_at"`
		}
		items := make([]activity, 0)
		for rows.Next() {
			var item activity
			if err := rows.Scan(&item.ID, &item.EntityType, &item.EntityID, &item.Kind, &item.Note, &item.AuthorName, &item.CreatedAt); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read activities")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		Kind       string `json:"kind"`
		Note       string `json:"note"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.EntityID == "" || strings.TrimSpace(input.Note) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "entity_id and note are required")
		return
	}
	if input.EntityType == "" {
		input.EntityType = "opportunity"
	}
	if input.Kind == "" {
		input.Kind = "note"
	}
	var id string
	if err := h.DB.QueryRow(r.Context(), `INSERT INTO crm_activities (organization_id, entity_type, entity_id, kind, note, created_by) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, user.OrganizationID, input.EntityType, input.EntityID, input.Kind, strings.TrimSpace(input.Note), user.ID).Scan(&id); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not log activity")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}
