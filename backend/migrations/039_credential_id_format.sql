-- Per-department employee sequence for username / password / EMP ID formats.
-- Example (Administrations): lucas_admin_000001 / password000001 / ADM000001

CREATE TABLE IF NOT EXISTS department_employee_counters (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    department_id UUID NOT NULL REFERENCES departments(id) ON DELETE CASCADE,
    next_number INT NOT NULL DEFAULT 1,
    PRIMARY KEY (organization_id, department_id)
);

CREATE INDEX IF NOT EXISTS department_employee_counters_dept_idx
    ON department_employee_counters (department_id);
