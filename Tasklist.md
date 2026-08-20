# Development Task List

This is the master implementation checklist for the installable enterprise business software.

## Legend

- `[ ]` Not started
- `[-]` In progress
- `[x]` Completed
- `[!]` Blocked or needs a decision

## Phase 0 — Workspace and Engineering Foundation

- [x] Confirm product distribution model: self-hosted installable software
- [x] Confirm backend language: Go
- [x] Confirm desktop shell: Tauri
- [x] Confirm frontend: React + TypeScript
- [x] Confirm database: PostgreSQL
- [x] Confirm design system direction
- [x] Create source repository structure
- [x] Add root README and contributor guide
- [x] Read the LukeLang `AGENTS.md` playbook before writing any LukeLang (reactive engine) code — canonical repo: https://github.com/lucasdmarshall/LukeLang
- [x] Define development commands and scripts (root `Makefile`, `docs/DEVELOPMENT.md`)
- [x] Define environment variable conventions (`docs/DEVELOPMENT.md`, `.env.example`)
- [x] Define local development setup
- [x] Add formatting and linting (gofmt/go vet; ESLint + Prettier for the frontend)
- [x] Add unit test and integration test foundations (Go `password_test.go`; Vitest `sync-transform.test.ts`)
- [x] Add CI pipeline (`.github/workflows/ci.yml`: backend + frontend gates)

## Phase 1 — Application Shell

- [x] Create Go backend application
- [x] Create Tauri desktop application configuration
- [x] Create React + TypeScript frontend
- [x] Create shared design tokens
- [x] Add Plus Jakarta Sans font direction
- [x] Implement light theme
- [x] Implement dark theme with `#222323` background
- [x] Add bottom-right theme switcher
- [x] Implement borderless GitBook / Swiss-style app shell
- [x] Install Rust/Cargo toolchain
- [x] Install Tauri CLI
- [x] Verify Tauri Rust project with `cargo check`
- [x] Build macOS `.app` bundle
- [!] Fix macOS `.dmg` packaging (blocked: disk-image bundling step fails; `.app` bundle works)
- [x] Add responsive layout behavior (tablet + phone breakpoints; sidebar folds, grids collapse)
- [x] Add keyboard navigation and accessibility baseline (skip link, `aria-current` nav, labelled status, focus-visible outlines)

## Phase 2 — Database and Platform Core

- [x] Configure PostgreSQL connection foundation
- [x] Add initial migration system folder and schema
- [x] Define organization model
- [x] Define user model
- [x] Define department model
- [x] Define team model
- [x] Define reporting line model
- [x] Define job title and position models
- [x] Define audit log model
- [x] Add audit log API
- [x] Record RBAC and leave decision events
- [x] Record authentication events
- [x] Record attendance events
- [x] Record scheduling events
- [x] Add Activity audit trail view
- [x] Define file and attachment model (generic `files` table; task attachments remain specialized)
- [x] Add health-check endpoint
- [x] Add structured logging (`log/slog` JSON logger + request-logging middleware)
- [x] Add error handling conventions (`internal/httpapi` shared error shape and helpers)
- [x] Add job-title catalogue, user placement, reporting-line, and file-list APIs

## Phase 3 — Authentication and RBAC

- [x] Implement first-run owner setup
- [x] Implement login
- [x] Implement logout and session revocation
- [x] Implement session management
- [x] Implement Argon2id password hashing
- [x] Implement password reset flow (admin reset + self-service change; revokes sessions)
- [x] Add privileged-user MFA foundation (RFC 6238 TOTP: enroll, verify, disable, login enforcement)
- [x] Implement organization membership (org-scoped users)
- [x] Implement department membership (user_departments + primary department)
- [x] Create roles and permissions schema
- [x] Seed owner role and base permissions
- [x] Implement roles and permissions management API
- [x] Add permission catalogue endpoint
- [x] Add custom role creation with permission assignment
- [x] Add user role assignment endpoint
- [x] Add user list/create API foundation
- [x] Add department list/create API foundation
- [x] Add People management screen
- [x] Add user creation form
- [x] Add department creation form
- [x] Implement data scopes: own, department, organization foundation
- [x] Add user-department membership model and assignment endpoint
- [x] Add task scope selector in Work UI
- [x] Implement team scope and scope-aware dashboards (user_teams membership, team task scope, scope-aware dashboard summary)
- [x] Add backend authorization middleware
- [x] Add permission checks for organization, user, and department endpoints
- [x] Add permission-change audit history (role assign/unassign and RBAC events recorded)
- [x] Add offboarding access revocation (status, session revocation, role and MFA removal)
- [x] Add protected route and UI permission handling
- [x] Add role management UI
- [x] Add frontend capability-aware role/permission loading

## Phase 4 — Shared Work Management

- [x] Define task model
- [x] Implement task creation and editing foundation
- [x] Implement assignment and ownership foundation
- [x] Implement task status and priority foundation
- [x] Implement due dates foundation
- [x] Add recurring rule field foundation
- [x] Implement recurring task generator foundation
- [x] Run recurring generation from a background worker
- [x] Add row locking to prevent duplicate generation
- [x] Implement comments and mention-ready text foundation
- [x] Implement secure attachment metadata and local storage upload foundation
- [x] Add secure attachment download endpoint
- [x] Record attachment download audit events
- [x] Add optional ClamAV scan integration and scan-status guard
- [x] Add retention policy configuration with backup verification gate
- [x] Add backup creation and archive verification command
- [x] Add backup checksum manifest
- [x] Add retention cleanup worker gated by verified backup manifest
- [x] Define project model
- [x] Implement project and team views (Projects view; team views pending)
- [x] Add calendar model
- [x] Add scheduling model foundation
- [x] Add shift list/create API
- [x] Add offline shift cache and sync processor
- [x] Add schedule UI
- [x] Add notifications foundation (platform notify service, API, bell UI, workflow emit)

### Attendance foundation

- [x] Define attendance record model
- [x] Add attendance permissions and migration
- [x] Add authenticated check-in and check-out API
- [x] Add offline attendance storage and sync operations
- [x] Add attendance screen with recent history
- [x] Add manager attendance views
- [x] Add attendance correction workflow
- [x] Attendance production depth: hours, validation, today rollup, correction history, remote status
- [x] Company check-in time policy (company-level only) + Early/Late auto-calc
- [x] Attendance desk: name, position, employee ID, check-in / absent + expandable recent table

### Schedule (third production-ready business module)

- [x] Week view + mine/team scope filters
- [x] Assign people (manage), overlap and approved-leave conflict checks
- [x] Confirm / complete / cancel status flows + duration and week hours rollup
- [x] Schedule UI with custom Dropdown / DatePicker / TimePicker
- [ ] Add shifts depth and payroll rules

### Leave (first production-ready business module)

- [x] Leave request create / list / approve / reject
- [x] Offline leave request queue (approvals remain server-authoritative)
- [x] Leave balances (entitled, used, carried, pending, remaining)
- [x] Org leave policies and seed-from-policy entitlements
- [x] Inclusive day count + half-day (single day)
- [x] Overlap prevention for pending/approved ranges
- [x] Balance enforcement on submit and approve
- [x] Cancel pending (self) / cancel approved (manager, restores balance)
- [x] Approval side effects: attendance `leave` days + calendar event
- [x] Optional workflow routing when a `leave` definition exists
- [x] Notifications on approve / reject / manager cancel
- [x] Leave UI filters (all / mine / pending) using custom Dropdown/DatePicker

## Phase 5 — Workflow Engine

- [x] Define workflow definition model
- [x] Define workflow instance model
- [x] Define approval step model
- [x] Support sequential approvals
- [x] Support parallel approvals
- [x] Support role-based approvers
- [x] Support amount-based approval rules
- [x] Support rejection reasons
- [x] Support resubmission
- [x] Support delegated approval
- [x] Add approval reminders and deadlines
- [x] Add complete workflow audit trail
- [x] Add generic approval engine API (definitions, instances, approve/reject/resubmit/cancel)
- [x] Add Approvals UI: reviewer inbox, request submission, workflow builder, and history

## Phase 6 — Offline Capability

- [x] Define local client storage strategy: SQLite for device offline mode
- [x] Define sync operation model: Go-owned sync queue and server validation
- [x] Add local SQLite database integration through Tauri SQL plugin
- [x] Add local session cache
- [x] Add local sync outbox table foundation
- [x] Add sync push endpoint with idempotent operation IDs
- [x] Add sync pull endpoint
- [x] Add local outbox operation helpers
- [x] Add task database schema and permissions
- [x] Add authenticated task list/create API
- [x] Apply offline task create operations on the server
- [x] Add client sync queue processing for pending operations
- [x] Add pending / synced / failed states
- [x] Add retry behavior for failed operations
- [x] Add conflict state and task conflict resolution actions
- [x] Add conflict history and entity-specific merge resolution (sync_conflicts + keep-mine/keep-server resolve)
- [x] Add tombstones for deleted records (task delete → tombstone + delete change-feed op → local drop)
- [x] Define task offline rules (docs/OFFLINE_RULES.md)
- [x] Define calendar offline rules (docs/OFFLINE_RULES.md)
- [x] Define attendance offline rules (docs/OFFLINE_RULES.md)
- [x] Define finance and permission server-authoritative rules (docs/OFFLINE_RULES.md)
- [x] Add offline conflict review UI (Work screen sync conflicts panel)
- [-] Test offline mode without internet access (manual test steps documented; automated device-offline harness pending)

## Phase 7 — Business Modules

### HR

- [x] Employee profiles
- [x] Attendance
- [x] Leave management foundation
- [x] Leave request submission
- [x] Leave approval and rejection foundation
- [x] Add offline leave request cache and sync processor
- [x] Onboarding (per-hire onboarding task checklist)
- [x] Offboarding (access revocation, Phase 3)
- [x] HR documents

### Finance

- [x] Expenses (draft, submit-for-approval, mark paid)
- [x] Invoices (create, status transitions)
- [x] Budgets (allocation with spend derived from paid expenses)
- [x] Purchase requests (draft, workflow approval)
- [x] Vendors
- [x] Finance approvals (routed through the generic workflow engine)
- [x] Finance suite: chart of accounts, journals, tax codes, customers
- [x] AP bills + payments (cash in/out linked to bills/invoices)
- [x] Suite UI tabs (Overview → Purchase)

### Sales and CRM

- [x] Leads (with status pipeline)
- [x] Contacts
- [x] Companies
- [x] Opportunities
- [x] Sales pipeline (opportunity stages)
- [x] Sales activities (CRM activity log)

### IT and Operations

- [x] IT tickets
- [x] Service requests (ticket type)
- [x] Asset registry
- [x] Access requests (ticket type)
- [x] Knowledge base
- [x] Procurement and inventory (purchase requests; inventory pending)

## Phase 8 — Analytics and Executive Control

- [x] Define KPI model (dashboard summary metrics foundation)
- [x] Define dashboard data permissions (own/team/department/organization scope-aware)
- [x] Add department dashboards (open work per department breakdown)
- [x] Add executive dashboard (organization-scope summary)
- [x] Add delayed-work alerts (overdue task metric)
- [x] Add approval bottleneck alerts (approvals-waiting metric)
- [x] Add spending summaries
- [x] Add attendance summaries
- [x] Add sales summaries
- [x] Add custom reports
- [x] Add scheduled reports
- [x] Add export audit trail when reporting/export services are introduced

## Phase 9 — Packaging, Licensing, and Operations

- [ ] Define installer architecture
- [ ] Package Go backend
- [ ] Package Tauri desktop client
- [ ] Package PostgreSQL setup
- [ ] Package local file storage
- [ ] Add backup and restore tools
- [ ] Add license file format
- [ ] Add signed license validation
- [ ] Add update manager
- [ ] Add database migration checks
- [ ] Add health-check and diagnostics bundle
- [ ] Add rollback support
- [ ] Test small-business deployment
- [ ] Test standard-company deployment
- [ ] Test enterprise deployment

## Phase 10 — LukeLang Evaluation Track

LukeLang is Build-first (`luke BUILD` → native/WASM, no GC). The reactive
**Live Graph** tier is the beachhead we would use for reactive dashboards, live
analytics, and realtime notifications. Before writing any LukeLang, read the
LukeLang repo's `AGENTS.md` and copy from its canonical examples rather than
inventing syntax:

- Reactive core: `examples/build/reactive_core.luke` (`REMEMBER`, `THE x IS …`, `BEGIN REACTIVE BATCH`, `CHANGE`)
- Live Graph server: `examples/build/live_graph_server.luke` (`WATCH … FROM db`, `PUSH WATCH … ON req`)
- Live Graph client: `examples/build/live_graph_client.luke` (`WATCH … FROM url`, `BIND`, `WHEN REACTIVE … CHANGES`)
- Backend + DB: `examples/build/backend_api.luke`, `examples/build/pg_api.luke` (`IMPORT std/server`, `std/pg`, `httpServe`, `pgQueryBind`)

LukeLang is compiled and bundled into the product; customers never install it.

- [ ] Define LukeLang integration boundary
- [ ] Add LukeLang language guide for AI-assisted development (point to LukeLang `AGENTS.md` + canonical examples)
- [ ] Build reactive dashboard proof of concept (Live Graph: Postgres CDC → PUSH WATCH → browser paint)
- [ ] Build realtime notification proof of concept
- [ ] Add compiler verification to CI
- [ ] Add cross-platform LukeLang build matrix
- [ ] Benchmark LukeLang against the relevant Go and React workloads
- [ ] Decide which modules, if any, should use LukeLang in production

## Release Readiness

- [ ] Security review
- [ ] RBAC penetration tests
- [ ] Offline and conflict tests
- [ ] Backup and restore verification
- [ ] Database migration rollback verification
- [ ] Performance tests
- [ ] Long-running stability tests
- [ ] Cross-platform installer tests
- [ ] Customer documentation
- [ ] Administrator documentation
- [ ] End-user documentation
- [ ] Support and diagnostics process
