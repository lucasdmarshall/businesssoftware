-- Department-level finance: operational money tracking inside a workspace.
-- This is NOT the company Finance suite (CoA / AP / AR / journals).
-- Departments keep salary/bonus (when enabled) plus a simple tracking ledger.

CREATE TABLE IF NOT EXISTS department_finance_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    department_id UUID NOT NULL REFERENCES departments(id) ON DELETE CASCADE,
    entry_date DATE NOT NULL DEFAULT CURRENT_DATE,
    category TEXT NOT NULL DEFAULT 'expense'
        CHECK (category IN (
            'expense', 'reimbursement', 'petty_cash', 'allowance',
            'salary', 'bonus', 'other'
        )),
    direction TEXT NOT NULL DEFAULT 'out' CHECK (direction IN ('in', 'out')),
    title TEXT NOT NULL,
    amount NUMERIC(14,2) NOT NULL CHECK (amount >= 0),
    currency TEXT NOT NULL DEFAULT 'USD',
    person_id UUID REFERENCES users(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'recorded'
        CHECK (status IN ('recorded', 'pending', 'settled', 'void')),
    note TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS department_finance_entries_dept_date_idx
    ON department_finance_entries (department_id, entry_date DESC, created_at DESC);

-- Allow finance module on position matrix + per-user overrides.
ALTER TABLE department_position_modules DROP CONSTRAINT IF EXISTS department_position_modules_module_code_check;
ALTER TABLE department_position_modules ADD CONSTRAINT department_position_modules_module_code_check
    CHECK (module_code IN (
        'overview', 'users', 'access', 'positions', 'attendance', 'calendar', 'leave',
        'schedule', 'salary', 'bonus', 'finance', 'tasks', 'activity', 'settings'
    ));

ALTER TABLE department_module_grants DROP CONSTRAINT IF EXISTS department_module_grants_module_code_check;
ALTER TABLE department_module_grants ADD CONSTRAINT department_module_grants_module_code_check
    CHECK (module_code IN (
        'overview', 'users', 'access', 'positions', 'attendance', 'calendar', 'leave',
        'schedule', 'salary', 'bonus', 'finance', 'tasks', 'activity', 'settings'
    ));

-- Head: manage finance tracker.
INSERT INTO department_position_modules (department_id, position_id, module_code, can_view, can_manage)
SELECT p.department_id, p.id, 'finance', TRUE, TRUE
FROM department_positions p
WHERE p.code = 'head'
ON CONFLICT (position_id, module_code) DO NOTHING;

-- Manager / employee: view department finance tracker (read-only by default).
INSERT INTO department_position_modules (department_id, position_id, module_code, can_view, can_manage)
SELECT p.department_id, p.id, 'finance', TRUE, FALSE
FROM department_positions p
WHERE p.code IN ('manager', 'employee')
ON CONFLICT (position_id, module_code) DO NOTHING;
