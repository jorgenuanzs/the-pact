SHELL := /bin/sh
.DEFAULT_GOAL := doctor

COMPOSE := docker compose
SERVER_IMAGE ?= the-pact-server:dev
PACT_BUILDER ?= the-pact-builder
PACT_CACHE_MAX_AGE ?= 168h
PACT_CACHE_KEEP_STORAGE ?= 1GB
PACT_TEMP_IMAGES := the-pact-test:dev the-pact-test-race:dev the-pact-integration-test:dev the-pact-source:dev
PACT_IMAGES := $(SERVER_IMAGE) $(PACT_TEMP_IMAGES)
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
HOST_OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
HOST_ARCH_RAW := $(shell uname -m)
HOST_ARCH := $(if $(filter arm64 aarch64,$(HOST_ARCH_RAW)),arm64,amd64)

.PHONY: init cli dev down logs ps migrate test test-race test-integration build verify doctor
.PHONY: docker-builder docker-audit docker-clean-stale docker-clean
.PHONY: _compose-config

init:
	@if [ -e .env ]; then \
		echo ".env already exists; leaving it unchanged."; \
	else \
		cp .env.example .env; \
		chmod 600 .env; \
		echo "Created .env. Set PACT_DB_PASSWORD and PACT_SETUP_TOKEN before the first make dev."; \
	fi

cli: docker-builder
	@mkdir -p bin
	@docker buildx build \
		--builder "$(PACT_BUILDER)" \
		--target cli-artifact \
		--build-arg VERSION="$(VERSION)" \
		--build-arg COMMIT="$(COMMIT)" \
		--build-arg BUILD_DATE="$(BUILD_DATE)" \
		--build-arg CLI_GOOS="$(HOST_OS)" \
		--build-arg CLI_GOARCH="$(HOST_ARCH)" \
		--output type=local,dest=bin \
		.

dev: doctor docker-builder
	@$(COMPOSE) build --builder "$(PACT_BUILDER)"
	@$(COMPOSE) up --detach --wait --no-build
	@address="$$($(COMPOSE) port pact-server 8080)"; \
		echo "Pact is ready at http://$${address}"

down:
	@PACT_DB_PASSWORD=not-used $(COMPOSE) down

logs:
	@PACT_DB_PASSWORD=not-used $(COMPOSE) logs --follow

ps:
	@PACT_DB_PASSWORD=not-used $(COMPOSE) ps

migrate: doctor docker-builder
	@$(COMPOSE) build --builder "$(PACT_BUILDER)" migrate
	@$(COMPOSE) run --rm migrate

test: docker-builder
	@docker buildx build \
		--builder "$(PACT_BUILDER)" \
		--target test \
		--build-arg GO_TEST_FLAGS= \
		--tag the-pact-test:dev \
		--load \
		.
	@docker image rm the-pact-test:dev >/dev/null

test-race: docker-builder
	@docker buildx build \
		--builder "$(PACT_BUILDER)" \
		--target test \
		--build-arg GO_TEST_FLAGS=-race \
		--tag the-pact-test-race:dev \
		--load \
		.
	@docker image rm the-pact-test-race:dev >/dev/null

test-integration: doctor docker-builder
	@status=0; \
		$(COMPOSE) --profile tools build --builder "$(PACT_BUILDER)" integration-test || status=$$?; \
		if [ "$$status" -eq 0 ]; then \
			$(COMPOSE) --profile tools run --rm integration-test || status=$$?; \
		fi; \
		$(COMPOSE) --profile tools rm --stop --force postgres-test >/dev/null 2>&1 || true; \
		docker image rm the-pact-integration-test:dev >/dev/null 2>&1 || true; \
		exit $$status

build: docker-builder
	@docker buildx build \
		--builder "$(PACT_BUILDER)" \
		--target runtime \
		--build-arg VERSION="$(VERSION)" \
		--build-arg COMMIT="$(COMMIT)" \
		--build-arg BUILD_DATE="$(BUILD_DATE)" \
		--tag "$(SERVER_IMAGE)" \
		--load \
		.

verify: _compose-config test build
	@$(MAKE) --no-print-directory docker-clean-stale

docker-builder:
	@docker buildx inspect "$(PACT_BUILDER)" >/dev/null 2>&1 || \
		docker buildx create --name "$(PACT_BUILDER)" --driver docker-container >/dev/null
	@docker buildx inspect "$(PACT_BUILDER)" --bootstrap >/dev/null

docker-audit:
	@echo "PACT containers"
	@docker ps --all \
		--filter label=com.docker.compose.project=the-pact \
		--format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Size}}'
	@echo "PACT images"
	@docker image ls \
		--filter label=io.pact.project=the-pact \
		--format 'table {{.Repository}}:{{.Tag}}\t{{.ID}}\t{{.Size}}\t{{.CreatedSince}}'
	@echo "PACT volumes (never removed automatically)"
	@docker volume ls \
		--filter label=com.docker.compose.project=the-pact \
		--format 'table {{.Name}}\t{{.Driver}}'
	@if docker buildx inspect "$(PACT_BUILDER)" >/dev/null 2>&1; then \
		echo "PACT build cache"; \
		docker buildx du --builder "$(PACT_BUILDER)"; \
	fi

docker-clean-stale:
	@docker image rm $(PACT_TEMP_IMAGES) >/dev/null 2>&1 || true
	@docker container prune --force \
		--filter label=com.docker.compose.project=the-pact >/dev/null
	@docker image prune --force \
		--filter label=io.pact.project=the-pact >/dev/null
	@if docker buildx inspect "$(PACT_BUILDER)" >/dev/null 2>&1; then \
		docker buildx prune \
			--builder "$(PACT_BUILDER)" \
			--all \
			--force \
			--filter until="$(PACT_CACHE_MAX_AGE)" >/dev/null; \
		docker buildx prune \
			--builder "$(PACT_BUILDER)" \
			--all \
			--force \
			--keep-storage "$(PACT_CACHE_KEEP_STORAGE)" >/dev/null; \
	fi
	@echo "Removed stale PACT containers, images, and build cache; volumes were preserved."

docker-clean:
	@PACT_DB_PASSWORD=not-used \
		$(COMPOSE) down --remove-orphans
	@docker image rm $(PACT_IMAGES) >/dev/null 2>&1 || true
	@docker image prune --force \
		--filter label=io.pact.project=the-pact >/dev/null
	@if docker buildx inspect "$(PACT_BUILDER)" >/dev/null 2>&1; then \
		docker buildx prune \
			--builder "$(PACT_BUILDER)" \
			--all \
			--force >/dev/null; \
	fi
	@echo "Removed local PACT containers, images, and build cache; the PostgreSQL volume was preserved."

doctor:
	@command -v docker >/dev/null 2>&1 || { echo "Docker is required."; exit 1; }
	@docker compose version >/dev/null 2>&1 || { echo "Docker Compose v2 is required."; exit 1; }
	@docker buildx version >/dev/null 2>&1 || { echo "Docker Buildx is required."; exit 1; }
	@docker info >/dev/null 2>&1 || { echo "The Docker daemon is not available."; exit 1; }
	@test -f .env || { echo "Missing .env. Run make init."; exit 1; }
	@password="$$(sed -n 's/^PACT_DB_PASSWORD=//p' .env | tail -n 1)"; \
		[ "$${#password}" -ge 16 ] || { echo "PACT_DB_PASSWORD in .env must contain at least 16 URL-safe characters."; exit 1; }; \
		printf '%s' "$$password" | grep -Eq '^[A-Za-z0-9._~-]+$$' || { echo "PACT_DB_PASSWORD in .env must contain only URL-safe characters."; exit 1; }
	@setup_token="$$(sed -n 's/^PACT_SETUP_TOKEN=//p' .env | tail -n 1)"; \
		[ -z "$$setup_token" ] || [ "$${#setup_token}" -ge 24 ] || { echo "PACT_SETUP_TOKEN in .env must be blank or contain at least 24 characters."; exit 1; }
	@$(COMPOSE) config --quiet
	@echo "Development environment configuration is valid."

_compose-config:
	@PACT_DB_PASSWORD=compose-validation-only \
		$(COMPOSE) config --quiet
