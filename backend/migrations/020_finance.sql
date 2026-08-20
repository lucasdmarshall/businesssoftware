-- Phase 7 Finance module: vendors and expenses. Expense approval routes through
-- the generic workflow engine (workflow_instances linked by entity_id).

CREATE TABLE IF NOT EXISTS vendors (
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

CREATE INDEX IF NOT EXISTS vendors_org_idx ON vendors (organization_id, name);

CREATE TABLE IF NOT EXISTS expenses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    submitted_by UUID NOT NULL REFERENCES users(id),
    vendor_id UUID REFERENCES vendors(id) ON DELETE SET NULL,
    category TEXT NOT NULL DEFAULT 'general',
    description TEXT NOT NULL,
    amount NUMERIC(14,2) NOT NULL CHECK (amount >= 0),
    currency TEXT NOT NULL DEFAULT 'USD',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'submitted', 'paid', 'cancelled')),
    workflow_instance_id UUID REFERENCES workflow_instances(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS expenses_org_status_idx ON expenses (organization_id, status, created_at DESC);

INSERT INTO permissions (code, description) VALUES
    ('finance.read', 'View finance records'),
    ('finance.manage', 'Create and manage finance records'),
    ('finance.approve', 'Approve finance requests')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r CROSS JOIN permissions p
WHERE r.code = 'owner' AND p.code IN ('finance.read', 'finance.manage', 'finance.approve')
ON CONFLICT DO NOTHING;
