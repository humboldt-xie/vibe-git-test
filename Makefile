.PHONY: build clean test install docker-up docker-down docker-logs docker-status

# Binary name
BINARY=vibe-git

# Build the binary
build:
	go build -o $(BINARY) .

# Install dependencies
deps:
	go mod download
	go mod tidy

# Run tests
test:
	go test ./...

# Clean build artifacts
clean:
	rm -f $(BINARY)
	go clean

# Install to $GOPATH/bin
install:
	go install .

# Format code
fmt:
	go fmt ./...

# Run linter
lint:
	golangci-lint run 2>/dev/null || echo "golangci-lint not installed"

# Docker commands
docker-up:
	docker-compose up -d
	@echo "Waiting for services to be ready..."
	@sleep 3
	@./docker/scripts/claude-exec.sh status 2>/dev/null || echo "Services starting, check with: make docker-status"

docker-down:
	docker-compose down

docker-build:
	docker-compose build --no-cache

docker-logs:
	docker-compose logs -f

docker-status:
	@./docker/scripts/claude-exec.sh status

docker-shell:
	docker exec -it vibe-git-worker /bin/bash

docker-gateway-shell:
	docker exec -it vibe-git-gateway /bin/sh

# Claude commands via Docker
docker-claude-version:
	@./docker/scripts/claude-exec.sh api version

docker-claude-init:
	@./docker/scripts/claude-exec.sh run init

docker-git-status:
	@./docker/scripts/claude-exec.sh git-status

docker-project-info:
	@./docker/scripts/claude-exec.sh project-info

# Development with Docker
# Usage: make docker-issue ISSUE=42 OWNER=myorg REPO=myproject
docker-issue: build
	@if [ -z "$(ISSUE)" ]; then echo "Usage: make docker-issue ISSUE=42 OWNER=myorg REPO=myproject"; exit 1; fi
	./$(BINARY) issue $(ISSUE) \
		--owner $(or $(OWNER),$(shell git remote get-url origin 2>/dev/null | sed 's/.*github.com[:/]//;s/.git$$//' | cut -d'/' -f1)) \
		--repo $(or $(REPO),$(shell git remote get-url origin 2>/dev/null | sed 's/.*github.com[:/]//;s/.git$$//' | cut -d'/' -f2)) \
		--worker-url http://localhost:3000 \
		--worker-token worker-secret-token

# Watch mode with Docker
docker-watch: build
	./$(BINARY) watch \
		--owner $(or $(OWNER),$(shell git remote get-url origin 2>/dev/null | sed 's/.*github.com[:/]//;s/.git$$//' | cut -d'/' -f1)) \
		--repo $(or $(REPO),$(shell git remote get-url origin 2>/dev/null | sed 's/.*github.com[:/]//;s/.git$$//' | cut -d'/' -f2)) \
		--worker-url http://localhost:3000 \
		--worker-token worker-secret-token \
		$(ARGS)

# Help
help:
	@echo "Vibe-Git Makefile"
	@echo ""
	@echo "Development Commands:"
	@echo "  make build           - Build the binary"
	@echo "  make deps            - Download dependencies"
	@echo "  make test            - Run tests"
	@echo "  make clean           - Clean build artifacts"
	@echo "  make install         - Install to GOPATH/bin"
	@echo ""
	@echo "Docker Commands:"
	@echo "  make docker-up       - Start Docker services"
	@echo "  make docker-down     - Stop Docker services"
	@echo "  make docker-build    - Rebuild Docker images"
	@echo "  make docker-logs     - View Docker logs"
	@echo "  make docker-status   - Check Docker service status"
	@echo "  make docker-shell    - Enter worker container shell"
	@echo ""
	@echo "Claude via Docker:"
	@echo "  make docker-claude-version  - Check Claude version"
	@echo "  make docker-claude-init     - Initialize Claude in project"
	@echo "  make docker-git-status      - Check git status"
	@echo "  make docker-project-info    - Show project info"
	@echo ""
	@echo "Usage with Docker:"
	@echo "  make docker-issue ISSUE=42 OWNER=myorg REPO=myproject"
	@echo "  make docker-watch OWNER=myorg REPO=myproject ARGS='--auto-merge'"
