# Octopilot Business OS — Online and Offline Sync

## Database and Offline Model

PostgreSQL is the authoritative company database and is installed on the customer's local server or private infrastructure. It can operate without public internet access when the client can reach it through the office LAN or localhost.

True device offline mode is separate. When a client cannot reach the company server, it uses a local SQLite store and queues changes until connectivity returns.

```text
Office offline:
Tauri Client -> Office LAN -> Go Backend -> PostgreSQL

Device offline:
Tauri Client -> SQLite -> Offline Queue -> Go Sync -> PostgreSQL
```

## Local-First Model

In device offline mode, the client writes supported user actions to SQLite first. The interface updates immediately, then a synchronization queue sends changes to the Go backend when the company server becomes reachable.

```text
User Action
   -> Local SQLite Database
   -> Sync Queue
   -> Go Server Validation
   -> PostgreSQL
   -> Change Feed Back to Clients
```

## Suitable Offline Features

- Tasks and task comments
- Notes
- Calendar events
- Basic attendance capture
- Field forms
- Previously downloaded records
- Draft requests
- Limited document access

## Online-Required or Server-Authoritative Features

- Final finance approvals
- Payroll confirmation
- Permission changes
- User deletion or access revocation
- Sensitive employee data changes
- Final invoice approval
- Company-wide destructive actions

## Synchronization Requirements

- Every mutation has a unique operation ID.
- Operations are idempotent and safe to retry.
- The client displays pending, synced, failed, and conflict states.
- Sync failures are retained for retry and investigation.
- Server authorization is rechecked during synchronization.
- Deleted records use tombstones so offline clients can receive deletions.
- PostgreSQL remains authoritative when local and server state disagree.
- The Go sync layer handles idempotency, ordering, retries, and conflict records.

## Conflict Rules

- Non-sensitive text: latest valid update may win.
- Tasks: merge fields where possible.
- Calendar: detect time conflicts.
- Attendance: preserve both events and require review when necessary.
- Finance: never silently overwrite; create a review conflict.
- Permissions: server version always wins.

Offline mode must not become a way to bypass authorization or approval controls.
