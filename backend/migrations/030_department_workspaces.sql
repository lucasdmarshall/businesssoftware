-- Department workspaces: membership head flag, company-wide access, per-dept module grants.

ALTER TABLE user_departments
    ADD COLUMN IF NOT EXISTS is_head BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS department_module_grants (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    department_id UUID NOT NULL REFERENCES departments(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    module_code TEXT NOT NULL
        CHECK (module_code IN (
            'overview', 'users', 'access', 'attendance', 'calendar', 'leave',
            'schedule', 'salary', 'bonus', 'tasks', 'activity', 'settings'
        )),
    can_view BOOLEAN NOT NULL DEFAULT TRUE,
    can_manage BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (department_id, user_id, module_code)
);

CREATE INDEX IF NOT EXISTS department_module_grants_user_idx
    ON department_module_grants (organization_id, user_id);

INSERT INTO permissions (code, description) VALUES
    ('company.departments.access', 'Enter and control any department workspace (company-wide)'),
    ('department.workspaces.read', 'List department workspaces the user may enter')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r CROSS JOIN permissions p
WHERE r.code = 'owner' AND p.code IN ('company.departments.access', 'department.workspaces.read')
ON CONFLICT DO NOTHING;

-- Salary & bonus (first HR-owned financial tables; always department-scoped).
CREATE TABLE IF NOT EXISTS department_salaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    department_id UUID NOT NULL REFERENCES departments(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount NUMERIC(12,2) NOT NULL CHECK (amount >= 0),
    currency TEXT NOT NULL DEFAULT 'USD',
    month DATE NOT NULL,
    withdrawn BOOLEAN NOT NULL DEFAULT FALSE,
    note TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (department_id, user_id, month)
);

CREATE TABLE IF NOT EXISTS department_bonuses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    department_id UUID NOT NULL REFERENCES departments(id) ON DELETE CASCADE,
    public_id TEXT NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_label TEXT NOT NULL DEFAULT '',
    privilege TEXT NOT NULL DEFAULT '',
    amount NUMERIC(12,2) NOT NULL CHECK (amount >= 0),
    currency TEXT NOT NULL DEFAULT 'USD',
    debited_on DATE NOT NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, public_id)
);

CREATE INDEX IF NOT EXISTS department_bonuses_dept_idx
    ON department_bonuses (department_id, debited_on DESC);

ALTER TABLE employee_profiles
    ADD COLUMN IF NOT EXISTS employee_code TEXT NOT NULL DEFAULT '';
