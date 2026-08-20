# Department Workspaces

Product decision (2026-08-20): the product is **not** one flat company app with
global module pages. Each department is its own workspace. Company-wide roles
may enter any department; everyone else only sees departments they belong to.

## Target shape (example: HR)

When the user opens **HR**, navigation becomes HR-scoped:

1. **Overview** — HR KPIs only  
2. **User Management** — HR people (phone, employee ID, …)  
3. **Access Control** — visible only to department heads and company-wide
   privileged users; sets per-user access **inside this department**  
4. **Attendance** — HR only  
5. **Calendar** — HR events; **organization-wide events may also appear**  
6. **Leave** — HR only  
7. **Schedule** — HR only  
8. **Salary management** — HR only (amount, withdrawn/not, month, …)  
9. **Bonus management** — HR only  
   - Columns: Name, Role, Privilege (e.g. punctuality allowance), Amount,
     Debited on (date), ID (system-assigned random 8-digit, stored in DB)  
10. **Tasks** — HR only (assigned tasks, grouping, goals, projects)  
11. **Activity** — HR audit trail only  
12. **Settings** — HR department settings  

IT, Finance, Sales, etc. get the same *pattern* with their own module set.

## Isolation rule

| Actor | What they see |
|---|---|
| HR member / Head of HR | HR workspace data only |
| IT member | IT workspace data only — **cannot** open HR Attendance/Leave/Salary/… |
| Company-wide access (Owner, CEO-class, explicit grant) | Can enter any department workspace and manage/control it |

Job title alone does **not** grant access. Access is:

1. **Department membership** (which workspaces you may enter), plus  
2. **Department-scoped permissions** (what you can do inside that workspace), plus  
3. **Company-wide override** (explicit permission, not seniority by title).

## Access Control menu (per department)

- Shown only if the user is a **department head** for that department **or**
  holds company-wide department access.
- Edits grants for users **in this department only** (not a global role editor
  for the whole company — company Owner still has org-wide Access in People /
  Settings as needed).

## Calendar exception

Department calendars are department-scoped **and** may include events marked
`visibility = organization` so company-wide announcements still surface.

## Mapping to today’s code (gap)

Today the app is a single shell with org-wide Leave/Attendance/Finance/… pages
and a soft Overview scope dropdown. That does **not** match this model.

Implementation moves toward:

- Department switcher → enter a workspace  
- Every HR (etc.) query filtered by `department_id` (deny-by-default)  
- Company-wide permission bypasses the filter  
- New HR modules: Salary, Bonus  
- Per-department Access Control UI  

## Non-goals (for the first cut)

- Automatic permission inheritance from job title strings (“Head of HR”)  
- Letting one department peek at another’s operational data without company-wide grant  
