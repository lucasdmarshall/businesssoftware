# Octopilot Business OS — Organization and RBAC Model

## Organization Hierarchy

```text
Organization
 ├── Business Unit
 ├── Department
 │    ├── Team
 │    │    └── User
 │    └── Department Dashboard
 ├── Branch / Region
 └── Organization-wide Administration
```

## Separate These Concepts

The following must not be merged into one field:

- Job title: what a person is called professionally
- Position: where the person sits in the organization
- Department: which business area they belong to
- Reporting manager: who they report to
- Permission role: what they can do in the software
- Data scope: which records they can access

For example, “Finance Manager” is a job title. “Invoice Approver” is a permission role. A Finance Manager may or may not be an invoice approver.

## Permission Dimensions

Permissions should support:

- View
- Create
- Edit
- Delete
- Submit
- Approve
- Reject
- Export
- Assign
- Manage settings
- Manage users
- Manage permissions

Each permission should be evaluated with a scope:

- Own records
- Own team
- Own department
- Assigned departments
- Business unit
- Branch or region
- Entire organization

## Access Levels

Common access levels include:

- Employee
- Team Lead
- Manager
- Department Head
- Executive
- Owner / Administrator

These are defaults only. Organizations should be able to create custom roles.

## Security Requirements

- Backend authorization on every protected operation
- Deny-by-default permissions
- Explicit cross-department access
- Permission-change audit history
- Immediate access revocation on offboarding
- MFA for privileged users
- Separation of duties for sensitive approvals
- No automatic permission inheritance solely because of a senior job title

The full corporate role title list is documented separately in `corporate-role-hierarchy.md`.
