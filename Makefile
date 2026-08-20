# Development commands for the self-hosted business OS.
# Backend: Go. Frontend: React + TypeScript (Vite). Desktop: Tauri.

.PHONY: help backend-run backend-build backend-test backend-vet backend-fmt backend-fmt-check \
        frontend-install frontend-dev frontend-build frontend-test frontend-lint \
        db-up db-down test lint build

help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

## --- Backend (Go) ---
backend-run: ## Run the API server (cd backend && go run ./cmd/server)
	cd backend && go run ./cmd/server

backend-build: ## Build all backend packages
	cd backend && go build ./...

backend-test: ## Run backend tests
	cd backend && go test ./...

backend-vet: ## Static-check backend
	cd backend && go vet ./...

backend-fmt: ## Format backend Go code
	cd backend && gofmt -w .

backend-fmt-check: ## Fail if any Go file is unformatted
	@test -z "$$(cd backend && gofmt -l .)" || (echo "Run 'make backend-fmt'"; cd backend && gofmt -l .; exit 1)

## --- Frontend (React + TypeScript) ---
frontend-install: ## Install frontend dependencies
	cd frontend && npm install

frontend-dev: ## Start the Vite dev server
	cd frontend && npm run dev

frontend-build: ## Type-check and build the frontend
	cd frontend && npm run build

frontend-test: ## Run frontend unit tests
	cd frontend && npm run test

frontend-lint: ## Lint the frontend
	cd frontend && npm run lint

## --- Database (PostgreSQL via Docker) ---
db-up: ## Start the local PostgreSQL development service
	docker compose up -d postgres

db-down: ## Stop the local PostgreSQL development service
	docker compose down

## --- Aggregates ---
build: backend-build frontend-build ## Build backend and frontend

test: backend-test frontend-test ## Run all tests

lint: backend-fmt-check backend-vet frontend-lint ## Run all linters
