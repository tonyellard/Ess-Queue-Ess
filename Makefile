.PHONY: build run run-with-config test clean docker-build docker-run docker-stop config help

# Variables
BINARY_NAME=ess-queue-ess
DOCKER_IMAGE=ess-queue-ess:latest
PORT=9324
CONFIG_FILE=config.yaml

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the Go binary
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) .

run: ## Run the application locally
	@echo "Running $(BINARY_NAME) on port $(PORT)..."
	PORT=$(PORT) go run .

run-with-config: config ## Run with configuration file
	@echo "Running $(BINARY_NAME) with config on port $(PORT)..."
	PORT=$(PORT) go run . --config $(CONFIG_FILE)

test: ## Run unit tests
	@echo "Running tests..."
	go test -v ./...

clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	go clean

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE) .

docker-run: ## Run Docker container
	@echo "Starting Docker container..."
	docker compose up -d

docker-stop: ## Stop Docker container
	@echo "Stopping Docker container..."
	docker compose down

docker-logs: ## Show Docker container logs
	docker compose logs -f

docker-shell: ## Open shell in running container
	docker compose exec ess-queue-ess sh

integration-test: ## Run integration tests
	@echo "Running integration tests..."
	@if [ -d test ]; then \
		cd test && python3 integration_test.py; \
	else \
		echo "No test directory found"; \
	fi

deps: ## Download Go dependencies
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

config: ## Create config.yaml from example
	@if [ ! -f $(CONFIG_FILE) ]; then \
		cp config.example.yaml $(CONFIG_FILE); \
		echo "Created $(CONFIG_FILE) from example"; \
	else \
		echo "$(CONFIG_FILE) already exists"; \
	fi

all: build ## Build everything
