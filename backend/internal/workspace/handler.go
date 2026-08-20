package workspace

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"name/backend/internal/auth"
	"name/backend/internal/httpapi"
)

// Module codes match the department workspace nav.
var AllModules = []string{
	"overview", "users", "access", "attendance", "calendar", "leave",
	"schedule", "salary", "bonus", "tasks", "activity", "settings",
}

var memberDefaultModules = []string{
	"overview", "attendance", "calendar", "leave", "schedule", "tasks", "activity",
}

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}

type DepartmentWorkspace struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Slug        string         `json:"slug"`
	IsHead      bool           `json:"is_head"`
	CompanyWide bool           `json:"company_wide"`
	Modules     []ModuleGrant  `json:"modules"`
}

type ModuleGrant struct {
	Code      string `json:"code"`
	CanView   bool   `json:"can_view"`
	CanManage bool   `json:"can_manage"`
}

type Member struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	EmployeeID  string `json:"employee_id"`
	IsHead      bool   `json:"is_head"`
	IsPrimary   bool   `json:"is_primary"`
	Status      string `json:"status"`
}

type AccessRow struct {
	UserID      string        `json:"user_id"`
	DisplayName string        `json:"display_name"`
	IsHead      bool          `json:"is_head"`
	Modules     []ModuleGrant `json:"modules"`
}

type SalaryRow struct {
	ID        string  `json:"id"`
	UserID    string  `json:"user_id"`
	UserName  string  `json:"user_name"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
	Month     string  `json:"month"`
	Withdrawn bool    `json:"withdrawn"`
	Note      string  `json:"note"`
}

type BonusRow struct {
	ID         string  `json:"id"`
	PublicID   string  `json:"public_id"`
	UserID     string  `json:"user_id"`
	UserName   string  `json:"user_name"`
	RoleLabel  string  `json:"role_label"`
	Privilege  string  `json:"privilege"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	DebitedOn  string  `json:"debited_on"`
}

func (h Handler) companyWide(ctx context.Context, user auth.SessionUser) bool {
	return h.Auth.HasPermission(ctx, user, "company.departments.access") ||
		h.Auth.HasPermission(ctx, user, "organization.manage")
}

func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	wide := h.companyWide(r.Context(), user)
	var rows interface {
		Next() bool
		Scan(dest ...any) error
		Close()
		Err() error
	}
	if wide {
		q, err := h.DB.Query(r.Context(), `
			SELECT d.id, d.name, d.slug, COALESCE(ud.is_head, FALSE)
			FROM departments d
			LEFT JOIN user_departments ud ON ud.department_id=d.id AND ud.user_id=$2
			WHERE d.organization_id=$1 ORDER BY d.name`, user.OrganizationID, user.ID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load departments")
			return
		}
		rows = q
	} else {
		q, err := h.DB.Query(r.Context(), `
			SELECT d.id, d.name, d.slug, ud.is_head
			FROM departments d
			JOIN user_departments ud ON ud.department_id=d.id
			WHERE d.organization_id=$1 AND ud.user_id=$2
			ORDER BY d.name`, user.OrganizationID, user.ID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load departments")
			return
		}
		rows = q
	}
	defer rows.Close()
	items := make([]DepartmentWorkspace, 0)
	for rows.Next() {
		var item DepartmentWorkspace
		if err := rows.Scan(&item.ID, &item.Name, &item.Slug, &item.IsHead); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read departments")
			return
		}
		item.CompanyWide = wide
		item.Modules = h.modulesFor(r.Context(), user, item.ID, item.IsHead, wide)
		items = append(items, item)
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

func (h Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	deptID := r.PathValue("id")
	ws, ok := h.resolve(r.Context(), user, deptID)
	if !ok {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "you cannot enter this department workspace")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, ws)
}

func (h Handler) resolve(ctx context.Context, user auth.SessionUser, deptID string) (DepartmentWorkspace, bool) {
	var ws DepartmentWorkspace
	wide := h.companyWide(ctx, user)
	err := h.DB.QueryRow(ctx, `
		SELECT d.id, d.name, d.slug, COALESCE(ud.is_head, FALSE)
		FROM departments d
		LEFT JOIN user_departments ud ON ud.department_id=d.id AND ud.user_id=$3
		WHERE d.id=$1 AND d.organization_id=$2`, deptID, user.OrganizationID, user.ID,
	).Scan(&ws.ID, &ws.Name, &ws.Slug, &ws.IsHead)
	if err != nil {
		return ws, false
	}
	if !wide {
		var member bool
		_ = h.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_departments WHERE department_id=$1 AND user_id=$2)`, deptID, user.ID).Scan(&member)
		if !member {
			return ws, false
		}
	}
	ws.CompanyWide = wide
	ws.Modules = h.modulesFor(ctx, user, deptID, ws.IsHead, wide)
	return ws, true
}

func (h Handler) modulesFor(ctx context.Context, user auth.SessionUser, deptID string, isHead, wide bool) []ModuleGrant {
	grants := map[string]ModuleGrant{}
	if wide || isHead {
		for _, code := range AllModules {
			grants[code] = ModuleGrant{Code: code, CanView: true, CanManage: true}
		}
	} else {
		for _, code := range memberDefaultModules {
			grants[code] = ModuleGrant{Code: code, CanView: true, CanManage: false}
		}
	}
	rows, err := h.DB.Query(ctx, `
		SELECT module_code, can_view, can_manage FROM department_module_grants
		WHERE organization_id=$1 AND department_id=$2 AND user_id=$3`,
		user.OrganizationID, deptID, user.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var g ModuleGrant
			if rows.Scan(&g.Code, &g.CanView, &g.CanManage) == nil {
				grants[g.Code] = g
			}
		}
	}
	out := make([]ModuleGrant, 0, len(AllModules))
	for _, code := range AllModules {
		if g, ok := grants[code]; ok && g.CanView {
			out = append(out, g)
		}
	}
	return out
}

func (h Handler) canAccessModule(ctx context.Context, user auth.SessionUser, deptID, module string, manage bool) bool {
	ws, ok := h.resolve(ctx, user, deptID)
	if !ok {
		return false
	}
	for _, m := range ws.Modules {
		if m.Code == module {
			if manage {
				return m.CanManage
			}
			return m.CanView
		}
	}
	return false
}

func (h Handler) Members(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	deptID := r.PathValue("id")
	if !h.canAccessModule(r.Context(), user, deptID, "users", false) && !h.canAccessModule(r.Context(), user, deptID, "overview", false) {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "users module required")
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT u.id, u.display_name, u.email, COALESCE(ep.phone,''), COALESCE(ep.employee_code,''), ud.is_head, ud.is_primary, u.status
		FROM user_departments ud
		JOIN users u ON u.id=ud.user_id
		LEFT JOIN employee_profiles ep ON ep.user_id=u.id AND ep.organization_id=u.organization_id
		WHERE ud.department_id=$1 AND u.organization_id=$2
		ORDER BY u.display_name`, deptID, user.OrganizationID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load members")
		return
	}
	defer rows.Close()
	items := make([]Member, 0)
	for rows.Next() {
		var item Member
		if err := rows.Scan(&item.UserID, &item.DisplayName, &item.Email, &item.Phone, &item.EmployeeID, &item.IsHead, &item.IsPrimary, &item.Status); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read members")
			return
		}
		items = append(items, item)
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

func (h Handler) Access(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	deptID := r.PathValue("id")
	if !h.canAccessModule(r.Context(), user, deptID, "access", r.Method != http.MethodGet) {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "Access Control is limited to department heads and company-wide access")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `
			SELECT u.id, u.display_name, ud.is_head
			FROM user_departments ud JOIN users u ON u.id=ud.user_id
			WHERE ud.department_id=$1 AND u.organization_id=$2
			ORDER BY u.display_name`, deptID, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load access list")
			return
		}
		defer rows.Close()
		items := make([]AccessRow, 0)
		for rows.Next() {
			var item AccessRow
			if err := rows.Scan(&item.UserID, &item.DisplayName, &item.IsHead); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read access list")
				return
			}
			item.Modules = h.modulesForUser(r.Context(), user.OrganizationID, deptID, item.UserID, item.IsHead)
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}

	var input struct {
		UserID  string        `json:"user_id"`
		IsHead  *bool         `json:"is_head"`
		Modules []ModuleGrant `json:"modules"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.UserID == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "user_id is required")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "tx_failed", "could not update access")
		return
	}
	defer tx.Rollback(r.Context())

	if input.IsHead != nil {
		_, err = tx.Exec(r.Context(), `
			UPDATE user_departments SET is_head=$1 WHERE department_id=$2 AND user_id=$3`,
			*input.IsHead, deptID, input.UserID)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "update_failed", "user is not in this department")
			return
		}
	}
	if input.Modules != nil {
		_, _ = tx.Exec(r.Context(), `DELETE FROM department_module_grants WHERE department_id=$1 AND user_id=$2`, deptID, input.UserID)
		for _, m := range input.Modules {
			if !validModule(m.Code) {
				continue
			}
			_, err = tx.Exec(r.Context(), `
				INSERT INTO department_module_grants (organization_id, department_id, user_id, module_code, can_view, can_manage)
				VALUES ($1,$2,$3,$4,$5,$6)`,
				user.OrganizationID, deptID, input.UserID, m.Code, m.CanView, m.CanManage)
			if err != nil {
				httpapi.WriteError(w, http.StatusBadRequest, "grant_failed", "could not save module grants")
				return
			}
		}
	}
	_, _ = tx.Exec(r.Context(), `INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata) VALUES ($1,$2,'department.access_updated','department',$3,$4)`,
		user.OrganizationID, user.ID, deptID, map[string]any{"user_id": input.UserID})
	if err := tx.Commit(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not save access")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h Handler) modulesForUser(ctx context.Context, orgID, deptID, userID string, isHead bool) []ModuleGrant {
	grants := map[string]ModuleGrant{}
	if isHead {
		for _, code := range AllModules {
			grants[code] = ModuleGrant{Code: code, CanView: true, CanManage: true}
		}
	} else {
		for _, code := range memberDefaultModules {
			grants[code] = ModuleGrant{Code: code, CanView: true, CanManage: false}
		}
	}
	rows, err := h.DB.Query(ctx, `
		SELECT module_code, can_view, can_manage FROM department_module_grants
		WHERE organization_id=$1 AND department_id=$2 AND user_id=$3`, orgID, deptID, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var g ModuleGrant
			if rows.Scan(&g.Code, &g.CanView, &g.CanManage) == nil {
				grants[g.Code] = g
			}
		}
	}
	out := make([]ModuleGrant, 0, len(AllModules))
	for _, code := range AllModules {
		if g, ok := grants[code]; ok {
			out = append(out, g)
		} else {
			out = append(out, ModuleGrant{Code: code})
		}
	}
	return out
}

func (h Handler) Salaries(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	deptID := r.PathValue("id")
	if r.Method == http.MethodGet {
		if !h.canAccessModule(r.Context(), user, deptID, "salary", false) {
			httpapi.WriteError(w, http.StatusForbidden, "forbidden", "salary module required")
			return
		}
		rows, err := h.DB.Query(r.Context(), `
			SELECT s.id, s.user_id, u.display_name, s.amount, s.currency, s.month::text, s.withdrawn, s.note
			FROM department_salaries s JOIN users u ON u.id=s.user_id
			WHERE s.department_id=$1 AND s.organization_id=$2
			ORDER BY s.month DESC, u.display_name`, deptID, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load salaries")
			return
		}
		defer rows.Close()
		items := make([]SalaryRow, 0)
		for rows.Next() {
			var item SalaryRow
			if err := rows.Scan(&item.ID, &item.UserID, &item.UserName, &item.Amount, &item.Currency, &item.Month, &item.Withdrawn, &item.Note); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read salaries")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	if !h.canAccessModule(r.Context(), user, deptID, "salary", true) {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "salary manage required")
		return
	}
	var input struct {
		UserID    string  `json:"user_id"`
		Amount    float64 `json:"amount"`
		Currency  string  `json:"currency"`
		Month     string  `json:"month"`
		Withdrawn bool    `json:"withdrawn"`
		Note      string  `json:"note"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.UserID == "" || input.Month == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "user_id and month are required")
		return
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	var item SalaryRow
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO department_salaries (organization_id, department_id, user_id, amount, currency, month, withdrawn, note, created_by)
		VALUES ($1,$2,$3,$4,$5,$6::date,$7,$8,$9)
		ON CONFLICT (department_id, user_id, month) DO UPDATE SET
			amount=EXCLUDED.amount, currency=EXCLUDED.currency, withdrawn=EXCLUDED.withdrawn, note=EXCLUDED.note, updated_at=NOW()
		RETURNING id, user_id, amount, currency, month::text, withdrawn, note`,
		user.OrganizationID, deptID, input.UserID, input.Amount, input.Currency, input.Month, input.Withdrawn, input.Note, user.ID,
	).Scan(&item.ID, &item.UserID, &item.Amount, &item.Currency, &item.Month, &item.Withdrawn, &item.Note)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "save_failed", "could not save salary")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, item)
}

func (h Handler) Bonuses(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	deptID := r.PathValue("id")
	if r.Method == http.MethodGet {
		if !h.canAccessModule(r.Context(), user, deptID, "bonus", false) {
			httpapi.WriteError(w, http.StatusForbidden, "forbidden", "bonus module required")
			return
		}
		rows, err := h.DB.Query(r.Context(), `
			SELECT b.id, b.public_id, b.user_id, u.display_name, b.role_label, b.privilege, b.amount, b.currency, b.debited_on::text
			FROM department_bonuses b JOIN users u ON u.id=b.user_id
			WHERE b.department_id=$1 AND b.organization_id=$2
			ORDER BY b.debited_on DESC`, deptID, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load bonuses")
			return
		}
		defer rows.Close()
		items := make([]BonusRow, 0)
		for rows.Next() {
			var item BonusRow
			if err := rows.Scan(&item.ID, &item.PublicID, &item.UserID, &item.UserName, &item.RoleLabel, &item.Privilege, &item.Amount, &item.Currency, &item.DebitedOn); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read bonuses")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	if !h.canAccessModule(r.Context(), user, deptID, "bonus", true) {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "bonus manage required")
		return
	}
	var input struct {
		UserID    string  `json:"user_id"`
		RoleLabel string  `json:"role_label"`
		Privilege string  `json:"privilege"`
		Amount    float64 `json:"amount"`
		Currency  string  `json:"currency"`
		DebitedOn string  `json:"debited_on"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.UserID == "" || input.DebitedOn == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "user_id and debited_on are required")
		return
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	publicID, err := randomEightDigit()
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "id_failed", "could not assign bonus id")
		return
	}
	var item BonusRow
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO department_bonuses (organization_id, department_id, public_id, user_id, role_label, privilege, amount, currency, debited_on, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::date,$10)
		RETURNING id, public_id, user_id, role_label, privilege, amount, currency, debited_on::text`,
		user.OrganizationID, deptID, publicID, input.UserID, input.RoleLabel, input.Privilege, input.Amount, input.Currency, input.DebitedOn, user.ID,
	).Scan(&item.ID, &item.PublicID, &item.UserID, &item.RoleLabel, &item.Privilege, &item.Amount, &item.Currency, &item.DebitedOn)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "save_failed", "could not save bonus")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, item)
}

func validModule(code string) bool {
	for _, m := range AllModules {
		if m == code {
			return true
		}
	}
	return false
}

func randomEightDigit() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(90000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%08d", n.Int64()+10000000), nil
}