-- Full Finance suite: chart of accounts, journals, customers, AP bills,
-- payments, and tax codes. Existing vendors/expenses/invoices/budgets/PRs remain.

CREATE TABLE IF NOT EXISTS finance_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    account_type TEXT NOT NULL CHECK (account_type IN ('asset', 'liability', 'equity', 'revenue', 'expense')),
    parent_id UUID REFERENCES finance_accounts(id) ON DELETE SET NULL,
    is_system BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, code)
);
CREATE INDEX IF NOT EXISTS finance_accounts_org_idx ON finance_accounts (organization_id, account_type, code);

CREATE TABLE IF NOT EXISTS finance_tax_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    rate_percent NUMERIC(8,4) NOT NULL DEFAULT 0 CHECK (rate_percent >= 0),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, code)
);

CREATE TABLE IF NOT EXISTS finance_customers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    contact_email TEXT NOT NULL DEFAULT '',
    contact_phone TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, name)
);
CREATE INDEX IF NOT EXISTS finance_customers_org_idx ON finance_customers (organization_id, name);

CREATE TABLE IF NOT EXISTS finance_journals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    entry_date DATE NOT NULL,
    memo TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'posted', 'void')),
    source TEXT NOT NULL DEFAULT 'manual',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    posted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS finance_journals_org_date_idx ON finance_journals (organization_id, entry_date DESC);

CREATE TABLE IF NOT EXISTS finance_journal_lines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    journal_id UUID NOT NULL REFERENCES finance_journals(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES finance_accounts(id),
    description TEXT NOT NULL DEFAULT '',
    debit NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (debit >= 0),
    credit NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (credit >= 0),
    CHECK (NOT (debit > 0 AND credit > 0)),
    CHECK (debit > 0 OR credit > 0)
);
CREATE INDEX IF NOT EXISTS finance_journal_lines_journal_idx ON finance_journal_lines (journal_id);

CREATE TABLE IF NOT EXISTS finance_bills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    vendor_id UUID NOT NULL REFERENCES vendors(id) ON DELETE RESTRICT,
    number TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    amount NUMERIC(14,2) NOT NULL CHECK (amount >= 0),
    tax_amount NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
    currency TEXT NOT NULL DEFAULT 'USD',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'open', 'partial', 'paid', 'void')),
    bill_date DATE NOT NULL DEFAULT CURRENT_DATE,
    due_date DATE,
    expense_account_id UUID REFERENCES finance_accounts(id) ON DELETE SET NULL,
    tax_code_id UUID REFERENCES finance_tax_codes(id) ON DELETE SET NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, number)
);
CREATE INDEX IF NOT EXISTS finance_bills_org_status_idx ON finance_bills (organization_id, status, due_date);

CREATE TABLE IF NOT EXISTS finance_payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    direction TEXT NOT NULL CHECK (direction IN ('in', 'out')),
    amount NUMERIC(14,2) NOT NULL CHECK (amount > 0),
    currency TEXT NOT NULL DEFAULT 'USD',
    method TEXT NOT NULL DEFAULT 'transfer' CHECK (method IN ('cash', 'transfer', 'card', 'check', 'other')),
    paid_on DATE NOT NULL DEFAULT CURRENT_DATE,
    reference TEXT NOT NULL DEFAULT '',
    memo TEXT NOT NULL DEFAULT '',
    vendor_id UUID REFERENCES vendors(id) ON DELETE SET NULL,
    customer_id UUID REFERENCES finance_customers(id) ON DELETE SET NULL,
    bill_id UUID REFERENCES finance_bills(id) ON DELETE SET NULL,
    invoice_id UUID REFERENCES invoices(id) ON DELETE SET NULL,
    cash_account_id UUID REFERENCES finance_accounts(id) ON DELETE SET NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (
        (direction = 'out' AND vendor_id IS NOT NULL) OR
        (direction = 'in' AND customer_id IS NOT NULL) OR
        (vendor_id IS NULL AND customer_id IS NULL)
    )
);
CREATE INDEX IF NOT EXISTS finance_payments_org_date_idx ON finance_payments (organization_id, paid_on DESC);

ALTER TABLE invoices
    ADD COLUMN IF NOT EXISTS customer_id UUID REFERENCES finance_customers(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS tax_amount NUMERIC(14,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS tax_code_id UUID REFERENCES finance_tax_codes(id) ON DELETE SET NULL;

INSERT INTO permissions (code, description) VALUES
    ('finance.accounts.manage', 'Manage chart of accounts and tax codes'),
    ('finance.journals.manage', 'Create and post journal entries'),
    ('finance.pay', 'Record payments against bills and invoices')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r CROSS JOIN permissions p
WHERE r.code = 'owner' AND p.code IN ('finance.accounts.manage', 'finance.journals.manage', 'finance.pay')
ON CONFLICT DO NOTHING;
