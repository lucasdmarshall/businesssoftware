-- Finance depth: bank accounts + statement lines for reconciliation.
-- Aging reports are computed from finance_bills / invoices (no new tables).

CREATE TABLE IF NOT EXISTS finance_bank_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    account_number_masked TEXT NOT NULL DEFAULT '',
    gl_account_id UUID REFERENCES finance_accounts(id) ON DELETE SET NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, name)
);
CREATE INDEX IF NOT EXISTS finance_bank_accounts_org_idx
    ON finance_bank_accounts (organization_id, is_active, name);

CREATE TABLE IF NOT EXISTS finance_bank_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    bank_account_id UUID NOT NULL REFERENCES finance_bank_accounts(id) ON DELETE CASCADE,
    txn_date DATE NOT NULL,
    amount NUMERIC(14,2) NOT NULL CHECK (amount > 0),
    direction TEXT NOT NULL CHECK (direction IN ('in', 'out')),
    description TEXT NOT NULL DEFAULT '',
    reference TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'unmatched'
        CHECK (status IN ('unmatched', 'matched', 'excluded')),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS finance_bank_transactions_account_idx
    ON finance_bank_transactions (bank_account_id, txn_date DESC, status);
CREATE INDEX IF NOT EXISTS finance_bank_transactions_org_status_idx
    ON finance_bank_transactions (organization_id, status, txn_date DESC);

CREATE TABLE IF NOT EXISTS finance_bank_matches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    bank_transaction_id UUID NOT NULL REFERENCES finance_bank_transactions(id) ON DELETE CASCADE,
    payment_id UUID REFERENCES finance_payments(id) ON DELETE SET NULL,
    journal_id UUID REFERENCES finance_journals(id) ON DELETE SET NULL,
    matched_by UUID REFERENCES users(id) ON DELETE SET NULL,
    matched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (bank_transaction_id),
    CHECK (payment_id IS NOT NULL OR journal_id IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS finance_bank_matches_payment_idx
    ON finance_bank_matches (payment_id) WHERE payment_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS finance_bank_matches_journal_idx
    ON finance_bank_matches (journal_id) WHERE journal_id IS NOT NULL;
