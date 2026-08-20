-- Company attendance policy + employee ID for attendance desk UI.

CREATE TABLE IF NOT EXISTS organization_attendance_settings (
    organization_id UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    expected_check_in_time TIME NOT NULL DEFAULT TIME '09:00:00',
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE employee_profiles
    ADD COLUMN IF NOT EXISTS employee_code TEXT NOT NULL DEFAULT '';

-- Seed default policy for existing orgs (09:00). Company-level roles may change it;
-- department heads cannot.
INSERT INTO organization_attendance_settings (organization_id, expected_check_in_time)
SELECT id, TIME '09:00:00' FROM organizations
ON CONFLICT (organization_id) DO NOTHING;
