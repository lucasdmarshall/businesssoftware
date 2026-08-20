-- Phase 4: leave balances, attendance correction history, and payroll rules.

CREATE TABLE IF NOT EXISTS leave_balances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    leave_type TEXT NOT NULL CHECK (leave_type IN ('annual', 'sick', 'personal', 'unpaid')),
    year INT NOT NULL CHECK (year >= 2000 AND year <= 2100),
    entitled_days NUMERIC(6,2) NOT NULL DEFAULT 0 CHECK (entitled_days >= 0),
    used_days NUMERIC(6,2) NOT NULL DEFAULT 0 CHECK (used_days >= 0),
    carried_over_days NUMERIC(6,2) NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, user_id, leave_type, year)
);

CREATE INDEX IF NOT EXISTS leave_balances_org_user_idx
    ON leave_balances (organization_id, user_id, year);

CREATE TABLE IF NOT EXISTS attendance_corrections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    attendance_id UUID REFERENCES attendance_records(id) ON DELETE SET NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    corrected_by UUID NOT NULL REFERENCES users(id),
    work_date DATE NOT NULL,
    previous_check_in_at TIMESTAMPTZ,
    previous_check_out_at TIMESTAMPTZ,
    previous_status TEXT NOT NULL DEFAULT '',
    previous_note TEXT NOT NULL DEFAULT '',
    check_in_at TIMESTAMPTZ,
    check_out_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'present',
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS attendance_corrections_org_date_idx
    ON attendance_corrections (organization_id, work_date DESC, created_at DESC);

CREATE TABLE IF NOT EXISTS payroll_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL DEFAULT 'Default',
    standard_hours_per_day NUMERIC(4,2) NOT NULL DEFAULT 8 CHECK (standard_hours_per_day > 0),
    overtime_after_hours NUMERIC(4,2) NOT NULL DEFAULT 8 CHECK (overtime_after_hours > 0),
    overtime_multiplier NUMERIC(4,2) NOT NULL DEFAULT 1.5 CHECK (overtime_multiplier >= 1),
    weekend_multiplier NUMERIC(4,2) NOT NULL DEFAULT 2.0 CHECK (weekend_multiplier >= 1),
    currency TEXT NOT NULL DEFAULT 'USD',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, name)
);

CREATE INDEX IF NOT EXISTS payroll_rules_org_active_idx
    ON payroll_rules (organization_id, active);

INSERT INTO permissions (code, description) VALUES
    ('payroll.read', 'View payroll rules and hour summaries'),
    ('payroll.manage', 'Manage payroll rules')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r CROSS JOIN permissions p
WHERE r.code = 'owner' AND p.code IN ('payroll.read', 'payroll.manage')
ON CONFLICT DO NOTHING;
