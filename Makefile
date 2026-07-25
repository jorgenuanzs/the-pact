SHELL := /bin/sh
.DEFAULT_GOAL := doctor

COMPOSE := docker compose
SERVER_IMAGE ?= the-pact-server:dev
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: init dev down logs ps migrate test test-race test-integration build verify doctor
.PHONY: _compose-config

init:
	@if [ -e .env ]; then \
		echo ".env already exists; leaving it unchanged."; \
	else \
		cp .env.example .env; \
		chmod 600 .env; \
		echo "Created .env. Set PACT_DB_PASSWORD and PACT_LOCAL_API_TOKEN before make dev."; \
	fi

dev: doctor
	@$(COMPOSE) up --build --detach --wait
	@address="$$($(COMPOSE) port pact-server 8080)"; \
		echo "Pact is ready at http://$${address}"

down:
	@PACT_DB_PASSWORD=not-used PACT_LOCAL_API_TOKEN=not-used-by-down $(COMPOSE) down

logs:
	@PACT_DB_PASSWORD=not-used PACT_LOCAL_API_TOKEN=not-used-by-logs $(COMPOSE) logs --follow

ps:
	@PACT_DB_PASSWORD=not-used PACT_LOCAL_API_TOKEN=not-used-by-ps $(COMPOSE) ps

migrate: doctor
	@$(COMPOSE) run --rm --build migrate

test:
	@docker build \
		--target test \
		--build-arg GO_TEST_FLAGS= \
		--tag the-pact-test:dev \
		.

test-race:
	@docker build \
		--target test \
		--build-arg GO_TEST_FLAGS=-race \
		--tag the-pact-test-race:dev \
		.

test-integration: doctor
	@status=0; \
		$(COMPOSE) --profile tools run --rm --build integration-test || status=$$?; \
		$(COMPOSE) --profile tools rm --stop --force postgres-test >/dev/null 2>&1 || true; \
		exit $$status

build:
	@docker build \
		--target runtime \
		--build-arg VERSION="$(VERSION)" \
		--build-arg COMMIT="$(COMMIT)" \
		--build-arg BUILD_DATE="$(BUILD_DATE)" \
		--tag "$(SERVER_IMAGE)" \
		.

verify: _compose-config test build

doctor:
	@command -v docker >/dev/null 2>&1 || { echo "Docker is required."; exit 1; }
	@docker compose version >/dev/null 2>&1 || { echo "Docker Compose v2 is required."; exit 1; }
	@docker info >/dev/null 2>&1 || { echo "The Docker daemon is not available."; exit 1; }
	@test -f .env || { echo "Missing .env. Run make init."; exit 1; }
	@password="$$(sed -n 's/^PACT_DB_PASSWORD=//p' .env | tail -n 1)"; \
		[ "$${#password}" -ge 16 ] || { echo "PACT_DB_PASSWORD in .env must contain at least 16 URL-safe characters."; exit 1; }; \
		printf '%s' "$$password" | grep -Eq '^[A-Za-z0-9._~-]+$$' || { echo "PACT_DB_PASSWORD in .env must contain only URL-safe characters."; exit 1; }
	@token="$$(sed -n 's/^PACT_LOCAL_API_TOKEN=//p' .env | tail -n 1)"; \
		[ "$${#token}" -ge 24 ] || { echo "PACT_LOCAL_API_TOKEN in .env must contain at least 24 characters."; exit 1; }
	@$(COMPOSE) config --quiet
	@echo "Development environment configuration is valid."

_compose-config:
	@PACT_DB_PASSWORD=compose-validation-only \
		PACT_LOCAL_API_TOKEN=compose-validation-token-only \
		$(COMPOSE) config --quiet
