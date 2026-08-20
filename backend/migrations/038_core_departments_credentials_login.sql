-- Username login, core departments, credentials module, soft-archive depts.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS username TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS users_org_username_uidx
    ON users (organization_id, lower(username))
    WHERE username IS NOT NULL AND username <> '';

-- Backfill username from email local-part when missing.
UPDATE users
SET username = split_part(lower(email), '@', 1)
WHERE (username IS NULL OR username = '')
  AND email IS NOT NULL AND email <> '';

ALTER TABLE departments
    ADD COLUMN IF NOT EXISTS is_core BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS organization_employee_counters (
    organization_id UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    next_number INT NOT NULL DEFAULT 1
);

-- Widen module CHECKs for credentials generator.
ALTER TABLE department_position_modules DROP CONSTRAINT IF EXISTS department_position_modules_module_code_check;
ALTER TABLE department_position_modules ADD CONSTRAINT department_position_modules_module_code_check
    CHECK (module_code IN (
        'overview', 'users', 'access', 'positions', 'attendance', 'calendar', 'leave',
        'schedule', 'salary', 'bonus', 'finance', 'credentials', 'tasks', 'activity', 'settings'
    ));

ALTER TABLE department_module_grants DROP CONSTRAINT IF EXISTS department_module_grants_module_code_check;
ALTER TABLE department_module_grants ADD CONSTRAINT department_module_grants_module_code_check
    CHECK (module_code IN (
        'overview', 'users', 'access', 'positions', 'attendance', 'calendar', 'leave',
        'schedule', 'salary', 'bonus', 'finance', 'credentials', 'tasks', 'activity', 'settings'
    ));

-- Seed core departments for every existing organization (idempotent by slug).
INSERT INTO departments (organization_id, name, slug, is_core)
SELECT o.id, v.name, v.slug, TRUE
FROM organizations o
CROSS JOIN (VALUES
    ('Administrations', 'administrations'),
    ('HR', 'hr'),
    ('Sales', 'sales'),
    ('Finance', 'finance'),
    ('IT', 'it'),
    ('Operations', 'operations'),
    ('Marketing', 'marketing'),
    ('Procurement', 'procurement'),
    ('Customer Service', 'customer-service'),
    ('Reports and Analytics', 'reports-and-analytics'),
    ('Legal and Compliance', 'legal-and-compliance')
) AS v(name, slug)
ON CONFLICT (organization_id, slug) DO UPDATE
SET is_core = TRUE,
    name = CASE WHEN departments.archived_at IS NULL THEN departments.name ELSE EXCLUDED.name END;

-- Default: credentials manage for heads of HR + IT; view for managers there.
INSERT INTO department_position_modules (department_id, position_id, module_code, can_view, can_manage)
SELECT p.department_id, p.id, 'credentials', TRUE, TRUE
FROM department_positions p
JOIN departments d ON d.id = p.department_id
WHERE p.code = 'head' AND d.slug IN ('hr', 'it') AND d.archived_at IS NULL
ON CONFLICT (position_id, module_code) DO NOTHING;

INSERT INTO department_position_modules (department_id, position_id, module_code, can_view, can_manage)
SELECT p.department_id, p.id, 'credentials', TRUE, TRUE
FROM department_positions p
JOIN departments d ON d.id = p.department_id
WHERE p.code = 'manager' AND d.slug IN ('hr', 'it') AND d.archived_at IS NULL
ON CONFLICT (position_id, module_code) DO NOTHING;
