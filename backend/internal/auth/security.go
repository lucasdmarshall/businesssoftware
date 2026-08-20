package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"name/backend/internal/mfa"
)

// revokeUserSessions revokes every active session for a user. Used on password
// changes and offboarding so access is cut immediately.
func (h Handler) revokeUserSessions(r *http.Request, userID string) {
	_, _ = h.DB.Exec(r.Context(), `UPDATE sessions SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
}

func (h Handler) audit(r *http.Request, orgID, actorID, action, entityType, entityID string, metadata map[string]any) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,$3,$4,$5,$6)`, orgID, actorID, action, entityType, entityID, metadata)
}

// EnrollMFA starts TOTP enrollment for the authenticated user and returns the
// secret and otpauth URI. Enrollment is not active until VerifyMFA confirms a
// code.
func (h Handler) EnrollMFA(w http.ResponseWriter, r *http.Request) {
	user, err := h.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	secret, err := mfa.GenerateSecret()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not start enrollment"})
		return
	}
	if _, err := h.DB.Exec(r.Context(), `INSERT INTO user_mfa (user_id, secret, enabled) VALUES ($1,$2,FALSE) ON CONFLICT (user_id) DO UPDATE SET secret=EXCLUDED.secret, enabled=FALSE, updated_at=NOW()`, user.ID, secret); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save enrollment"})
		return
	}
	var email, orgName string
	_ = h.DB.QueryRow(r.Context(), `SELECT u.email, o.name FROM users u JOIN organizations o ON o.id=u.organization_id WHERE u.id=$1`, user.ID).Scan(&email, &orgName)
	if orgName == "" {
		orgName = "Name"
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "otpauth_uri": mfa.OTPAuthURI(orgName, email, secret)})
}

type mfaCodeRequest struct {
	Code string `json:"code"`
}

// VerifyMFA confirms an enrollment by validating the first code, then enables MFA.
func (h Handler) VerifyMFA(w http.ResponseWriter, r *http.Request) {
	user, err := h.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var input mfaCodeRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Code) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a verification code is required"})
		return
	}
	var secret string
	if err := h.DB.QueryRow(r.Context(), `SELECT secret FROM user_mfa WHERE user_id=$1`, user.ID).Scan(&secret); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "start enrollment first"})
		return
	}
	if !mfa.Validate(secret, input.Code, time.Now()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid verification code"})
		return
	}
	if _, err := h.DB.Exec(r.Context(), `UPDATE user_mfa SET enabled=TRUE, updated_at=NOW() WHERE user_id=$1`, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not enable MFA"})
		return
	}
	h.audit(r, user.OrganizationID, user.ID, "mfa.enabled", "user", user.ID, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "enabled"})
}

// DisableMFA turns MFA off after re-validating a current code.
func (h Handler) DisableMFA(w http.ResponseWriter, r *http.Request) {
	user, err := h.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var input mfaCodeRequest
	_ = json.NewDecoder(r.Body).Decode(&input)
	var secret string
	if err := h.DB.QueryRow(r.Context(), `SELECT secret FROM user_mfa WHERE user_id=$1 AND enabled=TRUE`, user.ID).Scan(&secret); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "MFA is not enabled"})
		return
	}
	if !mfa.Validate(secret, input.Code, time.Now()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid verification code"})
		return
	}
	if _, err := h.DB.Exec(r.Context(), `DELETE FROM user_mfa WHERE user_id=$1`, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not disable MFA"})
		return
	}
	h.audit(r, user.OrganizationID, user.ID, "mfa.disabled", "user", user.ID, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword lets the authenticated user rotate their own password after
// proving the current one. The current session is kept; other sessions are
// revoked.
func (h Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user, err := h.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var input changePasswordRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	var currentHash string
	if err := h.DB.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id=$1`, user.ID).Scan(&currentHash); err != nil || !VerifyPassword(input.CurrentPassword, currentHash) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "current password is incorrect"})
		return
	}
	newHash, err := HashPassword(input.NewPassword)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if _, err := h.DB.Exec(r.Context(), `UPDATE users SET password_hash=$1, updated_at=NOW() WHERE id=$2`, newHash, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update password"})
		return
	}
	// Revoke other sessions but keep the one making this request.
	if cookie, cookieErr := r.Cookie(sessionCookieName); cookieErr == nil {
		sum := sha256.Sum256([]byte(cookie.Value))
		_, _ = h.DB.Exec(r.Context(), `UPDATE sessions SET revoked_at=NOW() WHERE user_id=$1 AND token_hash<>$2 AND revoked_at IS NULL`, user.ID, hex.EncodeToString(sum[:]))
	}
	h.audit(r, user.OrganizationID, user.ID, "auth.password_changed", "user", user.ID, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "changed"})
}

type resetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// ResetPassword lets an administrator set a new password for another user in the
// same organization and revokes that user's sessions. Gated by users.manage.
func (h Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	actor, err := h.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	targetID := r.PathValue("id")
	var input resetPasswordRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	newHash, err := HashPassword(input.NewPassword)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result, err := h.DB.Exec(r.Context(), `UPDATE users SET password_hash=$1, updated_at=NOW() WHERE id=$2 AND organization_id=$3`, newHash, targetID, actor.OrganizationID)
	if err != nil || result.RowsAffected() == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user does not belong to this organization"})
		return
	}
	h.revokeUserSessions(r, targetID)
	h.audit(r, actor.OrganizationID, actor.ID, "auth.password_reset", "user", targetID, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

// OffboardUser immediately revokes a user's access: status becomes offboarded,
// all sessions are revoked, role assignments removed, and MFA cleared. Gated by
// users.manage. A user cannot offboard themselves.
func (h Handler) OffboardUser(w http.ResponseWriter, r *http.Request) {
	actor, err := h.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	targetID := r.PathValue("id")
	if targetID == actor.ID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "you cannot offboard yourself"})
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not begin offboarding"})
		return
	}
	defer tx.Rollback(r.Context())
	result, err := tx.Exec(r.Context(), `UPDATE users SET status='offboarded', offboarded_at=NOW(), updated_at=NOW() WHERE id=$1 AND organization_id=$2 AND status<>'offboarded'`, targetID, actor.OrganizationID)
	if err != nil || result.RowsAffected() == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user not found or already offboarded"})
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE sessions SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`, targetID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not revoke sessions"})
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM user_roles WHERE user_id=$1`, targetID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not remove roles"})
		return
	}
	_, _ = tx.Exec(r.Context(), `DELETE FROM user_mfa WHERE user_id=$1`, targetID)
	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not complete offboarding"})
		return
	}
	h.audit(r, actor.OrganizationID, actor.ID, "user.offboarded", "user", targetID, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "offboarded"})
}

type unassignRoleRequest struct {
	UserID string `json:"user_id"`
	RoleID string `json:"role_id"`
}

// UnassignRole removes a permission role from a user and records the change for
// the permission-change audit history. Gated by roles.manage.
func (h Handler) UnassignRole(w http.ResponseWriter, r *http.Request) {
	actor, err := h.Authenticate(r)
	if err != nil || h.DB == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var input unassignRoleRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.UserID == "" || input.RoleID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id and role_id are required"})
		return
	}
	result, err := h.DB.Exec(r.Context(), `DELETE FROM user_roles ur USING roles ro WHERE ur.role_id=ro.id AND ur.user_id=$1 AND ur.role_id=$2 AND ro.organization_id=$3`, input.UserID, input.RoleID, actor.OrganizationID)
	if err != nil || result.RowsAffected() == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role assignment not found in this organization"})
		return
	}
	h.audit(r, actor.OrganizationID, actor.ID, "role.unassigned", "user", input.UserID, map[string]any{"role_id": input.RoleID})
	writeJSON(w, http.StatusOK, map[string]string{"status": "unassigned"})
}
