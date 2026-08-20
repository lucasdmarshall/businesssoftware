package workspace

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"name/backend/internal/auth"
	"name/backend/internal/httpapi"
)

// Module codes match the department workspace nav.
var AllModules = []string{
	"overview", "users", "access", "positions", "attendance", "calendar", "leave",
	"schedule", "salary", "bonus", "finance", "credentials", "tasks", "activity", "settings",
}

type Handler struct {
	DB   *pgxpool.Pool
	Auth auth.Handler
}

type DepartmentWorkspace struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Slug         string        `json:"slug"`
	IsHead       bool          `json:"is_head"`
	IsPrimary    bool          `json:"is_primary"`
	PositionCode string        `json:"position_code"`
	PositionName string        `json:"position_name"`
	CompanyWide  bool          `json:"company_wide"`
	Modules      []ModuleGrant `json:"modules"`
}

type ModuleGrant struct {
	Code      string `json:"code"`
	CanView   bool   `json:"can_view"`
	CanManage bool   `json:"can_manage"`
}

type Position struct {
	ID          string        `json:"id"`
	ParentID    string        `json:"parent_id"`
	Code        string        `json:"code"`
	Name        string        `json:"name"`
	RankOrder   int           `json:"rank_order"`
	IsSystem    bool          `json:"is_system"`
	Depth       int           `json:"depth"`
	MemberCount int           `json:"member_count"`
	Modules     []ModuleGrant `json:"modules,omitempty"`
}

type Member struct {
	UserID       string `json:"user_id"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	EmployeeID   string `json:"employee_id"`
	IsHead       bool   `json:"is_head"`
	IsPrimary    bool   `json:"is_primary"`
	PositionID   string `json:"position_id"`
	PositionCode string `json:"position_code"`
	PositionName string `json:"position_name"`
	Status       string `json:"status"`
}

type AccessRow struct {
	UserID       string        `json:"user_id"`
	DisplayName  string        `json:"display_name"`
	IsHead       bool          `json:"is_head"`
	PositionID   string        `json:"position_id"`
	PositionCode string        `json:"position_code"`
	PositionName string        `json:"position_name"`
	Modules      []ModuleGrant `json:"modules"`
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
	ID        string  `json:"id"`
	PublicID  string  `json:"public_id"`
	UserID    string  `json:"user_id"`
	UserName  string  `json:"user_name"`
	RoleLabel string  `json:"role_label"`
	Privilege string  `json:"privilege"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
	DebitedOn string  `json:"debited_on"`
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
	_ = SeedCoreDepartments(r.Context(), h.DB, user.OrganizationID)
	var rows interface {
		Next() bool
		Scan(dest ...any) error
		Close()
		Err() error
	}
	if wide {
		q, err := h.DB.Query(r.Context(), `
			SELECT d.id, d.name, d.slug, COALESCE(ud.is_head, FALSE), COALESCE(ud.is_primary, FALSE), COALESCE(p.code,''), COALESCE(p.name,'')
			FROM departments d
			LEFT JOIN user_departments ud ON ud.department_id=d.id AND ud.user_id=$2
			LEFT JOIN department_positions p ON p.id=ud.position_id
			WHERE d.organization_id=$1 AND d.archived_at IS NULL ORDER BY d.name`, user.OrganizationID, user.ID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load departments")
			return
		}
		rows = q
	} else {
		q, err := h.DB.Query(r.Context(), `
			SELECT d.id, d.name, d.slug, ud.is_head, ud.is_primary, COALESCE(p.code,''), COALESCE(p.name,'')
			FROM departments d
			JOIN user_departments ud ON ud.department_id=d.id
			LEFT JOIN department_positions p ON p.id=ud.position_id
			WHERE d.organization_id=$1 AND ud.user_id=$2 AND d.archived_at IS NULL
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
		if err := rows.Scan(&item.ID, &item.Name, &item.Slug, &item.IsHead, &item.IsPrimary, &item.PositionCode, &item.PositionName); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read departments")
			return
		}
		_ = SeedDepartmentPositions(r.Context(), h.DB, user.OrganizationID, item.ID)
		item.CompanyWide = wide
		item.Modules = h.modulesFor(r.Context(), user, item.ID, wide)
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
	_ = SeedDepartmentPositions(ctx, h.DB, user.OrganizationID, deptID)
	err := h.DB.QueryRow(ctx, `
		SELECT d.id, d.name, d.slug, COALESCE(ud.is_head, FALSE), COALESCE(ud.is_primary, FALSE), COALESCE(p.code,''), COALESCE(p.name,'')
		FROM departments d
		LEFT JOIN user_departments ud ON ud.department_id=d.id AND ud.user_id=$3
		LEFT JOIN department_positions p ON p.id=ud.position_id
		WHERE d.id=$1 AND d.organization_id=$2 AND d.archived_at IS NULL`, deptID, user.OrganizationID, user.ID,
	).Scan(&ws.ID, &ws.Name, &ws.Slug, &ws.IsHead, &ws.IsPrimary, &ws.PositionCode, &ws.PositionName)
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
	ws.Modules = h.modulesFor(ctx, user, deptID, wide)
	return ws, true
}

// modulesFor resolves access from department position hierarchy, then optional
// per-user overrides in department_module_grants. Company-wide access gets full manage.
func (h Handler) modulesFor(ctx context.Context, user auth.SessionUser, deptID string, wide bool) []ModuleGrant {
	grants := map[string]ModuleGrant{}
	if wide {
		for _, code := range AllModules {
			grants[code] = ModuleGrant{Code: code, CanView: true, CanManage: true}
		}
	} else {
		rows, err := h.DB.Query(ctx, `
			SELECT pm.module_code, pm.can_view, pm.can_manage
			FROM user_departments ud
			JOIN department_position_modules pm ON pm.position_id=ud.position_id
			WHERE ud.department_id=$1 AND ud.user_id=$2`, deptID, user.ID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var g ModuleGrant
				if rows.Scan(&g.Code, &g.CanView, &g.CanManage) == nil {
					grants[g.Code] = g
				}
			}
		}
		// Legacy fallback when position is not assigned yet.
		if len(grants) == 0 {
			var isHead bool
			_ = h.DB.QueryRow(ctx, `SELECT COALESCE(is_head,FALSE) FROM user_departments WHERE department_id=$1 AND user_id=$2`, deptID, user.ID).Scan(&isHead)
			if isHead {
				for _, code := range AllModules {
					grants[code] = ModuleGrant{Code: code, CanView: true, CanManage: true}
				}
			} else {
				for _, code := range employeeModules {
					grants[code] = ModuleGrant{Code: code, CanView: true, CanManage: false}
				}
			}
		}
	}
	// Optional per-user overrides (exceptions on top of position).
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
		SELECT u.id, u.display_name, u.email, COALESCE(ep.phone,''), COALESCE(ep.employee_code,''),
		       ud.is_head, ud.is_primary, COALESCE(ud.position_id::text,''), COALESCE(p.code,''), COALESCE(p.name,''), u.status
		FROM user_departments ud
		JOIN users u ON u.id=ud.user_id
		LEFT JOIN employee_profiles ep ON ep.user_id=u.id AND ep.organization_id=u.organization_id
		LEFT JOIN department_positions p ON p.id=ud.position_id
		WHERE ud.department_id=$1 AND u.organization_id=$2
		ORDER BY COALESCE(p.rank_order,0) DESC, u.display_name`, deptID, user.OrganizationID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load members")
		return
	}
	defer rows.Close()
	items := make([]Member, 0)
	for rows.Next() {
		var item Member
		if err := rows.Scan(&item.UserID, &item.DisplayName, &item.Email, &item.Phone, &item.EmployeeID,
			&item.IsHead, &item.IsPrimary, &item.PositionID, &item.PositionCode, &item.PositionName, &item.Status); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read members")
			return
		}
		items = append(items, item)
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

func (h Handler) Positions(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	deptID := r.PathValue("id")
	if r.Method == http.MethodPost {
		h.createPosition(w, r, user, deptID)
		return
	}
	if !h.canAccessModule(r.Context(), user, deptID, "positions", false) {
		if _, ok := h.resolve(r.Context(), user, deptID); !ok {
			httpapi.WriteError(w, http.StatusForbidden, "forbidden", "you cannot enter this department workspace")
			return
		}
	}
	_ = SeedDepartmentPositions(r.Context(), h.DB, user.OrganizationID, deptID)
	rows, err := h.DB.Query(r.Context(), `
		SELECT p.id, COALESCE(p.parent_id::text,''), p.code, p.name, p.rank_order, p.is_system,
		       (SELECT COUNT(*)::int FROM user_departments ud WHERE ud.position_id=p.id)
		FROM department_positions p
		WHERE p.department_id=$1
		ORDER BY p.rank_order DESC, p.name ASC`, deptID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load positions")
		return
	}
	defer rows.Close()
	items := make([]Position, 0)
	for rows.Next() {
		var item Position
		if err := rows.Scan(&item.ID, &item.ParentID, &item.Code, &item.Name, &item.RankOrder, &item.IsSystem, &item.MemberCount); err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read positions")
			return
		}
		item.Depth = len(items) // list index: 0 = top = highest seniority
		item.Modules = make([]ModuleGrant, 0)
		modRows, err := h.DB.Query(r.Context(), `
			SELECT module_code, can_view, can_manage FROM department_position_modules
			WHERE position_id=$1 ORDER BY module_code`, item.ID)
		if err == nil {
			for modRows.Next() {
				var g ModuleGrant
				if modRows.Scan(&g.Code, &g.CanView, &g.CanManage) == nil {
					item.Modules = append(item.Modules, g)
				}
			}
			modRows.Close()
		}
		items = append(items, item)
	}
	httpapi.WriteJSON(w, http.StatusOK, items)
}

// ReorderPositions applies vertical list order: first id = highest rank in the department.
func (h Handler) ReorderPositions(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	deptID := r.PathValue("id")
	if !h.canAccessModule(r.Context(), user, deptID, "positions", true) && !h.canAccessModule(r.Context(), user, deptID, "access", true) {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "only department heads or company-wide access can reorder positions")
		return
	}
	var input struct {
		OrderedIDs []string `json:"ordered_ids"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || len(input.OrderedIDs) == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "ordered_ids is required")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "tx_failed", "could not reorder positions")
		return
	}
	defer tx.Rollback(r.Context())

	var count int
	_ = tx.QueryRow(r.Context(), `SELECT COUNT(*) FROM department_positions WHERE department_id=$1`, deptID).Scan(&count)
	if count != len(input.OrderedIDs) {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_order", "ordered_ids must include every position in this department")
		return
	}
	seen := map[string]bool{}
	n := len(input.OrderedIDs)
	var prevID any = nil
	for i, id := range input.OrderedIDs {
		if seen[id] {
			httpapi.WriteError(w, http.StatusBadRequest, "duplicate_id", "ordered_ids has duplicates")
			return
		}
		seen[id] = true
		rank := (n - i) * 10
		tag, err := tx.Exec(r.Context(), `
			UPDATE department_positions
			SET rank_order=$1, parent_id=$2
			WHERE id=$3 AND department_id=$4`, rank, prevID, id, deptID)
		if err != nil || tag.RowsAffected() == 0 {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_position", "position does not belong to this department")
			return
		}
		prevID = id
	}
	_, _ = tx.Exec(r.Context(), `
		INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata)
		VALUES ($1,$2,'department.positions_reordered','department',$3,$4)`,
		user.OrganizationID, user.ID, deptID, map[string]any{"ordered_ids": input.OrderedIDs})
	if err := tx.Commit(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not save order")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "reordered"})
}

func (h Handler) createPosition(w http.ResponseWriter, r *http.Request, user auth.SessionUser, deptID string) {
	if !h.canAccessModule(r.Context(), user, deptID, "positions", true) && !h.canAccessModule(r.Context(), user, deptID, "access", true) {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "only department heads or company-wide access can add positions")
		return
	}
	_ = SeedDepartmentPositions(r.Context(), h.DB, user.OrganizationID, deptID)
	var input struct {
		Name      string `json:"name"`
		Code      string `json:"code"`
		ParentID  string `json:"parent_id"`
		RankOrder int    `json:"rank_order"`
		CopyFrom  string `json:"copy_modules_from"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	code := strings.TrimSpace(input.Code)
	if code == "" {
		code = slugifyPositionCode(input.Name)
	}
	if input.RankOrder == 0 {
		var minRank int
		_ = h.DB.QueryRow(r.Context(), `
			SELECT COALESCE(MIN(rank_order), 10) FROM department_positions WHERE department_id=$1`, deptID).Scan(&minRank)
		input.RankOrder = minRank - 10
		if input.ParentID != "" {
			var parentRank int
			_ = h.DB.QueryRow(r.Context(), `
				SELECT rank_order FROM department_positions WHERE id=$1 AND department_id=$2`,
				input.ParentID, deptID).Scan(&parentRank)
			// Sit just below the selected parent until the user drags to finalize.
			input.RankOrder = parentRank - 5
		}
	}
	if input.ParentID != "" {
		var ok bool
		_ = h.DB.QueryRow(r.Context(), `
			SELECT EXISTS(SELECT 1 FROM department_positions WHERE id=$1 AND department_id=$2)`,
			input.ParentID, deptID).Scan(&ok)
		if !ok {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_parent", "parent position must belong to this department")
			return
		}
	}
	var parent any
	if input.ParentID == "" {
		parent = nil
	} else {
		parent = input.ParentID
	}
	var item Position
	err := h.DB.QueryRow(r.Context(), `
		INSERT INTO department_positions (organization_id, department_id, parent_id, code, name, rank_order, is_system)
		VALUES ($1,$2,$3,$4,$5,$6,FALSE)
		RETURNING id, COALESCE(parent_id::text,''), code, name, rank_order, is_system`,
		user.OrganizationID, deptID, parent, code, strings.TrimSpace(input.Name), input.RankOrder,
	).Scan(&item.ID, &item.ParentID, &item.Code, &item.Name, &item.RankOrder, &item.IsSystem)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "create_failed", "could not create position (code may already exist)")
		return
	}
	copyID := input.CopyFrom
	if copyID == "" && input.ParentID != "" {
		copyID = input.ParentID
	}
	if copyID == "" {
		_ = h.DB.QueryRow(r.Context(), `
			SELECT id FROM department_positions WHERE department_id=$1 AND code='employee'`, deptID).Scan(&copyID)
	}
	if copyID != "" {
		_, _ = h.DB.Exec(r.Context(), `
			INSERT INTO department_position_modules (department_id, position_id, module_code, can_view, can_manage)
			SELECT department_id, $1, module_code, can_view, can_manage
			FROM department_position_modules WHERE position_id=$2
			ON CONFLICT DO NOTHING`, item.ID, copyID)
	}
	_, _ = h.DB.Exec(r.Context(), `
		INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata)
		VALUES ($1,$2,'department.position_created','department',$3,$4)`,
		user.OrganizationID, user.ID, deptID, map[string]any{"position_id": item.ID, "code": item.Code})
	httpapi.WriteJSON(w, http.StatusCreated, item)
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
			SELECT u.id, u.display_name, ud.is_head, COALESCE(ud.position_id::text,''), COALESCE(p.code,''), COALESCE(p.name,'')
			FROM user_departments ud
			JOIN users u ON u.id=ud.user_id
			LEFT JOIN department_positions p ON p.id=ud.position_id
			WHERE ud.department_id=$1 AND u.organization_id=$2
			ORDER BY COALESCE(p.rank_order,0) DESC, u.display_name`, deptID, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load access list")
			return
		}
		defer rows.Close()
		items := make([]AccessRow, 0)
		for rows.Next() {
			var item AccessRow
			if err := rows.Scan(&item.UserID, &item.DisplayName, &item.IsHead, &item.PositionID, &item.PositionCode, &item.PositionName); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read access list")
				return
			}
			item.Modules = h.effectiveModulesForUser(r.Context(), user.OrganizationID, deptID, item.UserID)
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}

	var input struct {
		UserID     string        `json:"user_id"`
		PositionID string        `json:"position_id"`
		IsHead     *bool         `json:"is_head"`
		Modules    []ModuleGrant `json:"modules"`
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

	if input.IsHead != nil && input.PositionID == "" {
		_ = SeedDepartmentPositions(r.Context(), h.DB, user.OrganizationID, deptID)
		posCode := "employee"
		if *input.IsHead {
			posCode = "head"
		}
		_, err = tx.Exec(r.Context(), `
			UPDATE user_departments SET is_head=$1, position_id=p.id
			FROM department_positions p
			WHERE user_departments.department_id=$2 AND user_departments.user_id=$3
			  AND p.department_id=$2 AND p.code=$4`,
			*input.IsHead, deptID, input.UserID, posCode)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "update_failed", "user is not in this department")
			return
		}
	}

	if input.PositionID != "" {
		var code string
		err = tx.QueryRow(r.Context(), `
			SELECT code FROM department_positions WHERE id=$1 AND department_id=$2`, input.PositionID, deptID).Scan(&code)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_position", "position does not belong to this department")
			return
		}
		_, err = tx.Exec(r.Context(), `
			UPDATE user_departments SET position_id=$1, is_head=($2='head')
			WHERE department_id=$3 AND user_id=$4`, input.PositionID, code, deptID, input.UserID)
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
		user.OrganizationID, user.ID, deptID, map[string]any{"user_id": input.UserID, "position_id": input.PositionID})
	if err := tx.Commit(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "commit_failed", "could not save access")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h Handler) effectiveModulesForUser(ctx context.Context, orgID, deptID, userID string) []ModuleGrant {
	fake := auth.SessionUser{ID: userID, OrganizationID: orgID}
	// Reuse position resolution without company-wide.
	grants := map[string]ModuleGrant{}
	rows, err := h.DB.Query(ctx, `
		SELECT pm.module_code, pm.can_view, pm.can_manage
		FROM user_departments ud
		JOIN department_position_modules pm ON pm.position_id=ud.position_id
		WHERE ud.department_id=$1 AND ud.user_id=$2`, deptID, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var g ModuleGrant
			if rows.Scan(&g.Code, &g.CanView, &g.CanManage) == nil {
				grants[g.Code] = g
			}
		}
	}
	_ = fake
	over, err := h.DB.Query(ctx, `
		SELECT module_code, can_view, can_manage FROM department_module_grants
		WHERE organization_id=$1 AND department_id=$2 AND user_id=$3`, orgID, deptID, userID)
	if err == nil {
		defer over.Close()
		for over.Next() {
			var g ModuleGrant
			if over.Scan(&g.Code, &g.CanView, &g.CanManage) == nil {
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

type FinanceEntry struct {
	ID         string  `json:"id"`
	EntryDate  string  `json:"entry_date"`
	Category   string  `json:"category"`
	Direction  string  `json:"direction"`
	Title      string  `json:"title"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	PersonID   string  `json:"person_id"`
	PersonName string  `json:"person_name"`
	Status     string  `json:"status"`
	Note       string  `json:"note"`
	CreatedAt  string  `json:"created_at"`
}

type FinanceSummary struct {
	OutTotal     float64 `json:"out_total"`
	InTotal      float64 `json:"in_total"`
	PendingCount int     `json:"pending_count"`
	EntryCount   int     `json:"entry_count"`
	Currency     string  `json:"currency"`
}

func validFinanceCategory(category string) bool {
	switch category {
	case "expense", "reimbursement", "petty_cash", "allowance", "salary", "bonus", "other":
		return true
	default:
		return false
	}
}

func validFinanceStatus(status string) bool {
	switch status {
	case "recorded", "pending", "settled", "void":
		return true
	default:
		return false
	}
}

// Finance lists or creates department-local money tracking rows (not company GL).
func (h Handler) Finance(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	deptID := r.PathValue("id")
	if r.Method == http.MethodGet {
		if !h.canAccessModule(r.Context(), user, deptID, "finance", false) {
			httpapi.WriteError(w, http.StatusForbidden, "forbidden", "finance module required")
			return
		}
		rows, err := h.DB.Query(r.Context(), `
			SELECT e.id, e.entry_date::text, e.category, e.direction, e.title, e.amount, e.currency,
			       COALESCE(e.person_id::text,''), COALESCE(u.display_name,''), e.status, e.note, e.created_at::text
			FROM department_finance_entries e
			LEFT JOIN users u ON u.id=e.person_id
			WHERE e.department_id=$1 AND e.organization_id=$2 AND e.status <> 'void'
			ORDER BY e.entry_date DESC, e.created_at DESC LIMIT 500`, deptID, user.OrganizationID)
		if err != nil {
			httpapi.WriteError(w, http.StatusInternalServerError, "query_failed", "could not load finance entries")
			return
		}
		defer rows.Close()
		items := make([]FinanceEntry, 0)
		for rows.Next() {
			var item FinanceEntry
			if err := rows.Scan(&item.ID, &item.EntryDate, &item.Category, &item.Direction, &item.Title, &item.Amount, &item.Currency,
				&item.PersonID, &item.PersonName, &item.Status, &item.Note, &item.CreatedAt); err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, "scan_failed", "could not read finance entries")
				return
			}
			items = append(items, item)
		}
		httpapi.WriteJSON(w, http.StatusOK, items)
		return
	}
	if !h.canAccessModule(r.Context(), user, deptID, "finance", true) {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "finance manage required")
		return
	}
	var input struct {
		EntryDate string  `json:"entry_date"`
		Category  string  `json:"category"`
		Direction string  `json:"direction"`
		Title     string  `json:"title"`
		Amount    float64 `json:"amount"`
		Currency  string  `json:"currency"`
		PersonID  string  `json:"person_id"`
		Status    string  `json:"status"`
		Note      string  `json:"note"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Title) == "" || input.Amount < 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "title and amount are required")
		return
	}
	if input.Category == "" {
		input.Category = "expense"
	}
	if !validFinanceCategory(input.Category) {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_category", "unknown finance category")
		return
	}
	if input.Direction == "" {
		input.Direction = "out"
	}
	if input.Direction != "in" && input.Direction != "out" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_direction", "direction must be in or out")
		return
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	if input.Status == "" {
		input.Status = "recorded"
	}
	if !validFinanceStatus(input.Status) {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_status", "invalid status")
		return
	}
	if input.EntryDate == "" {
		input.EntryDate = time.Now().UTC().Format("2006-01-02")
	}
	var person any
	if input.PersonID != "" {
		person = input.PersonID
	}
	var item FinanceEntry
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO department_finance_entries (
			organization_id, department_id, entry_date, category, direction, title, amount, currency, person_id, status, note, created_by
		) VALUES ($1,$2,$3::date,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id, entry_date::text, category, direction, title, amount, currency, COALESCE(person_id::text,''), status, note, created_at::text`,
		user.OrganizationID, deptID, input.EntryDate, input.Category, input.Direction, strings.TrimSpace(input.Title),
		input.Amount, input.Currency, person, input.Status, input.Note, user.ID,
	).Scan(&item.ID, &item.EntryDate, &item.Category, &item.Direction, &item.Title, &item.Amount, &item.Currency,
		&item.PersonID, &item.Status, &item.Note, &item.CreatedAt)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "save_failed", "could not save finance entry")
		return
	}
	if item.PersonID != "" {
		_ = h.DB.QueryRow(r.Context(), `SELECT display_name FROM users WHERE id=$1`, item.PersonID).Scan(&item.PersonName)
	}
	_, _ = h.DB.Exec(r.Context(), `
		INSERT INTO audit_logs (organization_id, actor_id, action, entity_type, entity_id, metadata)
		VALUES ($1,$2,'department.finance_entry_created','department',$3,$4)`,
		user.OrganizationID, user.ID, deptID, map[string]any{"entry_id": item.ID, "amount": item.Amount, "category": item.Category})
	httpapi.WriteJSON(w, http.StatusCreated, item)
}

func (h Handler) FinanceSummary(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	deptID := r.PathValue("id")
	if !h.canAccessModule(r.Context(), user, deptID, "finance", false) {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "finance module required")
		return
	}
	var summary FinanceSummary
	summary.Currency = "USD"
	_ = h.DB.QueryRow(r.Context(), `
		SELECT
			COALESCE(SUM(amount) FILTER (WHERE direction='out' AND status IN ('recorded','settled','pending')),0),
			COALESCE(SUM(amount) FILTER (WHERE direction='in' AND status IN ('recorded','settled','pending')),0),
			COUNT(*) FILTER (WHERE status='pending'),
			COUNT(*) FILTER (WHERE status <> 'void')
		FROM department_finance_entries
		WHERE department_id=$1 AND organization_id=$2`, deptID, user.OrganizationID,
	).Scan(&summary.OutTotal, &summary.InTotal, &summary.PendingCount, &summary.EntryCount)
	httpapi.WriteJSON(w, http.StatusOK, summary)
}

func (h Handler) FinanceStatus(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil || h.DB == nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	deptID := r.PathValue("id")
	if !h.canAccessModule(r.Context(), user, deptID, "finance", true) {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "finance manage required")
		return
	}
	var input struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.ID == "" || !validFinanceStatus(input.Status) {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "id and valid status are required")
		return
	}
	tag, err := h.DB.Exec(r.Context(), `
		UPDATE department_finance_entries SET status=$1, updated_at=NOW()
		WHERE id=$2 AND department_id=$3 AND organization_id=$4`,
		input.Status, input.ID, deptID, user.OrganizationID)
	if err != nil || tag.RowsAffected() == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "update_failed", "entry not found")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": input.Status})
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
