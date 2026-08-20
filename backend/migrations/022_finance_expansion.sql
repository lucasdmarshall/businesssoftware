-- Phase 7 Finance expansion: invoices, budgets, and purchase requests.
-- Purchase requests route approval through the generic workflow engine.

CREATE TABLE IF NOT EXISTS invoices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    number TEXT NOT NULL,
    customer_name TEXT NOT NULL,
    amount NUMERIC(14,2) NOT NULL CHECK (amount >= 0),
    currency TEXT NOT NULL DEFAULT 'USD',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'sent', 'paid', 'overdue', 'cancelled')),
    issued_date DATE,
    due_date DATE,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, number)
);
CREATE INDEX IF NOT EXISTS invoices_org_status_idx ON invoices (organization_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS budgets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    department_id UUID REFERENCES departments(id) ON DELETE SET NULL,
    amount NUMERIC(14,2) NOT NULL CHECK (amount >= 0),
    currency TEXT NOT NULL DEFAULT 'USD',
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (period_end >= period_start)
);
CREATE INDEX IF NOT EXISTS budgets_org_idx ON budgets (organization_id, period_start DESC);

CREATE TABLE IF NOT EXISTS purchase_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    requested_by UUID NOT NULL REFERENCES users(id),
    vendor_id UUID REFERENCES vendors(id) ON DELETE SET NULL,
    item TEXT NOT NULL,
    justification TEXT NOT NULL DEFAULT '',
    amount NUMERIC(14,2) NOT NULL CHECK (amount >= 0),
    currency TEXT NOT NULL DEFAULT 'USD',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'submitted', 'ordered', 'cancelled')),
    workflow_instance_id UUID REFERENCES workflow_instances(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS purchase_requests_org_status_idx ON purchase_requests (organization_id, status, created_at DESC);
