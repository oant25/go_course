include .env.example
-include .env
export

OPENAPI_SPEC   := contracts/openapi/trip-service.openapi.yaml
MIGRATIONS_DIR := migrations
GOOSE          := go tool goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)"

.PHONY: generate migrate migrate-down migrate-reset migrate-status run build tidy

generate:
	go tool oapi-codegen -config openapi.yaml $(OPENAPI_SPEC)

migrate:
	$(GOOSE) up

migrate-down:
	$(GOOSE) down

migrate-reset:
	$(GOOSE) reset

migrate-status:
	$(GOOSE) status

run:
	go run ./cmd/trip-service

build:
	go build -o bin/trip-service ./cmd/trip-service

tidy:
	go mod tidy