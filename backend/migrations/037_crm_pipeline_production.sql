-- CRM pipeline production depth: stage history, lead conversion links,
-- opportunity probability / close date usage, activity follow-ups.

ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS company_id UUID REFERENCES companies(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS converted_opportunity_id UUID REFERENCES opportunities(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS converted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS leads_org_company_idx ON leads (organization_id, company_id)
    WHERE company_id IS NOT NULL;

ALTER TABLE opportunities
    ADD COLUMN IF NOT EXISTS probability NUMERIC(5,2) NOT NULL DEFAULT 10
        CHECK (probability >= 0 AND probability <= 100),
    ADD COLUMN IF NOT EXISTS contact_id UUID REFERENCES contacts(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS lead_id UUID REFERENCES leads(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS opportunity_stage_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    opportunity_id UUID NOT NULL REFERENCES opportunities(id) ON DELETE CASCADE,
    from_stage TEXT,
    to_stage TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    changed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS opportunity_stage_history_opp_idx
    ON opportunity_stage_history (opportunity_id, changed_at DESC);
CREATE INDEX IF NOT EXISTS opportunity_stage_history_org_idx
    ON opportunity_stage_history (organization_id, changed_at DESC);

ALTER TABLE crm_activities
    ADD COLUMN IF NOT EXISTS due_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;

-- Backfill default probabilities for existing open stages.
UPDATE opportunities SET probability = CASE stage
    WHEN 'prospect' THEN 10
    WHEN 'qualified' THEN 35
    WHEN 'proposal' THEN 60
    WHEN 'won' THEN 100
    WHEN 'lost' THEN 0
    ELSE probability
END;
