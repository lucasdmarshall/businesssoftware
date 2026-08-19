package audit

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"name/backend/internal/auth"
)

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}
type Entry struct {
	ID         string         `json:"id"`
	Action     string         `json:"action"`
	EntityType string         `json:"entity_type"`
	EntityID   string         `json:"entity_id"`
	ActorName  string         `json:"actor_name"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  string         `json:"created_at"`
}

func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT a.id,a.action,a.entity_type,a.entity_id,COALESCE(u.display_name,'System'),a.metadata,a.created_at::text FROM audit_logs a LEFT JOIN users u ON u.id=a.actor_id WHERE a.organization_id=$1 ORDER BY a.created_at DESC LIMIT 500`, user.OrganizationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load audit logs"})
		return
	}
	defer rows.Close()
	items := make([]Entry, 0)
	for rows.Next() {
		var item Entry
		var raw []byte
		if err := rows.Scan(&item.ID, &item.Action, &item.EntityType, &item.EntityID, &item.ActorName, &raw, &item.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read audit logs"})
			return
		}
		_ = json.Unmarshal(raw, &item.Metadata)
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
