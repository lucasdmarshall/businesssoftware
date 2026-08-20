# Core departments, credentials, and login home

## Core departments

Every organization seeds these departments (slug in parentheses):

- Administrations (`administrations`)
- HR (`hr`)
- Sales (`sales`)
- Finance (`finance`)
- IT (`it`)
- Operations (`operations`)
- Marketing (`marketing`)
- Procurement (`procurement`)
- Customer Service (`customer-service`)
- Reports and Analytics (`reports-and-analytics`)
- Legal and Compliance (`legal-and-compliance`)

CEO/admin (`organization.manage`) can **rename**, **archive/remove**, or **add** departments from Company → People.

Migrations: `038_core_departments_credentials_login.sql`, `039_credential_id_format.sql`.

## Company-issued logins

Employees do not self-register. HR / IT (or any department granted the module) issues credentials with a **per-department sequence**:

| Field | Format | Example (Administrations, seq 1) |
|---|---|---|
| Username | `{firstname}_{dept_tag}_{NNNNNN}` | `lucas_admin_000001` |
| Password | `password{NNNNNN}` | `password000001` |
| Employee ID | `{PREFIX}{NNNNNN}` | `ADM000001` |

Department tags / EMP prefixes:

| Department | Username tag | EMP prefix |
|---|---|---|
| Administrations | `admin` | `ADM` |
| HR | `hr` | `HR` |
| Sales | `sales` | `SAL` |
| Finance | `finance` | `FIN` |
| IT | `it` | `IT` |
| Operations | `ops` | `OPS` |
| Marketing | `mkt` | `MKT` |
| Procurement | `proc` | `PRC` |
| Customer Service | `cs` | `CS` |
| Reports and Analytics | `analytics` | `RPT` |
| Legal and Compliance | `legal` | `LEG` |

Custom departments derive a tag from the first slug token and a 2–3 letter EMP prefix.

Default passwords meet the 12+ character policy (`password` + 6 digits = 14). Employees change their password from **department Settings** (`POST /api/v1/auth/change-password`).

APIs:

- `POST /api/v1/credentials/generate` — company People (`users.manage`)
- `POST /api/v1/workspaces/departments/{id}/credentials` — department **credentials** module manage

Login: `POST /api/v1/auth/login` with `{ "username", "password" }` (email still accepted as fallback).

## Credentials module

Grantable like Salary / Bonus / Finance via Access Control:

- Default: **HR** and **IT** head + manager get `credentials` manage
- Admin/CEO can grant the module to any other department’s users

UI: Department → **Credentials** tab, and Company → People → Credentials generator.

## Post-login home

`GET /auth/me` returns `home_department_id`:

1. Sole department membership, or
2. Primary membership (`is_primary`), or
3. `users.primary_department_id`

The app auto-opens that department workspace dashboard after sign-in.

## Logout

`POST /api/v1/auth/logout` is wired in company and department shells (sidebar + topbar).
