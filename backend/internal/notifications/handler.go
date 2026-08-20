package notifications

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"name/backend/internal/auth"
	"name/backend/internal/httpapi"
)

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}

type Notification struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	Title      string  `json:"title"`
	Body       string  `json:"body"`
	EntityType string  `json:"entity_type"`
	EntityID   string  `json:"entity_id"`
	ReadAt     *string `json:"read_at"`
	CreatedAt  string  `json:"created_at"`
}

// List returns the caller's most recent notifications and the unread count.
func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT id, kind, title, body, entity_type, entity_id, read_at::text, created_at::text FROM notifications WHERE user_id=$1 ORDER BY created_at DESC LIMIT 100`, user.ID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load notifications")
		return
	}
	defer rows.Close()
	items := make([]Notification, 0)
	unread := 0
	for rows.Next() {
		var item Notification
		if err := rows.Scan(&item.ID, &item.Kind, &item.Title, &item.Body, &item.EntityType, &item.EntityID, &item.ReadAt, &item.CreatedAt); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read notifications")
			return
		}
		if item.ReadAt == nil {
			unread++
		}
		items = append(items, item)
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"notifications": items, "unread": unread})
}

type markReadRequest struct {
	ID  string `json:"id"`
	All bool   `json:"all"`
}

// MarkRead marks one notification read, or all of the caller's when all is true.
func (h Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var input markReadRequest
	_ = json.NewDecoder(r.Body).Decode(&input)
	if input.All {
		if _, err := h.DB.Exec(r.Context(), `UPDATE notifications SET read_at=NOW() WHERE user_id=$1 AND read_at IS NULL`, user.ID); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "update_failed", "could not update notifications")
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "all_read"})
		return
	}
	if input.ID == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "id or all is required")
		return
	}
	if _, err := h.DB.Exec(r.Context(), `UPDATE notifications SET read_at=NOW() WHERE id=$1 AND user_id=$2`, input.ID, user.ID); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "update_failed", "could not update notification")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "read"})
}
