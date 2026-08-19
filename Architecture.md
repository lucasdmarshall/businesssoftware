# Octopilot Business OS — Architecture Decision

## Status

**Accepted**

## Product Distribution Decision

Octopilot Business OS is not a cloud-only SaaS or multi-tenant web application.

It is a **self-hosted, installable, offline-capable enterprise business software suite**. The complete software package will be sold and installed for each customer company on its own server, office network, or private infrastructure.

Each customer receives an independent installation and owns or controls its own operational data.

## Core Architecture Decision

The product will use:

> **Modular Monolith + Background Workers + Packaged Deployment**

The internal codebase will be divided into strong business-domain modules, but the initial product will be deployed as a manageable application package rather than a collection of independently operated microservices.

The architecture must remain microservice-ready so that selected modules can be extracted later when scale, security, or enterprise deployment requirements justify it.

## Why We Are Not Starting with Microservices

Microservices are not required merely because the product is large or may have high traffic. For self-hosted software, too many independent services increase the customer's operational burden.

Starting with microservices would create:

- More containers and services to install
- More configuration and network ports
- More upgrade and rollback complexity
- More distributed failure modes
- More difficult database consistency
- Higher support and deployment costs
- Greater dependency on the customer's IT team

The first product must be powerful for the customer but simple to install, backup, update, monitor, and support.

## Deployment Modes

### Small Business Mode

One server or workstation runs the application, database, storage, and workers.

```text
Application + Database + Storage + Worker
```

### Standard Company Mode

A local company server runs the platform. Users connect through the office network using an installed client or local application interface.

```text
Users -> Office LAN -> Company Server
```

### Enterprise Mode

Large customers may separate application servers, workers, database, and file storage.

```text
Load Balancer
 ├── Application Server 1
 ├── Application Server 2
 ├── Background Worker Server
 ├── PostgreSQL Cluster
 └── File Storage
```

The same product and codebase should support all three modes through deployment profiles.

## Logical Architecture

```text
Desktop / Mobile / Local Client Applications
                    |
              Local or Private API
                    |
             Modular Application Core
 ┌────────┬────────┬────────┬────────┬────────┐
 Auth   Organization  RBAC   Workflow  Audit
 Tasks  Attendance   HR     Finance   Sales
 IT     Calendar     Files  Analytics Notifications
                    |
       PostgreSQL + Local File Storage
                    |
              Background Workers
```

## Platform Modules

### Identity and Access

- Authentication
- User and employee accounts
- Sessions and devices
- MFA foundation
- Organization membership
- Roles and permissions
- Department and team access scopes
- License validation

### Organization Core

- Departments
- Teams
- Business units
- Branches and regions
- Job titles and positions
- Reporting lines
- Custom groups
- Employee status

### Shared Platform Services

- Workflow and approvals
- Notifications
- Files and documents
- Search
- Audit logs
- Activity timeline
- Custom fields
- Import and export
- Scheduled jobs

### Business Domains

- Tasks and projects
- Attendance and leave
- HR operations
- Finance operations
- Sales and CRM
- IT service management
- Calendar and scheduling
- Procurement and inventory
- Analytics and executive reporting

Each module must have its own data models, business rules, screens, permissions, audit events, and offline rules.

## Data Architecture

PostgreSQL will be the authoritative primary transactional database for a standard installation. It will be installed on the customer's local server or private infrastructure.

PostgreSQL does not require public internet access. If the client can reach the local PostgreSQL server through the office LAN or localhost, the company can continue operating without internet.

When a device cannot reach the company server at all, the client will use a local SQLite store and synchronize through the Go sync layer when connectivity returns.

The database should maintain clear domain boundaries while allowing modules to share trusted platform entities such as users, organizations, departments, and files.

Core records include:

```text
organizations
users
departments
teams
positions
roles
permissions
user_roles
reporting_lines
audit_logs
notifications
files
workflow_instances
```

All company-owned records must be associated with the customer's organization and protected by backend authorization.

## Online and Offline Operation

The product supports two offline situations.

### Office Offline Mode

The public internet is unavailable, but the customer's local server and office LAN are available.

```text
Tauri Client -> Office LAN -> Go Backend -> Local PostgreSQL
```

PostgreSQL remains active in this mode and normal company operations can continue.

### Device Offline Mode

The device cannot reach the office server or local network.

```text
Tauri Client -> Local SQLite -> Offline Queue
                                      |
                         Connectivity returns
                                      |
                           Go Sync Service
                                      |
                          Local PostgreSQL
```

PostgreSQL is the source of truth. SQLite is only the local offline store. The Go sync layer owns server validation, idempotency, retries, ordering, and conflict resolution.

Offline-capable features may include:

- Tasks and comments
- Notes
- Calendar events
- Attendance capture
- Field forms
- Draft requests
- Previously downloaded records

Security-critical actions remain server-authoritative and may require local server validation:

- Permission changes
- Final finance approvals
- Payroll confirmation
- Sensitive employee changes
- User deletion
- Company-wide destructive actions

## Background Workers

Long-running or scheduled work must run outside the main request process.

Workers will handle:

- Email and notifications
- PDF and spreadsheet reports
- Data imports and exports
- Scheduled jobs
- Analytics aggregation
- Offline synchronization
- File processing
- Webhooks and integrations
- Backup tasks

## Security Model

Access must be enforced at the backend.

```text
User
 -> Organization
 -> Department / Team
 -> Permission Role
 -> Data Scope
 -> Allowed Action
```

Required controls include:

- Deny-by-default authorization
- Explicit cross-department access
- MFA for privileged users
- Permission-change audit history
- Immediate offboarding and access revocation
- Separation of duties for sensitive approvals
- Secure local storage and encrypted backups
- Signed updates and license files

Job title seniority must not automatically grant unrestricted system permissions.

## Licensing and Commercial Distribution

The software may support:

- Per-company licensing
- Per-server licensing
- Per-user licensing
- Module-based licensing
- Annual maintenance and support
- Enterprise editions
- Offline license files

License files should be digitally signed and contain the company identity, edition, enabled modules, user limits, and validity rules.

The software should not become unusable merely because an external license server is temporarily unreachable. Graceful offline operation is a product requirement.

## Updates and Maintenance

Every installation must support safe updates:

```text
Create Backup
    -> Check Compatibility
    -> Run Database Migration
    -> Install Update
    -> Health Check
    -> Roll Back if Required
```

Release types:

- Stable releases
- Security patches
- Feature releases
- Database migrations
- Offline update bundles

Updates should be signed, versioned, reversible where possible, and compatible with the customer's deployment profile.

## Future Microservice Extraction

Microservices may be introduced when a specific boundary needs independent scaling, isolation, or deployment.

Potential extraction candidates:

1. Notification service
2. Reporting and analytics service
3. File and document processing service
4. Search service
5. Offline synchronization service
6. Finance service for strict enterprise isolation

Extraction must be justified by measurable requirements, not by architectural fashion.

## Final Decision

Octopilot Business OS will be built as a **self-hosted, installable, offline-capable, modular enterprise software suite**.

The initial technical approach is:

```text
Modular Monolith
 + Background Workers
 + PostgreSQL
 + Local File Storage
 + Packaged Deployment
 + Offline-Capable Client
 + Microservice-Ready Domain Boundaries
```

This approach balances product power, customer data ownership, offline operation, deployment simplicity, performance, and future scalability.
