package workspace

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SeedDepartmentPositions creates the standard head → manager → employee ladder
// and default module matrix for a department (idempotent).
func SeedDepartmentPositions(ctx context.Context, db *pgxpool.Pool, orgID, deptID string) error {
	type pos struct {
		Code, Name string
		Rank       int
	}
	defaults := []pos{
		{"head", "Department head", 100},
		{"manager", "Manager", 50},
		{"employee", "Employee", 10},
	}
	ids := map[string]string{}
	for _, p := range defaults {
		var id string
		err := db.QueryRow(ctx, `
			INSERT INTO department_positions (organization_id, department_id, code, name, rank_order, is_system)
			VALUES ($1,$2,$3,$4,$5,TRUE)
			ON CONFLICT (department_id, code) DO UPDATE SET name=EXCLUDED.name, is_system=TRUE
			RETURNING id`, orgID, deptID, p.Code, p.Name, p.Rank).Scan(&id)
		if err != nil {
			return err
		}
		ids[p.Code] = id
	}
	// parent chain: head → manager → employee
	if _, err := db.Exec(ctx, `UPDATE department_positions SET parent_id=NULL WHERE id=$1`, ids["head"]); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `UPDATE department_positions SET parent_id=$1 WHERE id=$2`, ids["head"], ids["manager"]); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `UPDATE department_positions SET parent_id=$1 WHERE id=$2`, ids["manager"], ids["employee"]); err != nil {
		return err
	}

	for code, id := range ids {
		modules := employeeModules
		manageAll := false
		switch code {
		case "head":
			modules = AllModules
			manageAll = true
		case "manager":
			modules = managerModules
		}
		for _, moduleCode := range modules {
			_, err := db.Exec(ctx, `
				INSERT INTO department_position_modules (department_id, position_id, module_code, can_view, can_manage)
				VALUES ($1,$2,$3,TRUE,$4)
				ON CONFLICT (position_id, module_code) DO NOTHING`,
				deptID, id, moduleCode, manageAll)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func slugifyPositionCode(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "custom"
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}

var managerModules = []string{
	"overview", "users", "positions", "attendance", "calendar", "leave", "schedule", "tasks", "activity",
}

var employeeModules = []string{
	"overview", "users", "positions", "attendance", "calendar", "leave", "schedule", "tasks", "activity",
}
