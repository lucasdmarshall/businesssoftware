package workspace

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
	"time"
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

// GenerateUsername builds a unique lowercase username for an organization.
func GenerateUsername(ctx context.Context, db *pgxpool.Pool, orgID, displayName string) (string, error) {
	base := slugifyUsername(displayName)
	if base == "" {
		base = "user"
	}
	for i := 0; i < 40; i++ {
		candidate := base
		if i > 0 {
			n, err := rand.Int(rand.Reader, big.NewInt(9000))
			if err != nil {
				return "", err
			}
			candidate = fmt.Sprintf("%s%d", base, 1000+int(n.Int64()))
		}
		var exists bool
		if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE organization_id=$1 AND lower(username)=lower($2))`, orgID, candidate).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not allocate username")
}

// GeneratePassword returns a random password that meets the 12+ character policy.
func GeneratePassword() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// NextEmployeeID allocates EMP-YYYY-NNNN for an organization.
func NextEmployeeID(ctx context.Context, db *pgxpool.Pool, orgID string) (string, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var n int
	err = tx.QueryRow(ctx, `
		INSERT INTO organization_employee_counters (organization_id, next_number)
		VALUES ($1, 2)
		ON CONFLICT (organization_id) DO UPDATE
		SET next_number = organization_employee_counters.next_number + 1
		RETURNING next_number - 1`, orgID).Scan(&n)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return fmt.Sprintf("EMP-%d-%04d", time.Now().UTC().Year(), n), nil
}

func slugifyUsername(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 24 {
		out = out[:24]
	}
	return out
}
