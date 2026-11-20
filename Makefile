ifeq (,$(wildcard .env))
$(error .env not found: copy .env.example to .env)
endif
include .env
export

# The migrate CLI is run at a pinned version instead of being added to go.mod,
# which keeps its database drivers out of the module's dependency graph.
MIGRATE := go run -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@v4.20.1

.PHONY: up down migrate run-api run-worker

up:
	docker compose up --detach --wait

down:
	docker compose down

migrate:
	$(MIGRATE) -path migrations -database "$(CURSUS_DATABASE_URL)" up

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker
