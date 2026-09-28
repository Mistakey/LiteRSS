# Makefile for LiteRSS (Wails v3 + Task)
.PHONY: help dev build package run test test-frontend test-backend lint lint-frontend format format-backend install-deps update-deps check setup clean love static-check release-check test-all test-coverage lint-backend

# Detect OS
ifeq ($(OS),Windows_NT)
    DETECTED_OS := Windows
    SHELL := pwsh.exe
    .SHELLFLAGS := -Command
    TASK := task.exe
else
    DETECTED_OS := $(shell uname -s)
    SHELL := /bin/bash
    TASK := task
endif

PYTHON ?= python

# Default target
help: ## Show this help message
	@echo "LiteRSS Development Makefile ($(DETECTED_OS))"
	@echo ""
	@echo "Wails v3 Build System - Using Task Runner"
	@echo ""
	@echo "Available targets:"
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
	@echo ""
	@echo "💡 Tip: Use 'task --list' to see all available tasks"

# Development (Wails v3 + Task)
dev: ## Start development server with hot reload
	$(TASK) dev

# Building (Wails v3 + Task)
build: ## Build application for current platform
	$(TASK) build

package: ## Package application with installer
	$(TASK) package

run: ## Run the built application
	$(TASK) run

build-frontend: ## Build frontend only
	$(TASK) common:build:frontend

build-backend: ## Build backend only
	go build -v -o build/bin/ ./...

# Testing
test: test-frontend test-backend ## Run all tests

test-frontend: ## Run frontend tests
	cd frontend && npm run test:unit

test-backend: ## Run backend tests
	go test -v -timeout=5m -cover ./internal/...

test-coverage: ## Run backend tests with coverage
	go test -v -timeout=5m -coverprofile=coverage.out -covermode=atomic ./internal/...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-all: test-frontend test-backend ## Run all tests

# Code Quality
lint: lint-frontend lint-backend ## Run all linters

lint-frontend: ## Run frontend linter
	cd frontend && npm run lint:check

lint-backend: ## Check Go vet and formatting without rewriting files
	go vet ./...
	$(PYTHON) scripts/verify.py format

format: format-frontend format-backend ## Format all code

format-frontend: ## Format frontend code
	cd frontend && npm run format

format-backend: ## Format backend code
ifeq ($(DETECTED_OS),Windows)
	powershell -Command '$$files = Get-ChildItem -Recurse -Include *.go; gofmt -w $$files; goimports -w $$files'
else
	find . -name '*.go' -exec gofmt -w {} +
	find . -name '*.go' -exec goimports -w {} +
endif

# Dependencies
install-deps: install-frontend-deps install-backend-deps ## Install all dependencies

install-frontend-deps: ## Install frontend dependencies
	cd frontend && npm install

install-backend-deps: ## Install backend dependencies
	go mod download

update-deps: update-frontend-deps update-backend-deps ## Update all dependencies

update-frontend-deps: ## Update frontend dependencies
	cd frontend && npm update

update-backend-deps: ## Update backend dependencies
	go get -u ./...
	go mod tidy

# Setup
setup: install-deps ## Initial project setup
	@echo "Dependencies installed."

# Task runner commands
task-list: ## List all available tasks
	$(TASK) --list

task-summary: ## Show task summary
	$(TASK) --summary build dev package

icons: ## Generate platform icons
	$(TASK) common:generate:icons

setup-docker: ## Setup Docker for cross-compilation
	$(TASK) common:setup:docker

# Clean
clean: ## Clean build artifacts
ifeq ($(DETECTED_OS),Windows)
	-Remove-Item -Recurse -Force build/bin,frontend/dist,coverage.out,coverage.html,*.syso 2>$$null
else
	rm -rf build/bin frontend/dist coverage.out coverage.html *.syso
endif
	@echo "✅ Cleaned build artifacts"

# Development helpers
check: ## Run source checks, unit tests and compile once
	$(PYTHON) scripts/verify.py check

release-check: ## Run checks plus dependency and version validation once
	$(PYTHON) scripts/verify.py release

love: ## Show some love
	@echo "❤️ LiteRSS loves you too! ❤️"

# Platform-specific builds
build-windows: ## Build for Windows
	$(TASK) windows:build

build-darwin: ## Build for macOS
	$(TASK) darwin:build

static-check: ## Run staticcheck for Go code analysis
	staticcheck ./...
