-- Phase 5: delegated approval, step SLAs / instance deadlines, reminder log.
-- Also links leave requests to the generic workflow engine.

CREATE TABLE IF NOT EXISTS workflow_delegations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    delegator_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    delegate_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    starts_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ends_at TIMESTAMPTZ,
    reason TEXT NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (delegator_id <> delegate_id),
    CHECK (ends_at IS NULL OR ends_at > starts_at)
);

CREATE INDEX IF NOT EXISTS workflow_delegations_org_delegate_idx
    ON workflow_delegations (organization_id, delegate_id, active);

CREATE INDEX IF NOT EXISTS workflow_delegations_org_delegator_idx
    ON workflow_delegations (organization_id, delegator_id, active);

ALTER TABLE workflow_steps
    ADD COLUMN IF NOT EXISTS sla_hours INTEGER
        CHECK (sla_hours IS NULL OR sla_hours > 0);

ALTER TABLE workflow_instances
    ADD COLUMN IF NOT EXISTS due_at TIMESTAMPTZ;

ALTER TABLE workflow_instances
    ADD COLUMN IF NOT EXISTS last_reminded_at TIMESTAMPTZ;

ALTER TABLE workflow_actions
    ADD COLUMN IF NOT EXISTS on_behalf_of UUID REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE workflow_actions DROP CONSTRAINT IF EXISTS workflow_actions_action_check;
ALTER TABLE workflow_actions ADD CONSTRAINT workflow_actions_action_check
    CHECK (action IN ('submit', 'approve', 'reject', 'resubmit', 'cancel', 'comment', 'delegate', 'remind'));

CREATE TABLE IF NOT EXISTS workflow_reminders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    instance_id UUID NOT NULL REFERENCES workflow_instances(id) ON DELETE CASCADE,
    recipient_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    step_order INTEGER,
    kind TEXT NOT NULL DEFAULT 'upcoming' CHECK (kind IN ('upcoming', 'overdue')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS workflow_reminders_instance_idx
    ON workflow_reminders (instance_id, created_at DESC);

ALTER TABLE leave_requests
    ADD COLUMN IF NOT EXISTS workflow_instance_id UUID REFERENCES workflow_instances(id) ON DELETE SET NULL;
