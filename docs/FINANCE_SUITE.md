# Finance Suite

Product decision (2026-08-20): Finance is not a single “expenses + vendors”
page. It is a **full finance suite** inside the company app — the ledger spine
plus payables, receivables, cash, and the existing expense/PR/budget flows.

## Suite map

| Area | Purpose |
|---|---|
| **Overview** | Cash / AP open / AR open / budget burn snapshot |
| **Chart of Accounts** | Asset · Liability · Equity · Revenue · Expense codes |
| **Journals** | Double-entry drafts → post (debits must equal credits) |
| **Tax codes** | Rates applied on bills/invoices |
| **Vendors** | AP counterparties |
| **Bills** | Vendor bills (AP) — open → pay |
| **Customers** | AR counterparties |
| **Invoices** | Customer invoices (AR) — draft → sent → paid |
| **Expenses** | Employee spend + workflow approval |
| **Purchase requests** | Procurement drafts + workflow |
| **Budgets** | Period envelopes vs paid spend |
| **Payments** | Cash in/out linked to bill or invoice |

## Architecture fit

- Permissions: `finance.read` / `finance.manage` / `finance.approve` plus
  `finance.accounts.manage`, `finance.journals.manage`, `finance.pay`
- Final approvals stay **server-authoritative** (see `docs/OFFLINE_RULES.md`)
- Workflow engine still owns expense + purchase-request approval
- Default chart of accounts is seeded per organization on first suite access
- Job titles / department workspaces stay separate; budgets may later scope by
  department membership

## What this is not (yet)

- Full bank reconciliation UI
- Multi-currency revaluation
- Automated tax filings
- Inventory / three-way match depth
- Payroll payruns (separate domain)

Those can layer on the same CoA + journal + payment spine.
