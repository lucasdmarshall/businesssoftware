# Department Workspaces

Product decision (2026-08-20): the product is **not** one flat company app with
global module pages. Each department is its own workspace. Company-wide roles
may enter any department; everyone else only sees departments they belong to.

## Target shape (example: HR)

When the user opens **HR**, navigation becomes HR-scoped:

1. **Overview** — HR KPIs only  
2. **User Management** — HR people (phone, employee ID, …)  
3. **Access Control** — assign a **department position**; access follows that
   position’s module matrix (optional per-user exceptions on top)  
4. **Position Visualizer** — list view of the department position ladder
   (system + **custom** positions). Drag vertically to reorder; **top = highest
   seniority** in this department. That order answers “who sits above whom.”  
5. **Attendance** — HR only  
6. **Calendar** — HR events; **organization-wide events may also appear**  
7. **Leave** — HR only  
8. **Schedule** — HR only  
9. **Salary management** — department-needed payroll tracking  
10. **Bonus management** — department-needed payouts  
11. **Finance** — department money needs (salary/bonus shortcuts) **plus a
    tracking table** for expenses/reimbursements/petty cash. This is **not**
    the company Finance suite (CoA / AP / AR / journals).  
12. **Tasks** — department only  
13. **Activity** — department audit trail only  
14. **Settings** — department settings  

## Department Finance vs company Finance suite

| Layer | Owns |
|---|---|
| **Company Finance suite** | Chart of accounts, journals, vendors/customers, AP bills, AR invoices, payments, tax |
| **Department Finance module** | What *this* department needs (e.g. HR salary/bonus) + a simple tracking ledger table |

Departments never get a second general ledger. They track operational money
inside their workspace; company Finance remains the system of record.

## Isolation rule

| Actor | What they see |
|---|---|
| HR member / Head of HR | HR workspace data only |
| IT member | IT workspace data only — **cannot** open HR Attendance/Leave/Salary/… |
| Company-wide access (Owner, CEO-class, explicit grant) | Can enter any department workspace and manage/control it |

Job title alone does **not** grant access. Access is:

1. **Department membership** (which workspaces you may enter), plus  
2. **Department position** (ladder rank + module rights inside that workspace), plus  
3. **Company-wide override** (explicit permission, not seniority by title).

This matches how companies normally govern: hierarchy defines who is senior;
position defines what you can do; exceptions are rare overrides.

## Position ladder (Position Visualizer)

- Default system positions: **Department head → Manager → Employee**
- Departments may add **custom** positions (e.g. Team lead)
- Visualizer is a **list** (not a graph): drag rows up/down
- Reorder persists `rank_order` (and a linear parent chain) so the top row is
  always the highest position in that department
- Access Control assigns people to a position on this ladder

## Access Control menu (per department)

- Shown only if the user’s position grants `access` **or** they hold
  company-wide department access (heads get manage by default)
- Primary action: pick a **position** for each member
- Module checkboxes are **exceptions** layered on position defaults

## Calendar exception

Department calendars are department-scoped **and** may include events marked
`visibility = organization` so company-wide announcements still surface.

## Mapping to today’s code

- Department switcher → enter a workspace  
- Workspace APIs under `/api/v1/workspaces/departments/...`  
- Workspace foundation: migration `030_department_workspaces.sql`
- Position seed + reorder: migration `031_department_position_access.sql`
- Department Finance tracking table: migration `032_department_finance.sql`  
- Remaining gap: Calendar / Schedule still need the same `department_id`
  query pattern end-to-end (Leave / Attendance / Tasks / Activity are scoped)

## Non-goals (for the first cut)

- Automatic permission inheritance from company job-title catalogue strings  
- Letting one department peek at another’s operational data without company-wide grant  
