# Offline Rules

How the product behaves without connectivity, and which actions must stay
server-authoritative. PostgreSQL is always the source of truth; SQLite is only a
local cache and outbox for device-offline mode.

## Two offline situations

- **Office offline** — the public internet is down but the company server and
  LAN are reachable. Everything works normally against PostgreSQL.
- **Device offline** — the device cannot reach the company server. Supported
  actions are written to local SQLite and queued in the outbox, then synced
  through the Go server when connectivity returns.

## Sync model

- Every queued mutation has a unique operation id and is idempotent on the
  server (duplicate ids are ignored).
- The client shows each operation as pending, synced, failed, or conflict.
- On reconnect the client pushes the outbox, then pulls the change feed and
  applies it to the local caches. Deletions arrive as tombstones and drop the
  local copy.
- Server authorization is re-checked during sync; being offline never bypasses
  permission or approval checks.

## Entity offline rules

### Tasks

- Offline-capable: create, and (where wired) update and delete.
- Reads: the last pulled set stays available offline.
- Conflict rule: a create whose id already exists on the server is recorded as a
  conflict for review rather than silently overwritten. Resolution is
  entity-specific — "keep mine" applies the client fields, "keep server" keeps
  the server row.
- Deletions propagate through a tombstone in the change feed.

### Calendar

- Offline-capable: creating events (queued) is acceptable.
- Conflict rule: overlapping events for the same person are flagged as a
  time conflict on create; they are not blocked, so the person can decide.

### Attendance

- Offline-capable: check-in and check-out capture.
- Conflict rule: check-in and check-out for the same day apply to separate
  columns so the two operations never clobber each other. Cross-device sync
  merges on `(organization, user, work_date)` rather than inventing a second
  day record. Manager corrections remain server-side and append to
  `attendance_corrections` history.

### Leave and shifts

- Offline-capable: submitting a request or a shift entry (queued).
- Approvals and rejections are **not** offline actions (see below).
- Leave balance updates happen only when the server approves a request.
- Shift status changes (confirm / complete / cancel) are server-side.
- Payroll rules and hour summaries are server-authoritative reads/writes.

## Server-authoritative actions (never offline)

These require the company server and are never applied from the local queue:

- Permission and role changes
- Final finance approvals and payroll confirmation
- Payroll rule changes and hour summary computation
- Leave balance entitlements and approval-driven balance deductions
- Attendance corrections (and their history)
- Leave and workflow **approval / rejection** decisions
- User deletion, offboarding, and access revocation
- MFA enable/disable and password changes
- Any company-wide destructive action

Rationale: these are security- or money-critical, so PostgreSQL must validate
and record them with the actor's identity. Offline mode must never become a way
to bypass authorization or approval controls.

## Conflict resolution

- Non-sensitive text: the latest valid update may win.
- Tasks: merge where possible; otherwise record a review conflict.
- Calendar: detect time conflicts and surface them.
- Attendance: preserve both events; require review when necessary.
- Finance: never silently overwrite — always create a review conflict.
- Permissions: the server version always wins.

Open conflicts appear in the Work screen's "Sync conflicts" panel with
keep-mine / keep-server actions.

## Manual offline test

Until an automated device-offline harness exists, verify manually:

1. Sign in while online so the session and caches populate.
2. Stop the backend (or disconnect) to simulate device-offline mode.
3. Create a task and check in for attendance — both should save locally and show
   as pending.
4. Confirm the screens still render from the local cache.
5. Restart the backend; confirm the queued operations sync (pending → synced)
   and that a create colliding with an existing id surfaces in the conflict
   panel.
6. Delete a task on another client; confirm this device drops it after the next
   pull.
