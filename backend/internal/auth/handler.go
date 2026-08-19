package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionCookieName = "name_session"

var ErrUnauthenticated = errors.New("unauthenticated")

type SessionUser struct {
	ID             string
	OrganizationID string
}

type Handler struct {
	DB *pgxpool.Pool
}

type setupStatusResponse struct {
	NeedsSetup bool `json:"needs_setup"`
}

type setupRequest struct {
	OrganizationName string `json:"organization_name"`
	OrganizationSlug string `json:"organization_slug"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	Password         string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Email          string   `json:"email"`
	OrganizationID string   `json:"organization_id"`
	Organization   string   `json:"organization"`
	Role           string   `json:"role"`
	Permissions    []string `json:"permissions"`
}

func (h Handler) SetupStatus(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database is not configured"})
		return
	}
	var count int
	if err := h.DB.QueryRow(r.Context(), `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read setup status"})
		return
	}
	writeJSON(w, http.StatusOK, setupStatusResponse{NeedsSetup: count == 0})
}

func (h Handler) Setup(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database is not configured"})
		return
	}
	var input setupRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.OrganizationName) == "" || strings.TrimSpace(input.OrganizationSlug) == "" || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Email) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "organization, name, and email are required"})
		return
	}
	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not begin setup"})
		return
	}
	defer tx.Rollback(r.Context())

	var existing int
	if err := tx.QueryRow(r.Context(), `SELECT COUNT(*) FROM users`).Scan(&existing); err != nil || existing > 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "setup has already been completed"})
		return
	}

	var organizationID, userID, roleID string
	if err := tx.QueryRow(r.Context(), `INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id`, strings.TrimSpace(input.OrganizationName), strings.TrimSpace(input.OrganizationSlug)).Scan(&organizationID); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "organization slug may already exist"})
		return
	}
	if err := tx.QueryRow(r.Context(), `INSERT INTO users (organization_id, email, display_name, password_hash) VALUES ($1, lower($2), $3, $4) RETURNING id`, organizationID, strings.TrimSpace(input.Email), strings.TrimSpace(input.Name), passwordHash).Scan(&userID); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "could not create owner account"})
		return
	}
	if err := tx.QueryRow(r.Context(), `INSERT INTO roles (organization_id, code, name) VALUES ($1, 'owner', 'Owner') RETURNING id`, organizationID).Scan(&roleID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create owner role"})
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO role_permissions (role_id, permission_code) SELECT $1, code FROM permissions`, roleID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not assign owner permissions"})
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, userID, roleID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not assign owner role"})
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not complete setup"})
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'organization.setup','organization',$1,$3)`, organizationID, userID, map[string]any{"email": input.Email})
	writeJSON(w, http.StatusCreated, map[string]string{"organization_id": organizationID, "user_id": userID})
}

func (h Handler) Login(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database is not configured"})
		return
	}
	var input loginRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid login request"})
		return
	}
	var userID, organizationID, passwordHash string
	if err := h.DB.QueryRow(r.Context(), `SELECT id, organization_id, password_hash FROM users WHERE email = lower($1) AND status = 'active'`, strings.TrimSpace(input.Email)).Scan(&userID, &organizationID, &passwordHash); err != nil || passwordHash == "" || !VerifyPassword(input.Password, passwordHash) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid email or password"})
		return
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create session"})
		return
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])
	if _, err := h.DB.Exec(r.Context(), `INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`, userID, tokenHash, time.Now().Add(24*time.Hour)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save session"})
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id) VALUES ($1,$2,'auth.login','session',$2)`, organizationID, userID)
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 24 * 60 * 60})
	writeJSON(w, http.StatusOK, map[string]string{"status": "authenticated"})
}

func (h Handler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" && h.DB != nil {
		hash := sha256.Sum256([]byte(cookie.Value))
		var userID, organizationID string
		_ = h.DB.QueryRow(r.Context(), `SELECT u.id, u.organization_id FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1`, hex.EncodeToString(hash[:])).Scan(&userID, &organizationID)
		_, _ = h.DB.Exec(r.Context(), `UPDATE sessions SET revoked_at = NOW() WHERE token_hash = $1`, hex.EncodeToString(hash[:]))
		if userID != "" {
			_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id) VALUES ($1,$2,'auth.logout','session',$2)`, organizationID, userID)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (h Handler) Authenticate(r *http.Request) (SessionUser, error) {
	if h.DB == nil {
		return SessionUser{}, ErrUnauthenticated
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return SessionUser{}, ErrUnauthenticated
	}
	hash := sha256.Sum256([]byte(cookie.Value))
	var user SessionUser
	err = h.DB.QueryRow(r.Context(), `
		SELECT u.id, u.organization_id
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > NOW() AND u.status = 'active'`, hex.EncodeToString(hash[:])).Scan(&user.ID, &user.OrganizationID)
	if err != nil {
		return SessionUser{}, ErrUnauthenticated
	}
	return user, nil
}

func (h Handler) RequirePermission(permission string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := h.Authenticate(r)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		var allowed bool
		err = h.DB.QueryRow(r.Context(), `
			SELECT EXISTS (
				SELECT 1 FROM user_roles ur
				JOIN roles ro ON ro.id = ur.role_id
				JOIN role_permissions rp ON rp.role_id = ro.id
				WHERE ur.user_id = $1 AND ro.organization_id = $2 AND rp.permission_code = $3
			)`, user.ID, user.OrganizationID, permission).Scan(&allowed)
		if err != nil || !allowed {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h Handler) Me(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database is not configured"})
		return
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	hash := sha256.Sum256([]byte(cookie.Value))
	tokenHash := hex.EncodeToString(hash[:])
	var response userResponse
	err = h.DB.QueryRow(r.Context(), `
		SELECT u.id, u.display_name, u.email, u.organization_id, o.name, COALESCE(MIN(r.name), 'Member')
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		JOIN organizations o ON o.id = u.organization_id
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		LEFT JOIN role_permissions rp ON rp.role_id = r.id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > NOW()
		GROUP BY u.id, u.display_name, u.email, u.organization_id, o.name`, tokenHash).Scan(&response.ID, &response.Name, &response.Email, &response.OrganizationID, &response.Organization, &response.Role, &response.Permissions)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session expired"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load current user"})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
