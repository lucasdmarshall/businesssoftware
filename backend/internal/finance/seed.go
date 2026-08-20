package finance

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type accountSeed struct {
	Code, Name, Type string
}

var defaultAccounts = []accountSeed{
	{"1000", "Cash", "asset"},
	{"1100", "Accounts receivable", "asset"},
	{"2000", "Accounts payable", "liability"},
	{"3000", "Owner equity", "equity"},
	{"4000", "Revenue", "revenue"},
	{"5000", "Operating expenses", "expense"},
	{"5100", "Cost of goods", "expense"},
}

var defaultTaxCodes = []struct {
	Code, Name string
	Rate       float64
}{
	{"EXEMPT", "Tax exempt", 0},
	{"STD", "Standard tax", 10},
}

// SeedFinanceSuite ensures each org has a starter chart of accounts and tax codes.
func SeedFinanceSuite(ctx context.Context, db *pgxpool.Pool, orgID string) error {
	for _, a := range defaultAccounts {
		_, err := db.Exec(ctx, `
			INSERT INTO finance_accounts (organization_id, code, name, account_type, is_system, is_active)
			VALUES ($1,$2,$3,$4,TRUE,TRUE)
			ON CONFLICT (organization_id, code) DO NOTHING`, orgID, a.Code, a.Name, a.Type)
		if err != nil {
			return err
		}
	}
	for _, t := range defaultTaxCodes {
		_, err := db.Exec(ctx, `
			INSERT INTO finance_tax_codes (organization_id, code, name, rate_percent, is_active)
			VALUES ($1,$2,$3,$4,TRUE)
			ON CONFLICT (organization_id, code) DO NOTHING`, orgID, t.Code, t.Name, t.Rate)
		if err != nil {
			return err
		}
	}
	return nil
}
