# Octopilot Business OS — System Architecture

## Recommended Starting Architecture

Start with a modular monolith rather than independent microservices. The platform should have clear domain boundaries internally so that high-volume modules can later be extracted into services.

```text
Web / Desktop / Mobile Clients
              |
        API and Auth Layer
              |
       Modular Application Core
  Auth | Org | RBAC | Workflow | Audit
  Tasks | HR | Finance | Sales | IT
              |
 PostgreSQL | Redis | Object Storage
              |
     Sync Queue and Background Jobs
```

## Platform Layers

### Client Layer

- Web application
- Desktop application
- Mobile application
- Offline local database
- Local sync queue
- Notifications and background synchronization

### Access Layer

- Authentication
- Session management
- MFA
- Organization resolution
- Permission evaluation
- API rate limiting
- Device and session security

### Platform Core

- Organizations and tenants
- Users and memberships
- Departments and teams
- Reporting lines
- Roles and permissions
- Custom fields
- Files and documents
- Notifications
- Search
- Audit logs
- Approval engine
- Activity timeline

### Business Domains

Each domain should own its data, rules, screens, and permissions while using shared platform services.

### Data and Infrastructure

- PostgreSQL as the primary transactional database
- Redis for caching, sessions, queues, and short-lived state
- Object storage for documents and attachments
- Background workers for reports, notifications, imports, and synchronization
- Centralized logs and monitoring
- Automated backups and disaster recovery

## Tenant Isolation

Every business record should be scoped to an organization. Department and team scope should be enforced through backend authorization and, where practical, database row-level security.

The minimum security context for a request is:

```text
User -> Organization -> Department/Team -> Role -> Permission -> Record Scope -> Action
```

## Data Ownership Rules

- The organization owns company records.
- Departments own department-operational records.
- Teams own team records.
- Users may own personal drafts and assigned work.
- Sensitive records require explicit permission and audit logging.

## Integration Strategy

The platform should expose stable APIs and webhooks for:

- Identity providers
- Email and messaging
- Accounting and payment services
- Payroll providers
- File storage
- Calendar providers
- Business intelligence tools

External integrations should extend the platform without becoming the only source of truth for core organization and permission data.
