include .env.example
-include .env
export

OPENAPI_SPEC   := contracts/openapi/trip-service.openapi.yaml
MIGRATIONS_DIR := migrations
GOOSE          := go tool goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)"

.PHONY: generate generate-openapi generate-mocks migrate migrate-down migrate-reset migrate-status run build tidy

generate: generate-openapi generate-mocks

generate-mocks:
	go generate ./internal/usecase/... ./internal/infra/postgres/transaction/... ./internal/infra/telemetry/... ./internal/http/...

generate-openapi:
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

GOLANGCI_LINT ?= golangci-lint
COVERAGE_DIR := .coverage

.PHONY: test test-unit test-integration lint fmt
test:
	$(MAKE) test-unit
	$(MAKE) test-integration

test-unit:
	mkdir -p $(COVERAGE_DIR)
	go test -race -count=2 -covermode=atomic -coverprofile=$(COVERAGE_DIR)/unit.out ./...
	go tool cover -func=$(COVERAGE_DIR)/unit.out

test-integration:
	@test -n "$${DATABASE_URL:-}" || { echo 'Для интеграционных тестов требуется DATABASE_URL' >&2; exit 1; }
	go test -race -count=2 -tags=integration ./internal/repository/...

lint:
	$(GOLANGCI_LINT) run
	@set -eu; diff_file=$$(mktemp); trap 'rm -f "$$diff_file"' EXIT HUP INT TERM; \
	$(GOLANGCI_LINT) fmt --diff > "$$diff_file"; \
	cat "$$diff_file"; \
	if test -s "$$diff_file"; then echo 'Форматирование отличается; выполните make fmt' >&2; exit 1; fi

fmt:
	$(GOLANGCI_LINT) fmt
