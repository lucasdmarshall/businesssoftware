ALTER TABLE tasks ADD COLUMN IF NOT EXISTS recurrence_next_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS tasks_recurrence_due_idx ON tasks (organization_id, recurrence_next_at) WHERE recurrence_rule IS NOT NULL;
