.PHONY: all help up down logs ps build test test-backend test-web test-integration typecheck lint-copy migrate-up migrate-down

help:
	@echo "Galeriku Management Makefile"
	@echo "  make up          - Start all containers via docker-compose"
	@echo "  make down        - Stop all containers"
	@echo "  make logs        - Tail container logs"
	@echo "  make build       - Rebuild backend and ai-worker images"
	@echo "  make migrate-up  - Run initial SQL migration 000001"
	@echo "  make migrate-down- Revert initial SQL migration 000001"
	@echo "  make test-backend- Run backend Go tests (short mode)"
	@echo "  make test-web    - Run web TypeScript/Vitest tests"
	@echo "  make typecheck   - Run web TypeScript type checking"
	@echo "  make lint-copy   - Run antislop copywriting em-dash linter"
	@echo "  make test        - Run backend, web, typecheck, and lint-copy tests"
	@echo "  make test-integration - Run full backend integration tests with Docker"

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

build:
	docker compose build

test-backend:
	cd backend && go test -v -short ./...

test-web:
	cd web && npm test

typecheck:
	cd web && npm run typecheck

lint-copy:
	cd web && npm run lint:copy

test: test-backend test-web typecheck lint-copy

test-integration:
	cd backend && go test -v ./...
