-- Production Attendance: durable correction history for manager edits.

CREATE TABLE IF NOT EXISTS attendance_corrections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    attendance_id UUID REFERENCES attendance_records(id) ON DELETE SET NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    corrected_by UUID NOT NULL REFERENCES users(id),
    work_date DATE NOT NULL,
    previous_check_in_at TIMESTAMPTZ,
    previous_check_out_at TIMESTAMPTZ,
    previous_status TEXT NOT NULL DEFAULT '',
    previous_note TEXT NOT NULL DEFAULT '',
    check_in_at TIMESTAMPTZ,
    check_out_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'present',
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS attendance_corrections_org_date_idx
    ON attendance_corrections (organization_id, work_date DESC, created_at DESC);

CREATE INDEX IF NOT EXISTS attendance_corrections_user_idx
    ON attendance_corrections (organization_id, user_id, created_at DESC);
