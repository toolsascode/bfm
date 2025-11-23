.PHONY: help build up down restart logs ps clean clean-all health status shell
.DEFAULT_GOAL := help

# Docker Compose file location
COMPOSE_FILE := deploy/docker-compose.yml

# Service names
BFM_SERVICE := bfm
BFM_WORKER_SERVICE := bfm-worker
FFM_SERVICE := ffm
POSTGRES_SERVICE := postgres
KAFKA_SERVICE := kafka
PULSAR_SERVICE := pulsar

# Colors for output
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
NC := \033[0m # No Color

help: ## Show this help message
	@echo "$(GREEN)BfM (Migration Operations) - Available Commands:$(NC)"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(YELLOW)%-20s$(NC) %s\n", $$1, $$2}'
	@echo ""

# ============================================================================
# Service Management
# ============================================================================

up: ## Start all services in detached mode
	@echo "$(GREEN)Starting all services...$(NC)"
	docker compose -f $(COMPOSE_FILE) up -d --remove-orphans
	@echo "$(GREEN)Services started!$(NC)"
	@make ps

up-build: ## Build and start all services
	@echo "$(GREEN)Building and starting all services...$(NC)"
	docker compose -f $(COMPOSE_FILE) up -d --build --remove-orphans
	@make ps

down: ## Stop all services
	@echo "$(YELLOW)Stopping all services...$(NC)"
	docker compose -f $(COMPOSE_FILE) down

down-volumes: ## Stop all services and remove volumes
	@echo "$(RED)Stopping all services and removing volumes...$(NC)"
	docker compose -f $(COMPOSE_FILE) down -v

restart: ## Restart all services
	@echo "$(YELLOW)Restarting all services...$(NC)"
	docker compose -f $(COMPOSE_FILE) restart
	@make ps

restart-bfm: ## Restart BFM server only
	@echo "$(YELLOW)Restarting BFM server...$(NC)"
	docker compose -f $(COMPOSE_FILE) restart $(BFM_SERVICE)

restart-worker: ## Restart BFM worker only
	@echo "$(YELLOW)Restarting BFM worker...$(NC)"
	docker compose -f $(COMPOSE_FILE) restart $(BFM_WORKER_SERVICE)

restart-ffm: ## Restart FFM frontend only
	@echo "$(YELLOW)Restarting FFM frontend...$(NC)"
	docker compose -f $(COMPOSE_FILE) restart $(FFM_SERVICE)

# ============================================================================
# Building
# ============================================================================

build: ## Build all service images
	@echo "$(GREEN)Building all service images...$(NC)"
	docker compose -f $(COMPOSE_FILE) build

build-bfm: ## Build BFM server image
	@echo "$(GREEN)Building BFM server image...$(NC)"
	docker compose -f $(COMPOSE_FILE) build $(BFM_SERVICE)

build-worker: ## Build BFM worker image
	@echo "$(GREEN)Building BFM worker image...$(NC)"
	docker compose -f $(COMPOSE_FILE) build $(BFM_WORKER_SERVICE)

build-ffm: ## Build FFM frontend image
	@echo "$(GREEN)Building FFM frontend image...$(NC)"
	docker compose -f $(COMPOSE_FILE) build $(FFM_SERVICE)

build-no-cache: ## Build all images without cache
	@echo "$(GREEN)Building all images without cache...$(NC)"
	docker compose -f $(COMPOSE_FILE) build --no-cache

prod-build: ## Build standalone production Docker image
	@echo "$(GREEN)Building standalone production Docker image...$(NC)"
	docker build -t bfm-production:latest -f docker/Dockerfile .
	@echo "$(GREEN)Production image built successfully: bfm-production:latest$(NC)"

# ============================================================================
# Standalone Production (Docker Compose)
# ============================================================================

COMPOSE_STANDALONE_FILE := deploy/docker-compose.standalone.yml

standalone-up: ## Start standalone production container
	@echo "$(GREEN)Starting standalone production container...$(NC)"
	docker compose -p bfm-standalone -f $(COMPOSE_STANDALONE_FILE) up -d --remove-orphans --build --force-recreate
	@echo "$(GREEN)Standalone container started!$(NC)"
	@make standalone-ps

standalone-down: ## Stop standalone production container
	@echo "$(YELLOW)Stopping standalone production container...$(NC)"
	docker compose -p bfm-standalone -f $(COMPOSE_STANDALONE_FILE) down

standalone-build: ## Build standalone production container
	@echo "$(GREEN)Building standalone production container...$(NC)"
	docker compose -p bfm-standalone -f $(COMPOSE_STANDALONE_FILE) build

standalone-logs: ## Show logs from standalone container
	docker compose -p bfm-standalone -f $(COMPOSE_STANDALONE_FILE) logs -f

standalone-ps: ## Show status of standalone container
	@echo "$(GREEN)Standalone Container Status:$(NC)"
	@docker compose -p bfm-standalone -f $(COMPOSE_STANDALONE_FILE) ps

standalone-restart: ## Restart standalone container
	@echo "$(YELLOW)Restarting standalone container...$(NC)"
	docker compose -p bfm-standalone -f $(COMPOSE_STANDALONE_FILE) restart
	@make standalone-ps

standalone-shell: ## Open shell in standalone container
	docker compose -p bfm-standalone -f $(COMPOSE_STANDALONE_FILE) exec bfm-standalone /bin/sh

# ============================================================================
# Logs
# ============================================================================

logs: ## Show logs from all services
	docker compose -f $(COMPOSE_FILE) logs -f

logs-bfm: ## Show logs from BFM server
	docker compose -f $(COMPOSE_FILE) logs -f $(BFM_SERVICE)

logs-worker: ## Show logs from BFM worker
	docker compose -f $(COMPOSE_FILE) logs -f $(BFM_WORKER_SERVICE)

logs-ffm: ## Show logs from FFM frontend
	docker compose -f $(COMPOSE_FILE) logs -f $(FFM_SERVICE)

logs-postgres: ## Show logs from PostgreSQL
	docker compose -f $(COMPOSE_FILE) logs -f $(POSTGRES_SERVICE)

logs-kafka: ## Show logs from Kafka
	docker compose -f $(COMPOSE_FILE) logs -f $(KAFKA_SERVICE)

logs-pulsar: ## Show logs from Pulsar
	docker compose -f $(COMPOSE_FILE) logs -f $(PULSAR_SERVICE)

# ============================================================================
# Status and Health
# ============================================================================

ps: ## Show status of all services
	@echo "$(GREEN)Service Status:$(NC)"
	@docker compose -f $(COMPOSE_FILE) ps

status: ps ## Alias for ps

health: ## Check health of all services
	@echo "$(GREEN)Health Check:$(NC)"
	@echo ""
	@echo "$(YELLOW)BFM Server:$(NC)"
	@curl -s http://localhost:7070/api/v1/health | jq . || echo "  Not responding"
	@echo ""
	@echo "$(YELLOW)FFM Frontend:$(NC)"
	@curl -s -o /dev/null -w "  HTTP Status: %{http_code}\n" http://localhost:4040 || echo "  Not responding"
	@echo ""
	@echo "$(YELLOW)PostgreSQL:$(NC)"
	@docker compose -f $(COMPOSE_FILE) exec -T $(POSTGRES_SERVICE) pg_isready -U postgres 2>/dev/null && echo "  ✓ Ready" || echo "  ✗ Not ready"
	@echo ""
	@echo "$(YELLOW)Kafka:$(NC)"
	@docker compose -f $(COMPOSE_FILE) exec -T $(KAFKA_SERVICE) kafka-broker-api-versions --bootstrap-server localhost:9092 >/dev/null 2>&1 && echo "  ✓ Ready" || echo "  ✗ Not ready"

# ============================================================================
# Shell Access
# ============================================================================

shell-bfm: ## Open shell in BFM server container
	docker compose -f $(COMPOSE_FILE) exec $(BFM_SERVICE) /bin/sh

shell-worker: ## Open shell in BFM worker container
	docker compose -f $(COMPOSE_FILE) exec $(BFM_WORKER_SERVICE) /bin/sh

shell-postgres: ## Open PostgreSQL shell
	docker compose -f $(COMPOSE_FILE) exec $(POSTGRES_SERVICE) psql -U postgres -d migration_state

shell-kafka: ## Open shell in Kafka container
	docker compose -f $(COMPOSE_FILE) exec $(KAFKA_SERVICE) /bin/bash

# ============================================================================
# Database Operations
# ============================================================================

# ============================================================================
# CLI Tools
# ============================================================================

build-cli: ## Build BfM CLI tool
	@echo "$(GREEN)Building BfM CLI...$(NC)"
	@cd api && go build -o ../bfm-cli ./cmd/cli
	@echo "$(GREEN)CLI built successfully: ./bfm-cli$(NC)"

build-migrations: build-cli ## Build migration .go files from examples
	@echo "$(GREEN)Building migration files...$(NC)"
	@./bfm-cli build examples/sfm
	@echo "$(GREEN)Migration files built successfully!$(NC)"

build-migrations-verbose: build-cli ## Build migration files with verbose output
	@echo "$(GREEN)Building migration files (verbose)...$(NC)"
	@./bfm-cli build examples/sfm --verbose

build-migrations-dry-run: build-cli ## Show what would be generated (dry run)
	@echo "$(YELLOW)Dry run - showing what would be generated...$(NC)"
	@./bfm-cli build examples/sfm --dry-run

# ============================================================================
# Database Operations
# ============================================================================

db-migrate: ## Run database migrations (placeholder - implement as needed)
	@echo "$(YELLOW)Running database migrations...$(NC)"
	@echo "  Use the FFM frontend at http://localhost:4040 to execute migrations"
	@echo "  Or use: curl -X POST http://localhost:7070/api/v1/migrations/up"

db-reset: ## Reset database (WARNING: This will delete all data!)
	@echo "$(RED)WARNING: This will delete all migration state data!$(NC)"
	@read -p "Are you sure? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		docker compose -f $(COMPOSE_FILE) exec -T $(POSTGRES_SERVICE) psql -U postgres -d migration_state -c "DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;"; \
		echo "$(GREEN)Database reset complete!$(NC)"; \
	fi

# ============================================================================
# Queue Operations
# ============================================================================

kafka-topics: ## List Kafka topics
	docker compose -f $(COMPOSE_FILE) exec $(KAFKA_SERVICE) kafka-topics --bootstrap-server localhost:9092 --list

kafka-create-topic: ## Create Kafka topic for migrations (if not exists)
	docker compose -f $(COMPOSE_FILE) exec $(KAFKA_SERVICE) kafka-topics --create --if-not-exists --bootstrap-server localhost:9092 --topic bfm-migrations --partitions 3 --replication-factor 1

kafka-fix-permissions: ## Fix Kafka volume permissions (run if you see permission errors)
	@echo "$(YELLOW)Fixing Kafka volume permissions...$(NC)"
	docker compose -f $(COMPOSE_FILE) stop $(KAFKA_SERVICE) || true
	docker compose -f $(COMPOSE_FILE) run --rm --user root $(KAFKA_SERVICE) chown -R appuser:appuser /tmp/kraft-combined-logs
	docker compose -f $(COMPOSE_FILE) run --rm --user root $(KAFKA_SERVICE) chmod -R 755 /tmp/kraft-combined-logs
	@echo "$(GREEN)Permissions fixed! Restarting Kafka...$(NC)"
	docker compose -f $(COMPOSE_FILE) up -d $(KAFKA_SERVICE)

kafka-reset: ## Reset Kafka volume (WARNING: This will delete all Kafka data!)
	@echo "$(RED)WARNING: This will delete all Kafka data!$(NC)"
	@read -p "Are you sure? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		docker compose -f $(COMPOSE_FILE) stop $(KAFKA_SERVICE) || true; \
		docker volume rm bfm_bfm-kafka-data 2>/dev/null || true; \
		echo "$(GREEN)Kafka volume reset! Restarting Kafka...$(NC)"; \
		docker compose -f $(COMPOSE_FILE) up -d $(KAFKA_SERVICE); \
	fi

kafka-consumers: ## List Kafka consumer groups
	docker compose -f $(COMPOSE_FILE) exec $(KAFKA_SERVICE) kafka-consumer-groups --bootstrap-server localhost:9092 --list

# ============================================================================
# Cleanup
# ============================================================================

clean: ## Stop and remove containers (keeps volumes)
	@echo "$(YELLOW)Cleaning up containers...$(NC)"
	docker compose -f $(COMPOSE_FILE) down

clean-all: ## Stop and remove containers, volumes, and images
	@echo "$(RED)WARNING: This will remove all containers, volumes, and images!$(NC)"
	@read -p "Are you sure? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		docker compose -f $(COMPOSE_FILE) down -v --rmi all; \
		echo "$(GREEN)Cleanup complete!$(NC)"; \
	fi

clean-logs: ## Clean Docker logs (requires sudo on some systems)
	@echo "$(YELLOW)Cleaning Docker logs...$(NC)"
	@sudo truncate -s 0 /var/lib/docker/containers/*/*-json.log 2>/dev/null || echo "  Note: May require sudo or may not be applicable on all systems"

prune: ## Remove unused Docker resources
	@echo "$(YELLOW)Pruning unused Docker resources...$(NC)"
	docker system prune -f

# ============================================================================
# GitHub Actions
# ============================================================================

github-cache-list: ## List GitHub Actions caches
	@echo "$(GREEN)Listing GitHub Actions caches...$(NC)"
	@if ! command -v gh &> /dev/null; then \
		echo "$(RED)Error: GitHub CLI (gh) is not installed$(NC)"; \
		echo "  Install it from: https://cli.github.com/"; \
		exit 1; \
	fi
	@gh cache list || echo "$(YELLOW)Note: Make sure you're authenticated with 'gh auth login'$(NC)"

github-cache-delete: ## Delete all GitHub Actions caches (interactive)
	@echo "$(RED)WARNING: This will delete all GitHub Actions caches!$(NC)"
	@if ! command -v gh &> /dev/null; then \
		echo "$(RED)Error: GitHub CLI (gh) is not installed$(NC)"; \
		echo "  Install it from: https://cli.github.com/"; \
		exit 1; \
	fi
	@read -p "Are you sure? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		echo "$(YELLOW)Deleting all GitHub Actions caches...$(NC)"; \
		gh cache list --limit 1000 | grep -v "^key\|^---" | awk '{print $$1}' | xargs -I {} gh cache delete {} || echo "$(YELLOW)No caches found or error occurred$(NC)"; \
		echo "$(GREEN)Cache cleanup complete!$(NC)"; \
	fi

github-cache-clean: github-cache-delete ## Alias for github-cache-delete

# ============================================================================
# Development
# ============================================================================

dev: up ## Start all services for development
	@echo "$(GREEN)Development environment ready!$(NC)"
	@echo ""
	@echo "  BFM Server:    http://localhost:7070"
	@echo "  FFM Frontend:   http://localhost:4040"
	@echo "  PostgreSQL:     localhost:5433"
	@echo "  Kafka:          localhost:9092"
	@echo ""
	@echo "  View logs:      make logs"
	@echo "  Check status:   make ps"

dev-logs: ## Show logs from all services (development)
	docker compose -f $(COMPOSE_FILE) logs -f

dev-bfm: ## Start BFM backend with hot-reload (requires air)
	@echo "$(GREEN)Starting BFM with hot-reload...$(NC)"
	@echo "$(YELLOW)Note: Make sure 'air' is installed: go install github.com/air-verse/air@latest$(NC)"
	@cd bfm && air

dev-ffm: ## Start FFM frontend with hot-reload
	@echo "$(GREEN)Starting FFM with hot-reload...$(NC)"
	@cd ffm && npm run dev

dev-local: ## Start both BFM and FFM locally with hot-reload (requires air and npm)
	@echo "$(GREEN)Starting local development with hot-reload...$(NC)"
	@echo "$(YELLOW)Starting BFM backend...$(NC)"
	@cd bfm && air &
	@echo "$(YELLOW)Starting FFM frontend...$(NC)"
	@cd ffm && npm run dev
	@echo "$(GREEN)Both services running with hot-reload!$(NC)"
	@echo "$(YELLOW)Press Ctrl+C to stop both services$(NC)"

# ============================================================================
# Docker Development (with hot-reload)
# ============================================================================

COMPOSE_DEV_FILE := deploy/docker-compose.dev.yml

dev-docker: dev-docker-build ## Start all services in Docker with hot-reload
	@echo "$(GREEN)Starting development environment with hot-reload in Docker...$(NC)"
	docker compose -f $(COMPOSE_DEV_FILE) up -d
	@echo "$(GREEN)Development environment ready!$(NC)"
	@echo ""
	@echo "  BFM Server:    http://localhost:7070"
	@echo "  FFM Frontend:  http://localhost:4040"
	@echo "  PostgreSQL:    localhost:5433"
	@echo ""
	@echo "  View logs:     make dev-docker-logs"
	@echo "  Stop:          make dev-docker-down"
	@make dev-docker-ps

dev-docker-build: ## Build development Docker images
	@echo "$(GREEN)Building development images...$(NC)"
	docker compose -f $(COMPOSE_DEV_FILE) build

dev-docker-down: ## Stop development Docker services
	@echo "$(YELLOW)Stopping development services...$(NC)"
	docker compose -f $(COMPOSE_DEV_FILE) down

dev-docker-logs: ## Show logs from development services
	docker compose -f $(COMPOSE_DEV_FILE) logs -f

dev-docker-logs-bfm: ## Show logs from BFM server (dev)
	docker compose -f $(COMPOSE_DEV_FILE) logs -f bfm-server

dev-docker-logs-ffm: ## Show logs from FFM frontend (dev)
	docker compose -f $(COMPOSE_DEV_FILE) logs -f ffm

dev-docker-ps: ## Show status of development services
	@echo "$(GREEN)Development Service Status:$(NC)"
	@docker compose -f $(COMPOSE_DEV_FILE) ps

dev-docker-restart: ## Restart development services
	@echo "$(YELLOW)Restarting development services...$(NC)"
	docker compose -f $(COMPOSE_DEV_FILE) restart
	@make dev-docker-ps

dev-docker-clean: ## Stop and remove development containers and volumes
	@echo "$(RED)WARNING: This will remove all development containers and volumes!$(NC)"
	@read -p "Are you sure? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		docker compose -f $(COMPOSE_DEV_FILE) down -v; \
		echo "$(GREEN)Cleanup complete!$(NC)"; \
	fi

# ============================================================================
# Testing
# ============================================================================

test-api: ## Test BFM API health endpoint
	@echo "$(GREEN)Testing BFM API...$(NC)"
	@curl -s http://localhost:7070/api/v1/health | jq . || echo "$(RED)API not responding$(NC)"

test-frontend: ## Test FFM frontend
	@echo "$(GREEN)Testing FFM frontend...$(NC)"
	@curl -s -o /dev/null -w "HTTP Status: %{http_code}\n" http://localhost:4040

test-all: test-api test-frontend ## Run all tests
	@echo "$(GREEN)All tests complete!$(NC)"

# ============================================================================
# Pre-commit Hooks
# ============================================================================

pre-commit-install: ## Install pre-commit hooks
	@echo "$(GREEN)Installing pre-commit hooks...$(NC)"
	@if ! command -v pre-commit &> /dev/null; then \
		echo "$(YELLOW)pre-commit not found. Installing...$(NC)"; \
		pip install pre-commit || brew install pre-commit || echo "$(RED)Please install pre-commit manually: pip install pre-commit$(NC)"; \
	fi
	@pre-commit install || (echo "$(RED)Failed to install pre-commit hooks$(NC)" && exit 1)
	@echo "$(GREEN)Pre-commit hooks installed!$(NC)"

pre-commit-uninstall: ## Uninstall pre-commit hooks
	@echo "$(YELLOW)Uninstalling pre-commit hooks...$(NC)"
	@pre-commit uninstall || true
	@echo "$(GREEN)Pre-commit hooks uninstalled!$(NC)"

pre-commit-run: ## Run pre-commit hooks on staged files
	@echo "$(GREEN)Running pre-commit hooks...$(NC)"
	@pre-commit run || (echo "$(RED)Pre-commit hooks failed$(NC)" && exit 1)

pre-commit-run-all: ## Run pre-commit hooks on all files
	@echo "$(GREEN)Running pre-commit hooks on all files...$(NC)"
	@pre-commit run --all-files || (echo "$(RED)Pre-commit hooks failed$(NC)" && exit 1)

pre-commit-update: ## Update pre-commit hooks to latest versions
	@echo "$(GREEN)Updating pre-commit hooks...$(NC)"
	@pre-commit autoupdate
	@echo "$(GREEN)Pre-commit hooks updated!$(NC)"

pre-commit-clean: ## Clean pre-commit cache
	@echo "$(YELLOW)Cleaning pre-commit cache...$(NC)"
	@pre-commit clean
	@echo "$(GREEN)Pre-commit cache cleaned!$(NC)"

precommit: pre-commit-run ## Alias for pre-commit-run

# ============================================================================
# Quick Actions
# ============================================================================

quick-start: build up ## Quick start: build and start all services
	@make health

quick-stop: down ## Quick stop: stop all services

quick-restart: restart ## Quick restart: restart all services

# ============================================================================
# Information
# ============================================================================

info: ## Show system information
	@echo "$(GREEN)BfM System Information:$(NC)"
	@echo ""
	@echo "$(YELLOW)Docker Compose Version:$(NC)"
	@docker compose version
	@echo ""
	@echo "$(YELLOW)Docker Version:$(NC)"
	@docker --version
	@echo ""
	@echo "$(YELLOW)Service Status:$(NC)"
	@make ps
	@echo ""
	@echo "$(YELLOW)Network Information:$(NC)"
	@docker network inspect bfm-network 2>/dev/null | jq -r '.[0].Containers | to_entries[] | "  \(.key): \(.value.IPv4Address)"' || echo "  Network not found"

version: ## Show version information
	@echo "$(GREEN)BfM Version Information:$(NC)"
	@echo "  BFM: Backend For Migrations"
	@echo "  FFM: Frontend For Migrations"
	@echo ""
	@echo "Docker Compose:"
	@docker compose version
	@echo ""
	@echo "Docker:"
	@docker --version
