# Name

Self-hosted, offline-capable enterprise business software.

## Workspace layout

```text
backend/       Go API and business services
frontend/      React + TypeScript application shell
desktop/       Tauri desktop packaging configuration
```

## Development commands

Backend:

```bash
cd backend
go run ./cmd/server
```

Frontend:

```bash
cd frontend
npm install
npm run dev
```

PostgreSQL development service:

```bash
cp .env.example .env
docker compose up -d postgres
```

The basic backend health check does not require a live database connection. The initial core schema is in `backend/migrations/001_initial.sql` and is mounted automatically by the development compose setup.

The first milestone intentionally keeps PostgreSQL configuration ready without requiring a live database connection for the health-check screen.
