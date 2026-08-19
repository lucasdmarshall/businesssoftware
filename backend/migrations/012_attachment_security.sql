ALTER TABLE task_attachments ADD COLUMN IF NOT EXISTS scan_status TEXT NOT NULL DEFAULT 'unscanned';
ALTER TABLE task_attachments ADD CONSTRAINT task_attachments_scan_status_check CHECK (scan_status IN ('unscanned','clean','quarantined','infected'));
