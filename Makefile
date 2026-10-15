.PHONY: all help up down logs ps build test migrate-up migrate-down

help:
	@echo "Galeriku Management Makefile"
	@echo "  make up          - Start all containers via docker-compose"
	@echo "  make down        - Stop all containers"
	@echo "  make logs        - Tail container logs"
	@echo "  make build       - Rebuild backend and ai-worker images"
	@echo "  make migrate-up  - Run initial SQL migration 000001"
	@echo "  make migrate-down- Revert initial SQL migration 000001"
	@echo "  make test        - Run backend Go tests"

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

build:
	docker compose build

test:
	cd backend && go test ./... -v
