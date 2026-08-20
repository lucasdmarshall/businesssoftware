-- Phase 7 Sales and CRM module: companies, contacts, leads, opportunities
-- (with a pipeline stage), and activities.

CREATE TABLE IF NOT EXISTS companies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    industry TEXT NOT NULL DEFAULT '',
    website TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, name)
);
CREATE INDEX IF NOT EXISTS companies_org_idx ON companies (organization_id, name);

CREATE TABLE IF NOT EXISTS contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    company_id UUID REFERENCES companies(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    email TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS contacts_org_idx ON contacts (organization_id, name);

CREATE TABLE IF NOT EXISTS leads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    contact_email TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'other',
    status TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'qualified', 'converted', 'lost')),
    owner_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS leads_org_status_idx ON leads (organization_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS opportunities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    company_id UUID REFERENCES companies(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    stage TEXT NOT NULL DEFAULT 'prospect' CHECK (stage IN ('prospect', 'qualified', 'proposal', 'won', 'lost')),
    amount NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (amount >= 0),
    currency TEXT NOT NULL DEFAULT 'USD',
    owner_id UUID REFERENCES users(id) ON DELETE SET NULL,
    close_date DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS opportunities_org_stage_idx ON opportunities (organization_id, stage, updated_at DESC);

CREATE TABLE IF NOT EXISTS crm_activities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    entity_id UUID NOT NULL,
    kind TEXT NOT NULL DEFAULT 'note' CHECK (kind IN ('call', 'email', 'meeting', 'note')),
    note TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS crm_activities_entity_idx ON crm_activities (organization_id, entity_type, entity_id, created_at DESC);

INSERT INTO permissions (code, description) VALUES
    ('sales.read', 'View sales and CRM records'),
    ('sales.manage', 'Manage sales and CRM records')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r CROSS JOIN permissions p
WHERE r.code = 'owner' AND p.code IN ('sales.read', 'sales.manage')
ON CONFLICT DO NOTHING;
