# Development Guide

Engineering commands, environment conventions, and quality gates for the
self-hosted business OS.

## Prerequisites

- Go (1.24+) for the backend
- Node.js (20+) and npm for the frontend
- PostgreSQL 14+ (Docker Compose service provided) for a live database
- Rust stable + Tauri CLI for native desktop builds (optional during API/UI work)

## Commands

All commands are available through the root `Makefile` (`make help` lists them).
The underlying commands:

| Task | Command |
| --- | --- |
| Run backend API | `cd backend && go run ./cmd/server` |
| Build backend | `cd backend && go build ./...` |
| Test backend | `cd backend && go test ./...` |
| Vet backend | `cd backend && go vet ./...` |
| Format backend | `cd backend && gofmt -w .` |
| Install frontend deps | `cd frontend && npm install` |
| Frontend dev server | `cd frontend && npm run dev` |
| Build frontend | `cd frontend && npm run build` |
| Test frontend | `cd frontend && npm run test` |
| Lint frontend | `cd frontend && npm run lint` |
| Start PostgreSQL | `docker compose up -d postgres` |

Aggregates: `make build`, `make test`, `make lint`.

## Environment variables

Copy `.env.example` to `.env` for local development. The backend reads these at
startup (see `backend/internal/config/config.go`):

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `8080` | HTTP port for the API server |
| `DATABASE_URL` | _(empty)_ | PostgreSQL connection string; when empty the server runs the health check without a live database |
| `STORAGE_PATH` | `data/uploads` | Private local directory for task attachment storage |
| `CLAMDSCAN_PATH` | _(empty)_ | Path to `clamdscan`; when empty, uploads stay `unscanned` and downloads are gated to `clean` |
| `ATTACHMENT_RETENTION_DAYS` | `365` | Attachment retention window |
| `BACKUP_VERIFIED` | `false` | Gate that keeps retention cleanup disabled until backups are verified |
| `BACKUP_PATH` | `data/backups` | Destination for backup archives and checksum manifests |

Compose-only variables (`POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`,
`POSTGRES_PORT`) configure the local PostgreSQL container.

### Conventions

- Read configuration through `config.FromEnvironment()`; do not call `os.Getenv`
  scattered across handlers.
- Every variable must have a safe default or degrade gracefully when unset — the
  product must stay usable offline and without optional services.
- Never commit real secrets. `.env` is git-ignored; `.env.example` documents the
  shape with placeholder values only.

## Quality gates

Before landing a change, run the gates that match what you touched (CI runs all
of them):

- Backend: `gofmt -l .` (must be empty), `go vet ./...`, `go test ./...`, `go build ./...`
- Frontend: `npm run lint`, `npm run test`, `npm run build`

## Backend conventions

### Error responses

New handlers use `internal/httpapi` for a single error shape:

```json
{ "error": { "code": "not_found", "message": "request not found" } }
```

- `httpapi.WriteJSON(w, status, payload)` for success bodies.
- `httpapi.WriteError(w, status, code, message)` for failures — `code` is a
  stable machine-readable slug, `message` is human-readable.
- Never leak internal error strings to clients; log the detail server-side and
  return a safe message.

Earlier handlers use a local `writeJSON` with a `{"error": "..."}` shape; migrate
them to `httpapi` opportunistically rather than in one large change.

### Structured logging

- The default logger is JSON via `log/slog` (`httpapi.NewLogger`, set in
  `main.go`). Use `slog.Info/Warn/Error` with key/value pairs, not `log.Printf`.
- `httpapi.WithRequestLogging` logs one line per request (method, path, status,
  duration). It never logs bodies or query values, which may hold sensitive data.
- Set `LOG_LEVEL=debug` for verbose logging.

## LukeLang (reactive engine)

LukeLang is an owner-maintained strategic technology used for selected reactive
UI and live-data modules; it is compiled and bundled so customers never install
it. Before writing any LukeLang code, read the LukeLang repository's `AGENTS.md`
playbook and build from its canonical examples rather than inventing syntax:
<https://github.com/lucasdmarshall/LukeLang>.
