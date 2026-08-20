-- Schedule production depth: assignee rules stay on shifts; add creator + soft
-- uniqueness helpers for overlap checks in application code.

ALTER TABLE shifts
    ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS shifts_assignee_date_idx
    ON shifts (organization_id, assigned_to, shift_date)
    WHERE status <> 'cancelled';
