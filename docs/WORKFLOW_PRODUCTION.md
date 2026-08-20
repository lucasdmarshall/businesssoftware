# Workflow production depth

Approvals run through the generic workflow engine (`014_workflow.sql` +
`035_workflow_delegation_reminders.sql`).

## Deadlines (SLA)

- Each workflow step may set `sla_hours`.
- When an instance enters that step, `due_at` is set to now + SLA.
- Submitters / workflow managers can override via
  `POST /api/v1/workflow/instances/{id}/due`.

## Reminders

A background worker (started with the API server) polls every minute:

| Kind | When |
|---|---|
| `upcoming` | Due within 24 hours |
| `overdue` | Past due |

Recipients are the step’s named user / role members **plus** active delegates.
Notifications use kinds `workflow.upcoming` / `workflow.overdue`. Reminders
repeat at most every 12 hours per instance.

## Delegated approval (coverage)

`POST /api/v1/workflow/delegations` creates a time-boxed coverage window:
delegate may act on steps assigned to the delegator (user or role). Actions
record `on_behalf_of`. Revoke with
`POST /api/v1/workflow/delegations/{id}/revoke`.

## Linked modules

Leave / expenses / purchase requests still `Start` the engine when a matching
definition exists. While a leave request’s workflow is `in_review`, Leave
`Decide` is blocked — use Approvals. Terminal workflow decisions sync leave
status and apply production side effects (balance, attendance, calendar).
