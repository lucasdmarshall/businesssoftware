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

Migration: `038_core_departments_credentials_login.sql`.

## Company-issued logins

Employees do not self-register. HR / IT (or any department granted the module) issues:

| Field | Source |
|---|---|
| Username | Auto-generated from name (unique per org) |
| Password | One-time random temporary password (12+) |
| Employee ID | `EMP-YYYY-NNNN` |

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
