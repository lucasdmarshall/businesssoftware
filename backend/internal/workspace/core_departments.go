package workspace

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
)

type coreDepartment struct {
	Name string
	Slug string
}

// CoreDepartments is the default company catalog. CEO/admin may rename, archive, or add more.
var CoreDepartments = []coreDepartment{
	{"Administrations", "administrations"},
	{"HR", "hr"},
	{"Sales", "sales"},
	{"Finance", "finance"},
	{"IT", "it"},
	{"Operations", "operations"},
	{"Marketing", "marketing"},
	{"Procurement", "procurement"},
	{"Customer Service", "customer-service"},
	{"Reports and Analytics", "reports-and-analytics"},
	{"Legal and Compliance", "legal-and-compliance"},
}

// deptIdentity holds username tag + EMP ID prefix for a department slug.
type deptIdentity struct {
	UsernameTag string // e.g. admin → lucas_admin_000001
	EmpPrefix   string // e.g. ADM → ADM000001
}

// knownDeptIdentities maps core department slugs to stable credential codes.
var knownDeptIdentities = map[string]deptIdentity{
	"administrations":       {UsernameTag: "admin", EmpPrefix: "ADM"},
	"hr":                    {UsernameTag: "hr", EmpPrefix: "HR"},
	"sales":                 {UsernameTag: "sales", EmpPrefix: "SAL"},
	"finance":               {UsernameTag: "finance", EmpPrefix: "FIN"},
	"it":                    {UsernameTag: "it", EmpPrefix: "IT"},
	"operations":            {UsernameTag: "ops", EmpPrefix: "OPS"},
	"marketing":             {UsernameTag: "mkt", EmpPrefix: "MKT"},
	"procurement":           {UsernameTag: "proc", EmpPrefix: "PRC"},
	"customer-service":      {UsernameTag: "cs", EmpPrefix: "CS"},
	"reports-and-analytics": {UsernameTag: "analytics", EmpPrefix: "RPT"},
	"legal-and-compliance":  {UsernameTag: "legal", EmpPrefix: "LEG"},
}

// SeedCoreDepartments inserts the default department catalog (idempotent by slug).
func SeedCoreDepartments(ctx context.Context, db *pgxpool.Pool, orgID string) error {
	for _, dept := range CoreDepartments {
		var id string
		err := db.QueryRow(ctx, `
			INSERT INTO departments (organization_id, name, slug, is_core)
			VALUES ($1,$2,$3,TRUE)
			ON CONFLICT (organization_id, slug) DO UPDATE SET is_core = TRUE
			RETURNING id`, orgID, dept.Name, dept.Slug).Scan(&id)
		if err != nil {
			return err
		}
		if err := SeedDepartmentPositions(ctx, db, orgID, id); err != nil {
			return err
		}
		if dept.Slug == "hr" || dept.Slug == "it" {
			_, _ = db.Exec(ctx, `
				INSERT INTO department_position_modules (department_id, position_id, module_code, can_view, can_manage)
				SELECT p.department_id, p.id, 'credentials', TRUE, TRUE
				FROM department_positions p
				WHERE p.department_id=$1 AND p.code IN ('head','manager')
				ON CONFLICT (position_id, module_code) DO NOTHING`, id)
		}
	}
	return nil
}

// IdentityForDepartmentSlug returns username tag + EMP prefix for a department.
func IdentityForDepartmentSlug(slug string) deptIdentity {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if id, ok := knownDeptIdentities[slug]; ok {
		return id
	}
	return deriveDeptIdentity(slug)
}

func deriveDeptIdentity(slug string) deptIdentity {
	parts := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '-' || r == '_' || r == ' '
	})
	tag := "dept"
	if len(parts) > 0 {
		tag = slugifyToken(parts[0])
		if tag == "" {
			tag = "dept"
		}
		if len(tag) > 12 {
			tag = tag[:12]
		}
	}
	compact := strings.ToUpper(slugifyToken(strings.Join(parts, "")))
	if compact == "" {
		compact = "DEPT"
	}
	prefix := compact
	if len(prefix) > 3 {
		prefix = prefix[:3]
	}
	for len(prefix) < 2 {
		prefix += "X"
	}
	return deptIdentity{UsernameTag: tag, EmpPrefix: prefix}
}

// AllocateEmployeeIdentity assigns the next per-department sequence and builds
// username, default password, and EMP id:
//
//	username  = {firstname}_{dept_tag}_{NNNNNN}   e.g. lucas_admin_000001
//	password  = password{NNNNNN}                  e.g. password000001
//	employee  = {PREFIX}{NNNNNN}                  e.g. ADM000001
func AllocateEmployeeIdentity(ctx context.Context, db *pgxpool.Pool, orgID, departmentID, displayName, deptSlug string) (username, password, employeeID string, err error) {
	seq, err := nextDepartmentEmployeeNumber(ctx, db, orgID, departmentID)
	if err != nil {
		return "", "", "", err
	}
	ident := IdentityForDepartmentSlug(deptSlug)
	first := firstNameToken(displayName)
	seqStr := fmt.Sprintf("%06d", seq)
	username = fmt.Sprintf("%s_%s_%s", first, ident.UsernameTag, seqStr)
	password = "password" + seqStr
	employeeID = ident.EmpPrefix + seqStr

	// Ensure username uniqueness; collision is rare (same first+dept+seq) but guard anyway.
	for i := 0; i < 20; i++ {
		var exists bool
		if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE organization_id=$1 AND lower(username)=lower($2))`, orgID, username).Scan(&exists); err != nil {
			return "", "", "", err
		}
		if !exists {
			return username, password, employeeID, nil
		}
		seq++
		seqStr = fmt.Sprintf("%06d", seq)
		username = fmt.Sprintf("%s_%s_%s", first, ident.UsernameTag, seqStr)
		password = "password" + seqStr
		employeeID = ident.EmpPrefix + seqStr
	}
	return "", "", "", fmt.Errorf("could not allocate unique username")
}

func nextDepartmentEmployeeNumber(ctx context.Context, db *pgxpool.Pool, orgID, departmentID string) (int, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var n int
	err = tx.QueryRow(ctx, `
		INSERT INTO department_employee_counters (organization_id, department_id, next_number)
		VALUES ($1, $2, 2)
		ON CONFLICT (organization_id, department_id) DO UPDATE
		SET next_number = department_employee_counters.next_number + 1
		RETURNING next_number - 1`, orgID, departmentID).Scan(&n)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return n, nil
}

func firstNameToken(displayName string) string {
	fields := strings.Fields(strings.TrimSpace(displayName))
	token := "user"
	if len(fields) > 0 {
		token = slugifyToken(fields[0])
	}
	if token == "" {
		token = "user"
	}
	if len(token) > 24 {
		token = token[:24]
	}
	return token
}

func slugifyToken(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
