-- Platform core: reporting lines, job titles/positions, and a generic file
-- model. Per the RBAC model, job title, position, department, reporting
-- manager, permission role, and data scope are separate concepts and must not
-- be merged into one field.

-- Job titles: the professional label catalogue (e.g. "Finance Manager"),
-- separate from permission roles. Category maps to the corporate hierarchy
-- groups in corporate-role-hierarchy.md.
CREATE TABLE IF NOT EXISTS job_titles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'professional'
        CHECK (category IN ('governance', 'executive', 'general_management', 'senior_management', 'middle_management', 'professional', 'junior')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, name)
);

CREATE INDEX IF NOT EXISTS job_titles_org_idx ON job_titles (organization_id, category);

-- A person's placement: which job title they hold and which department seat is
-- their primary one. Permission roles remain entirely separate (user_roles).
ALTER TABLE users ADD COLUMN IF NOT EXISTS job_title_id UUID REFERENCES job_titles(id) ON DELETE SET NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS primary_department_id UUID REFERENCES departments(id) ON DELETE SET NULL;

-- Reporting lines: who reports to whom, distinct from department membership and
-- permission roles. A user may have one primary manager plus dotted-line ones.
CREATE TABLE IF NOT EXISTS reporting_lines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    manager_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    is_primary BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, manager_id),
    CHECK (user_id <> manager_id)
);

CREATE INDEX IF NOT EXISTS reporting_lines_manager_idx ON reporting_lines (organization_id, manager_id);
CREATE UNIQUE INDEX IF NOT EXISTS reporting_lines_one_primary_idx
    ON reporting_lines (user_id) WHERE is_primary;

-- Generic platform file model. Task attachments keep their own specialized
-- table; this is the shared files record other modules reference.
CREATE TABLE IF NOT EXISTS files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    uploaded_by UUID REFERENCES users(id) ON DELETE SET NULL,
    entity_type TEXT NOT NULL DEFAULT 'generic',
    entity_id UUID,
    filename TEXT NOT NULL,
    storage_key TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    scan_status TEXT NOT NULL DEFAULT 'unscanned'
        CHECK (scan_status IN ('unscanned', 'clean', 'quarantined', 'infected')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS files_org_entity_idx ON files (organization_id, entity_type, entity_id);
