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
| **Aging** | AP/AR open balances by days past due (0–30 / 31–60 / 61–90 / 90+) |
| **Bank** | Bank accounts, statement lines, match to payment or journal |

## Aging reports

- `GET /api/v1/finance/aging/ap?as_of=YYYY-MM-DD`
- `GET /api/v1/finance/aging/ar?as_of=YYYY-MM-DD`
- Open amount = document total − linked payments
- AP uses open/partial bills; AR uses sent/overdue invoices
- Due date falls back to bill date / issued date / created date when missing

## Bank reconciliation

Migration `036_finance_aging_bank_recon.sql`:

- `finance_bank_accounts` — optional link to a GL cash account
- `finance_bank_transactions` — imported statement lines (`unmatched` / `matched` / `excluded`)
- `finance_bank_matches` — one match per line to a payment and/or journal

APIs:

- `GET|POST /api/v1/finance/bank-accounts`
- `GET|POST /api/v1/finance/bank-transactions`
- `POST /api/v1/finance/bank-transactions/{id}/match|unmatch|exclude`

## Architecture fit

- Permissions: `finance.read` / `finance.manage` / `finance.approve` plus
  `finance.accounts.manage`, `finance.journals.manage`, `finance.pay`
- Final approvals stay **server-authoritative** (see `docs/OFFLINE_RULES.md`)
- Workflow engine still owns expense + purchase-request approval
- Default chart of accounts is seeded per organization on first suite access
- Job titles / department workspaces stay separate; budgets may later scope by
  department membership

## What this is not (yet)

- CSV / OFX statement import
- Multi-currency revaluation
- Automated tax filings
- Inventory / three-way match depth
- Payroll payruns (separate domain)

Those can layer on the same CoA + journal + payment + bank spine.
