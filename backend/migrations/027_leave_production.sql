-- Production Leave module: balances, policies, richer requests, and cancel support.

CREATE TABLE IF NOT EXISTS leave_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    leave_type TEXT NOT NULL,
    entitled_days NUMERIC(6,2) NOT NULL DEFAULT 0 CHECK (entitled_days >= 0),
    allow_half_day BOOLEAN NOT NULL DEFAULT TRUE,
    requires_balance BOOLEAN NOT NULL DEFAULT TRUE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, leave_type)
);

CREATE TABLE IF NOT EXISTS leave_balances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    leave_type TEXT NOT NULL,
    year INT NOT NULL CHECK (year >= 2000 AND year <= 2100),
    entitled_days NUMERIC(6,2) NOT NULL DEFAULT 0 CHECK (entitled_days >= 0),
    used_days NUMERIC(6,2) NOT NULL DEFAULT 0 CHECK (used_days >= 0),
    carried_over_days NUMERIC(6,2) NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, user_id, leave_type, year)
);

CREATE INDEX IF NOT EXISTS leave_balances_org_user_idx
    ON leave_balances (organization_id, user_id, year);

ALTER TABLE leave_requests
    ADD COLUMN IF NOT EXISTS total_days NUMERIC(6,2) NOT NULL DEFAULT 1 CHECK (total_days > 0),
    ADD COLUMN IF NOT EXISTS half_day BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS workflow_instance_id UUID REFERENCES workflow_instances(id) ON DELETE SET NULL;

-- Drop the hard-coded type check so Settings lookup lists can extend leave types.
ALTER TABLE leave_requests DROP CONSTRAINT IF EXISTS leave_requests_leave_type_check;

CREATE INDEX IF NOT EXISTS leave_requests_overlap_idx
    ON leave_requests (organization_id, requested_by, status, start_date, end_date);

-- Seed default policies for every organization that does not yet have them.
INSERT INTO leave_policies (organization_id, leave_type, entitled_days, requires_balance)
SELECT o.id, v.leave_type, v.entitled_days, v.requires_balance
FROM organizations o
CROSS JOIN (VALUES
    ('annual', 20.0, TRUE),
    ('sick', 10.0, TRUE),
    ('personal', 5.0, TRUE),
    ('unpaid', 0.0, FALSE)
) AS v(leave_type, entitled_days, requires_balance)
ON CONFLICT (organization_id, leave_type) DO NOTHING;
