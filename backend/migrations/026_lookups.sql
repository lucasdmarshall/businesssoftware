-- Admin-managed dropdown option lists (a.k.a. lookups / reference data).
-- Privileged roles edit the values that populate dropdowns across the product
-- (leave types, expense categories, ticket priorities, and so on). Each
-- category is governed by a domain "manage" permission enforced in the API.

CREATE TABLE IF NOT EXISTS lookup_options (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    category TEXT NOT NULL,
    value TEXT NOT NULL,
    label TEXT NOT NULL,
    color TEXT,
    sort_order INT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, category, value)
);

CREATE INDEX IF NOT EXISTS idx_lookup_options_org_cat
    ON lookup_options (organization_id, category, sort_order);
