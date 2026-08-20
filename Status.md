# Development Status

## Current State

**Status:** Milestone 0 in progress; runnable foundation created

**Last updated:** 2026-08-20

## Product Definition

This is an installable, self-hosted, offline-capable enterprise business software suite. It is sold as a complete software product and installed independently for each customer company. It is not a cloud-only SaaS or shared multi-tenant web application.

The product name is intentionally undecided. Use **Name** in development documents and interfaces until branding is defined.

## Confirmed Architecture

```text
Desktop Client: Tauri
Frontend: React + TypeScript
Backend: Go
Database: PostgreSQL
Deployment: Packaged self-hosted installation
Architecture: Modular monolith + background workers
Future: Microservice extraction only when justified
```

## Confirmed Design System

- Primary font: Plus Jakarta Sans
- Themes: Light and dark
- Dark background: `#222323`
- Light background: warm white, not stark pure white
- Theme switcher: docked at the bottom-right corner
- Interaction approach: Material Design patterns
- Visual style: GitBook-inspired and Swiss-style
- Default layout: borderless, cardless, boxless, clean
- Cards and boxes: only where necessary; may appear subtly on hover or focus

Reference: [Language and Design System](./Language-and-Design-System.md)

## Confirmed Product Principles

- The company should be able to run its operations through the software.
- Department dashboards and data must be protected by backend-enforced RBAC.
- Job titles and permissions are separate concepts.
- Important actions must be auditable.
- Approved work must continue without internet access.
- Office offline mode uses local PostgreSQL through the company LAN.
- Device offline mode uses local SQLite, an offline queue, and Go synchronization.
- PostgreSQL remains the authoritative company database.
- Finance, permissions, and other security-critical actions remain server-authoritative.
- The customer should not need to install or maintain LukeLang.

## Current Milestone

### Milestone 0 — Engineering Foundation

Goal: create a working repository, development environment, application shell, and verified build pipeline.

Completed in this milestone:

- Go backend health endpoint at `/health` and `/api/v1/health`
- React + TypeScript frontend shell
- Tauri desktop configuration
- Plus Jakarta Sans design direction
- Light and dark themes
- Dark background token `#222323`
- Bottom-right theme switcher
- Borderless GitBook / Swiss-style initial layout
- Go tests and frontend production build
- Rust stable toolchain installed and Tauri `cargo check` passing
- Tauri CLI installed and macOS `.app` bundle built successfully
- PostgreSQL Docker Compose configuration and initial core migration added
- Local PostgreSQL 14 development database created and initial migration applied
- PostgreSQL-backed organization list/create API added
- Frontend now displays backend and database connection state
- Local organization smoke test completed with `Acme Workspace`
- First-run owner setup flow added
- Argon2id password hashing and database-backed session cookies added
- Owner role and base permissions seeded during setup
- Login and current-user (`/auth/me`) flow verified end-to-end
- Auth-aware frontend setup/login screens added
- Backend session authorization middleware added
- Logout and session revocation added
- User and department list/create API foundations added
- Protected organization, user, and department routes verified
- People management screen added to the authenticated frontend
- User and department list/create flows connected to the API
- Tauri SQLite plugin installed and enabled
- Local SQLite database initialization added
- Local session cache and sync outbox foundation added
- Frontend can fall back to a cached session when the backend is unreachable
- Native macOS `.app` rebuilt with SQLite support
- Sync operations migration added
- Go sync push/pull endpoints added
- Local outbox operation helpers added
- Sync smoke test passed with an offline task operation
- Task module schema and permission seeds added
- Task list/create API added
- Offline task create operation now applies to PostgreSQL
- Task management screen added with online create and offline local create
- Offline task creates are queued in SQLite and sync automatically when the backend is reachable
- Sync outbox now records failed operations and exposes a task-level Retry action
- Task sync conflicts are classified separately and expose Retry / Discard actions
- Attendance foundation added: PostgreSQL records, permissions, check-in/check-out API, SQLite cache, offline sync, and frontend screen
- Manager attendance endpoint and permission-protected correction workflow added
- Attendance UI now shows the manager team view and correction form when the user has `attendance.manage`
- Leave management foundation added: request schema, permissions, list/create API, approve/reject API, and Leave UI
- Leave requests now work offline through SQLite cache and the sync outbox; approval and rejection remain server-authoritative
- Local PostgreSQL migration `006_leave.sql` applied successfully
- Scheduling foundation added: shift schema, permissions, list/create API, offline cache/sync, and Schedule UI
- Local PostgreSQL migration `007_shifts.sql` applied successfully
- RBAC management API added: permission catalogue, organization-scoped role list/create, and user role assignment
- RBAC management UI added to People: role list, custom role creation, permission selection, and user role assignment
- `/auth/me` now returns effective permissions and the frontend filters module navigation by capability
- Offline cached sessions now retain permissions for safe local UI gating
- Audit trail foundation added with PostgreSQL audit logs, API, RBAC/leave event recording, and Activity view
- Audit coverage expanded to setup, login, logout, attendance check-in/out, and shift creation
- Task module expanded with assignment, due dates, PATCH update API, offline payload support, and Work UI controls
- Local PostgreSQL migration `009_task_assignment.sql` applied successfully
- Task collaboration foundation added: comments API/UI, mention-ready comment text, and recurrence rule storage
- Local PostgreSQL migration `010_task_collaboration.sql` applied successfully
- Secure task attachment upload foundation added with private local storage, randomized keys, 10 MB limit, and organization/task authorization
- Recurring task generator foundation added for daily, weekly, and monthly rules with duplicate-safe chain advancement
- Local PostgreSQL migration `011_recurring_tasks.sql` applied successfully
- Backend recurring-task worker now runs every minute while the local server is online and uses `FOR UPDATE SKIP LOCKED`
- Attachment download endpoint added with task/org authorization, path traversal protection, safe filename headers, and audit logging
- Optional ClamAV integration added; attachments remain `unscanned` when no scanner is configured and downloads require `clean` status
- Attachment retention defaults to 365 days and is configuration-ready; cleanup remains disabled until backup verification is automated
- Local PostgreSQL migration `012_attachment_security.sql` applied successfully
- Data scope foundation added: user-department memberships, own/department/organization task filtering, and Work scope selector
- Local PostgreSQL migration `013_memberships_and_scopes.sql` applied successfully
- Backup verification command added: creates a custom PostgreSQL dump, verifies it with `pg_restore --list`, and writes a SHA-256 manifest
- Local PostgreSQL migration `008_audit_logs.sql` applied successfully
- Frontend production build, Go tests, and Tauri Rust check pass after attendance changes
- Frontend production build and Tauri Rust check pass after task management changes
- Generic approval workflow engine added: definitions, ordered steps, instances, and an append-only action history (migration `014_workflow.sql`)
- Workflow engine supports sequential steps, parallel steps (per-step required approvals), role- or user-based approvers, amount-band step rules, rejection with reason, resubmission, and cancellation; approvals remain server-authoritative
- Workflow API added: definition list/create, instance list (reviewer inbox / mine filters), instance detail with history, and approve/reject/resubmit/cancel actions
- Approvals UI added: reviewer inbox, request submission, workflow builder, and per-request approval history
- Fixed CORS `Access-Control-Allow-Methods` to include POST and PATCH so browser-preflighted mutations succeed
- Declared the `@tauri-apps/plugin-sql` frontend dependency so the production build type-checks and offline SQLite bindings resolve
- Bundled Plus Jakarta Sans locally via `@fontsource` instead of the Google Fonts CDN so typography survives a fully offline installation
- Reworked responsive layout: removed the `min-width: 1080px` contradiction and added tablet (1080px) and phone (720px) breakpoints that stack the sidebar and collapse every multi-column grid
- Frontend now consumes the `sync/pull` change feed (`pullRemoteChanges`) after login and workspace load, writing tasks, leave, shifts, and attendance into the local SQLite caches so a device keeps other clients' synced changes when it later goes offline
- Phase 0 engineering foundation completed: root `Makefile` and `docs/DEVELOPMENT.md` (commands + environment conventions), gofmt/go vet plus ESLint + Prettier for the frontend, a Go test foundation (`password_test.go`) and a Vitest test foundation (`sync-transform.test.ts`), and a GitHub Actions CI pipeline running both stacks' gates
- Phase 1 shell finished except blocked `.dmg` packaging: responsive tablet/phone layout and an accessibility baseline (skip link, `aria-current` navigation, labelled connection status, focus-visible outlines)

Next actions:

1. Add team scope, scope-aware dashboards, and department data policies.
2. Add shifts, leave balance, correction history, and payroll rules.
3. Add conflict history and entity-specific merge resolution.
4. Add export audit hooks when reporting/export services are introduced.
5. Wire existing modules (leave, future finance) through the generic workflow engine instead of module-local approve/reject.
6. Add workflow approval reminders/deadlines and delegated approval.
7. Add delete tombstones so device-offline clients also receive deletions through the pull feed (pull now applies creates/updates only).
8. Add a cross-device attendance merge keyed on work date (pull currently upserts attendance by record id).

## Active Work

- [x] Source code repository structure
- [x] Go backend bootstrap
- [x] Tauri client configuration bootstrap
- [x] React + TypeScript frontend bootstrap
- [x] PostgreSQL local development setup (Homebrew PostgreSQL 14)
- [x] Rust/Cargo toolchain for native Tauri build
- [x] Confirm online/offline database strategy
- [x] PostgreSQL-backed organization API
- [x] Backend/database connection state in frontend

## Blockers

Docker is not currently available on the development machine, so the local setup currently uses Homebrew PostgreSQL 14. The Docker Compose setup remains available for repeatable environments. The native macOS `.app` bundle is working. DMG packaging still fails in the macOS disk-image bundling step and is tracked separately.

## Confirmed Release Targets

- Supported desktop operating systems for the first release: **Windows and macOS**
- Installer formats: Windows `.msi` (WiX) and `.exe` (NSIS, per-machine); macOS `.app` and `.dmg`
- Desktop packaging: Tauri `bundle.targets: "all"`, built per-OS via the `release.yml` matrix (`windows-latest`, `macos-latest`)

## Important Decisions Pending

- Minimum supported PostgreSQL version
- Code signing / notarization identities for Windows and macOS distribution
- Local development container strategy
- Office offline: local PostgreSQL over LAN
- Device offline: SQLite cache plus Go sync layer
- Authentication protocol and password policy
- First production module after platform foundation
- Exact warm-white color token
- Whether LukeLang is used in the first production release or after a dedicated proof of concept

## Tracking Rules

- Update `Tasklist.md` whenever a task starts or finishes.
- Update this file after every meaningful architecture or implementation decision.
- Keep completed decisions stable unless a new decision is explicitly recorded.
- Record blockers with enough detail for another developer or AI agent to resume work.
- Every implementation change must be verified with tests or a documented manual check.
