-- Phase 8 analytics and executive reporting: saved report definitions,
-- schedules, and permissions. Summary endpoints read live operational tables;
-- exports write audit_logs with action report.exported.

CREATE TABLE IF NOT EXISTS report_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    report_type TEXT NOT NULL
        CHECK (report_type IN ('spending', 'attendance', 'sales', 'custom')),
    description TEXT NOT NULL DEFAULT '',
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, code)
);

CREATE INDEX IF NOT EXISTS report_definitions_org_type_idx
    ON report_definitions (organization_id, report_type);

CREATE TABLE IF NOT EXISTS scheduled_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    definition_id UUID NOT NULL REFERENCES report_definitions(id) ON DELETE CASCADE,
    cadence TEXT NOT NULL CHECK (cadence IN ('daily', 'weekly', 'monthly')),
    next_run_at TIMESTAMPTZ NOT NULL,
    last_run_at TIMESTAMPTZ,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS scheduled_reports_due_idx
    ON scheduled_reports (active, next_run_at)
    WHERE active = TRUE;

INSERT INTO permissions (code, description) VALUES
    ('analytics.read', 'View analytics summaries'),
    ('reports.read', 'View saved and scheduled reports'),
    ('reports.manage', 'Create reports, schedules, and exports')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r CROSS JOIN permissions p
WHERE r.code = 'owner' AND p.code IN ('analytics.read', 'reports.read', 'reports.manage')
ON CONFLICT DO NOTHING;
