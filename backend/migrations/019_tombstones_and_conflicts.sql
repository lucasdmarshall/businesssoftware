-- Offline capability: deletion tombstones and a conflict history so device
-- clients can reconcile deletions and merge conflicts after reconnecting.

-- Tombstones record deletions so offline clients receive them through the pull
-- feed and can drop the local copy.
CREATE TABLE IF NOT EXISTS tombstones (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    entity TEXT NOT NULL,
    entity_id UUID NOT NULL,
    deleted_by UUID REFERENCES users(id) ON DELETE SET NULL,
    deleted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, entity, entity_id)
);

CREATE INDEX IF NOT EXISTS tombstones_org_idx ON tombstones (organization_id, deleted_at);

-- Sync conflicts keep the client's payload and the reason so a person can review
-- and resolve, choosing the client version or the server version.
CREATE TABLE IF NOT EXISTS sync_conflicts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    operation_id TEXT NOT NULL,
    entity TEXT NOT NULL,
    action TEXT NOT NULL,
    client_payload JSONB NOT NULL,
    reason TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved_client', 'resolved_server')),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    UNIQUE (organization_id, operation_id)
);

CREATE INDEX IF NOT EXISTS sync_conflicts_org_status_idx ON sync_conflicts (organization_id, status, created_at DESC);
