-- Generic reusable approval workflow engine.
-- One engine drives approvals for leave, finance, and any future module.

CREATE TABLE IF NOT EXISTS workflow_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    entity_type TEXT NOT NULL DEFAULT 'generic',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, code)
);

-- Each step names who may act and, optionally, an amount band that makes the
-- step apply. required_approvals > 1 turns a step into a parallel approval that
-- advances only once that many distinct approvers have approved.
CREATE TABLE IF NOT EXISTS workflow_steps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    definition_id UUID NOT NULL REFERENCES workflow_definitions(id) ON DELETE CASCADE,
    step_order INTEGER NOT NULL,
    name TEXT NOT NULL,
    approver_role_code TEXT,
    approver_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    required_approvals INTEGER NOT NULL DEFAULT 1 CHECK (required_approvals >= 1),
    min_amount NUMERIC,
    max_amount NUMERIC,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (definition_id, step_order)
);

CREATE INDEX IF NOT EXISTS workflow_steps_def_idx ON workflow_steps (definition_id, step_order);

CREATE TABLE IF NOT EXISTS workflow_instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    definition_id UUID NOT NULL REFERENCES workflow_definitions(id),
    title TEXT NOT NULL,
    entity_type TEXT NOT NULL DEFAULT 'generic',
    entity_id UUID,
    amount NUMERIC,
    status TEXT NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'in_review', 'approved', 'rejected', 'cancelled')),
    current_step_order INTEGER,
    submitted_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS workflow_instances_org_status_idx
    ON workflow_instances (organization_id, status, updated_at DESC);

-- Complete, append-only approval history for every instance.
CREATE TABLE IF NOT EXISTS workflow_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id UUID NOT NULL REFERENCES workflow_instances(id) ON DELETE CASCADE,
    step_order INTEGER,
    actor_id UUID REFERENCES users(id),
    action TEXT NOT NULL CHECK (action IN ('submit', 'approve', 'reject', 'resubmit', 'cancel', 'comment')),
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS workflow_actions_instance_idx ON workflow_actions (instance_id, created_at);

INSERT INTO permissions (code, description) VALUES
    ('workflow.read', 'View workflows and approvals'),
    ('workflow.manage', 'Create workflow definitions and submit requests'),
    ('workflow.act', 'Approve or reject workflow steps')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r CROSS JOIN permissions p
WHERE r.code = 'owner' AND p.code IN ('workflow.read', 'workflow.manage', 'workflow.act')
ON CONFLICT DO NOTHING;
