# CRM Pipeline

Sales/CRM production depth on top of the Phase 7 scaffold (`024_crm.sql`).

## Suite map

| Tab | Purpose |
|---|---|
| **Pipeline** | Stage board (prospect → qualified → proposal → won/lost), create deals, move stages |
| **Leads** | Capture + status + **Convert** to opportunity |
| **Companies** | Account registry |
| **Contacts** | People linked to companies |
| **Activities** | Call / email / meeting / note timeline |

## Migration `037_crm_pipeline_production.sql`

- `opportunity_stage_history` — from/to stage, actor, note
- Opportunity `probability` (auto-set on stage move), `contact_id`, `lead_id`
- Lead `company_id`, `converted_opportunity_id`, `converted_at`
- Activity optional `due_at` / `completed_at` (reserved for follow-ups)

Default probabilities: prospect 10 · qualified 35 · proposal 60 · won 100 · lost 0.

## APIs

| Endpoint | Role |
|---|---|
| `GET /api/v1/crm/pipeline/summary` | Open / weighted / won / per-stage buckets |
| `POST /api/v1/crm/opportunities/{id}/stage` | Move stage + history + activity + audit |
| `GET /api/v1/crm/opportunities/{id}/history` | Stage change log |
| `POST /api/v1/crm/leads/{id}/convert` | Create opportunity (+ contact/company) and mark lead converted |
| Existing CRUD | Companies, contacts, leads, opportunities, activities |

`GET /api/v1/analytics/sales` now returns live CRM totals (`module_available: true`).

## What this is not (yet)

- Custom org-defined stage catalogs
- Quotations / proposals documents
- Sales targets / quotas
- Deep revenue forecasting beyond weighted pipeline
- Merge with Finance customers table

Those can layer on the same opportunity + stage-history spine.
