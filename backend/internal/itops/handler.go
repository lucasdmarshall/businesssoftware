package itops

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

// Tickets lists (GET) or creates (POST) IT tickets, which cover service and
// access requests and incidents via the type field. POST with id updates status.
func (h Handler) Tickets(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `SELECT t.id, t.type, t.title, t.description, t.priority, t.status, u.display_name, COALESCE(a.display_name,'') FROM it_tickets t JOIN users u ON u.id=t.requested_by LEFT JOIN users a ON a.id=t.assigned_to WHERE t.organization_id=$1 ORDER BY t.updated_at DESC LIMIT 500`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load tickets")
			return
		}
		defer rows.Close()
		type ticket struct {
			ID            string `json:"id"`
			Type          string `json:"type"`
			Title         string `json:"title"`
			Description   string `json:"description"`
			Priority      string `json:"priority"`
			Status        string `json:"status"`
			RequesterName string `json:"requester_name"`
			AssigneeName  string `json:"assignee_name"`
		}
		items := make([]ticket, 0)
		for rows.Next() {
			var item ticket
			if err := rows.Scan(&item.ID, &item.Type, &item.Title, &item.Description, &item.Priority, &item.Status, &item.RequesterName, &item.AssigneeName); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read tickets")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Priority    string `json:"priority"`
		Status      string `json:"status"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	if input.ID != "" {
		valid := map[string]bool{"open": true, "in_progress": true, "resolved": true, "closed": true}
		if !valid[input.Status] {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid status")
			return
		}
		if _, err := h.DB.Exec(r.Context(), `UPDATE it_tickets SET status=$1, updated_at=NOW() WHERE id=$2 AND organization_id=$3`, input.Status, input.ID, user.OrganizationID); err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "update_failed", "could not update ticket")
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": input.Status})
		return
	}
	if strings.TrimSpace(input.Title) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}
	validType := map[string]bool{"ticket": true, "service_request": true, "access_request": true, "incident": true}
	if !validType[input.Type] {
		input.Type = "ticket"
	}
	if input.Priority == "" {
		input.Priority = "normal"
	}
	var id string
	if err := h.DB.QueryRow(r.Context(), `INSERT INTO it_tickets (organization_id, requested_by, type, title, description, priority) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, user.OrganizationID, user.ID, input.Type, strings.TrimSpace(input.Title), strings.TrimSpace(input.Description), input.Priority).Scan(&id); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create ticket")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// Assets lists (GET) or creates (POST) registry assets.
func (h Handler) Assets(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `SELECT a.id, a.name, a.category, a.serial_number, a.status, COALESCE(u.display_name,'') FROM assets a LEFT JOIN users u ON u.id=a.assigned_to WHERE a.organization_id=$1 ORDER BY a.name`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load assets")
			return
		}
		defer rows.Close()
		type asset struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			Category     string `json:"category"`
			SerialNumber string `json:"serial_number"`
			Status       string `json:"status"`
			AssigneeName string `json:"assignee_name"`
		}
		items := make([]asset, 0)
		for rows.Next() {
			var item asset
			if err := rows.Scan(&item.ID, &item.Name, &item.Category, &item.SerialNumber, &item.Status, &item.AssigneeName); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read assets")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		Name         string `json:"name"`
		Category     string `json:"category"`
		SerialNumber string `json:"serial_number"`
		AssignedTo   string `json:"assigned_to"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if input.Category == "" {
		input.Category = "hardware"
	}
	var assignedTo any
	status := "available"
	if strings.TrimSpace(input.AssignedTo) != "" {
		assignedTo = input.AssignedTo
		status = "in_use"
	}
	var id string
	if err := h.DB.QueryRow(r.Context(), `
		INSERT INTO assets (organization_id, name, category, serial_number, assigned_to, status)
		SELECT $1,$2,$3,$4,$5,$6 WHERE $5::uuid IS NULL OR EXISTS (SELECT 1 FROM users u WHERE u.id=$5 AND u.organization_id=$1)
		RETURNING id`, user.OrganizationID, strings.TrimSpace(input.Name), input.Category, strings.TrimSpace(input.SerialNumber), assignedTo, status).Scan(&id); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create asset")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// Articles lists (GET) or creates (POST) knowledge-base articles.
func (h Handler) Articles(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `SELECT a.id, a.title, a.body, a.category, COALESCE(u.display_name,''), a.updated_at::text FROM kb_articles a LEFT JOIN users u ON u.id=a.created_by WHERE a.organization_id=$1 ORDER BY a.updated_at DESC LIMIT 500`, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load articles")
			return
		}
		defer rows.Close()
		type article struct {
			ID         string `json:"id"`
			Title      string `json:"title"`
			Body       string `json:"body"`
			Category   string `json:"category"`
			AuthorName string `json:"author_name"`
			UpdatedAt  string `json:"updated_at"`
		}
		items := make([]article, 0)
		for rows.Next() {
			var item article
			if err := rows.Scan(&item.ID, &item.Title, &item.Body, &item.Category, &item.AuthorName, &item.UpdatedAt); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read articles")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	var input struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		Category string `json:"category"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Title) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}
	if input.Category == "" {
		input.Category = "general"
	}
	var id string
	if err := h.DB.QueryRow(r.Context(), `INSERT INTO kb_articles (organization_id, title, body, category, created_by) VALUES ($1,$2,$3,$4,$5) RETURNING id`, user.OrganizationID, strings.TrimSpace(input.Title), strings.TrimSpace(input.Body), input.Category, user.ID).Scan(&id); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create article")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}
