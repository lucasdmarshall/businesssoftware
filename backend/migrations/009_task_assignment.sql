ALTER TABLE tasks ADD COLUMN IF NOT EXISTS assigned_to UUID REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS tasks_assigned_to_idx ON tasks (organization_id, assigned_to, updated_at DESC);
