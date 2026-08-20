-- Department position hierarchy: access inside a department follows position.
-- Position Visualizer shows this tree (including custom positions) as a list.
-- Example (HR / User Management):
--   head     → users read+write
--   manager  → users read
--   employee → users read

CREATE TABLE IF NOT EXISTS department_positions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    department_id UUID NOT NULL REFERENCES departments(id) ON DELETE CASCADE,
    parent_id UUID REFERENCES department_positions(id) ON DELETE SET NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    rank_order INT NOT NULL DEFAULT 0,
    is_system BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (department_id, code)
);

CREATE INDEX IF NOT EXISTS department_positions_dept_rank_idx
    ON department_positions (department_id, rank_order DESC);

CREATE INDEX IF NOT EXISTS department_positions_parent_idx
    ON department_positions (department_id, parent_id);

-- Default module rights for each position in a department.
CREATE TABLE IF NOT EXISTS department_position_modules (
    department_id UUID NOT NULL REFERENCES departments(id) ON DELETE CASCADE,
    position_id UUID NOT NULL REFERENCES department_positions(id) ON DELETE CASCADE,
    module_code TEXT NOT NULL
        CHECK (module_code IN (
            'overview', 'users', 'access', 'positions', 'attendance', 'calendar', 'leave',
            'schedule', 'salary', 'bonus', 'tasks', 'activity', 'settings'
        )),
    can_view BOOLEAN NOT NULL DEFAULT TRUE,
    can_manage BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (position_id, module_code)
);

ALTER TABLE user_departments
    ADD COLUMN IF NOT EXISTS position_id UUID REFERENCES department_positions(id) ON DELETE SET NULL;

-- Allow positions module on per-user grant overrides too.
ALTER TABLE department_module_grants DROP CONSTRAINT IF EXISTS department_module_grants_module_code_check;
ALTER TABLE department_module_grants ADD CONSTRAINT department_module_grants_module_code_check
    CHECK (module_code IN (
        'overview', 'users', 'access', 'positions', 'attendance', 'calendar', 'leave',
        'schedule', 'salary', 'bonus', 'tasks', 'activity', 'settings'
    ));

-- Seed three standard positions for every existing department.
INSERT INTO department_positions (organization_id, department_id, code, name, rank_order, is_system)
SELECT d.organization_id, d.id, v.code, v.name, v.rank_order, TRUE
FROM departments d
CROSS JOIN (VALUES
    ('head', 'Department head', 100),
    ('manager', 'Manager', 50),
    ('employee', 'Employee', 10)
) AS v(code, name, rank_order)
ON CONFLICT (department_id, code) DO UPDATE SET is_system = TRUE, name = EXCLUDED.name;

-- Wire default parent chain: head → manager → employee.
UPDATE department_positions child
SET parent_id = parent.id
FROM department_positions parent
WHERE child.department_id = parent.department_id
  AND parent.code = 'head'
  AND child.code = 'manager'
  AND child.parent_id IS NULL;

UPDATE department_positions child
SET parent_id = parent.id
FROM department_positions parent
WHERE child.department_id = parent.department_id
  AND parent.code = 'manager'
  AND child.code = 'employee'
  AND child.parent_id IS NULL;

-- Head: full manage on every module (including Position Visualizer).
INSERT INTO department_position_modules (department_id, position_id, module_code, can_view, can_manage)
SELECT p.department_id, p.id, m.code, TRUE, TRUE
FROM department_positions p
CROSS JOIN (VALUES
    ('overview'),('users'),('access'),('positions'),('attendance'),('calendar'),('leave'),
    ('schedule'),('salary'),('bonus'),('tasks'),('activity'),('settings')
) AS m(code)
WHERE p.code = 'head'
ON CONFLICT (position_id, module_code) DO NOTHING;

-- Manager: read operational modules + positions list; no write on users/access/salary/bonus/settings.
INSERT INTO department_position_modules (department_id, position_id, module_code, can_view, can_manage)
SELECT p.department_id, p.id, m.code, TRUE, FALSE
FROM department_positions p
CROSS JOIN (VALUES
    ('overview'),('users'),('positions'),('attendance'),('calendar'),('leave'),
    ('schedule'),('tasks'),('activity')
) AS m(code)
WHERE p.code = 'manager'
ON CONFLICT (position_id, module_code) DO NOTHING;

-- Employee: read limited self-service modules (including users list + Position Visualizer).
INSERT INTO department_position_modules (department_id, position_id, module_code, can_view, can_manage)
SELECT p.department_id, p.id, m.code, TRUE, FALSE
FROM department_positions p
CROSS JOIN (VALUES
    ('overview'),('users'),('positions'),('attendance'),('calendar'),('leave'),
    ('schedule'),('tasks'),('activity')
) AS m(code)
WHERE p.code = 'employee'
ON CONFLICT (position_id, module_code) DO NOTHING;

-- Backfill: existing heads → head position; everyone else → employee.
UPDATE user_departments ud
SET position_id = p.id
FROM department_positions p
WHERE p.department_id = ud.department_id AND p.code = 'head' AND ud.is_head = TRUE AND ud.position_id IS NULL;

UPDATE user_departments ud
SET position_id = p.id
FROM department_positions p
WHERE p.department_id = ud.department_id AND p.code = 'employee' AND ud.is_head = FALSE AND ud.position_id IS NULL;
