.PHONY: all build build-cli build-server build-desktop build-all clean clean-binaries test test-coverage lint fmt run run-cli run-server run-desktop dev deps release release-cli release-server release-all help

# Binary names
CLI_BIN := pwman
SERVER_BIN := pwman-server
DESKTOP_BIN := src-tauri/target/release/pwman

# Build directory
BUILD_DIR := bin

# Full paths
CLI_PATH := $(BUILD_DIR)/$(CLI_BIN)
SERVER_PATH := $(BUILD_DIR)/$(SERVER_BIN)

# Source paths
CLI_SRC := ./cmd/pwman
SERVER_SRC := ./cmd/server
DESKTOP_SRC := ./src-tauri

# Version info
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Default target
all: build

# ===========================================
# Build Targets
# ===========================================

build: build-cli
	@echo "✓ Default build (CLI) complete"

build-cli:
	@echo "Building CLI..."
	@mkdir -p $(BUILD_DIR)
	@go build $(LDFLAGS) -o $(CLI_PATH) $(CLI_SRC)
	@echo "✓ CLI built: $(CLI_PATH)"

build-server:
	@echo "Building API server..."
	@mkdir -p $(BUILD_DIR)
	@go build $(LDFLAGS) -o $(SERVER_PATH) $(SERVER_SRC)
	@echo "✓ Server built: $(SERVER_PATH)"

build-desktop:
	@echo "Building Tauri desktop app..."
	@npm run tauri build
	@echo "✓ Desktop app built: $(DESKTOP_BIN)"

build-all: build-cli build-server build-desktop
	@echo ""
	@echo "========================================"
	@echo "Build Summary"
	@echo "========================================"
	@echo "CLI:     $(CLI_PATH)"
	@echo "Server:  $(SERVER_PATH)"
	@echo "Desktop: $(DESKTOP_BIN)"
	@echo ""
	@ls -lh $(BUILD_DIR)/
	@echo ""
	@echo "✓ All binaries built successfully"

# ===========================================
# Development Targets
# ===========================================

run: run-cli

run-cli:
	@go run $(CLI_SRC)

run-server:
	@go run $(SERVER_SRC)

run-desktop:
	@npm run tauri dev

dev: run-server
	@echo "Starting development server..."

# ===========================================
# Testing Targets
# ===========================================

test:
	@go test -v ./...

test-race:
	@go test -race ./...

test-coverage:
	@go test -cover ./...

test-coverage-html:
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# ===========================================
# Code Quality Targets
# ===========================================

lint:
	@golangci-lint run

fmt:
	@go fmt ./...
	@echo "✓ Code formatted"

vet:
	@go vet ./...

check: fmt vet lint test
	@echo "✓ All checks passed"

# ===========================================
# Release Targets
# ===========================================

release-cli:
	@echo "Building CLI for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BIN)-linux-amd64 $(CLI_SRC)
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BIN)-linux-arm64 $(CLI_SRC)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BIN)-darwin-amd64 $(CLI_SRC)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BIN)-darwin-arm64 $(CLI_SRC)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BIN)-windows-amd64.exe $(CLI_SRC)
	@echo "✓ CLI release builds created in $(BUILD_DIR)/"

release-server:
	@echo "Building server for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BIN)-linux-amd64 $(SERVER_SRC)
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BIN)-linux-arm64 $(SERVER_SRC)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BIN)-darwin-amd64 $(SERVER_SRC)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BIN)-darwin-arm64 $(SERVER_SRC)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BIN)-windows-amd64.exe $(SERVER_SRC)
	@echo "✓ Server release builds created in $(BUILD_DIR)/"

release: release-cli release-server
	@echo ""
	@echo "========================================"
	@echo "Release Build Summary"
	@echo "========================================"
	@ls -lh $(BUILD_DIR)/
	@echo ""
	@echo "✓ All release builds created"

# ===========================================
# Cleanup Targets
# ===========================================

clean:
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "✓ Cleaned build directory"

clean-desktop:
	@rm -rf $(DESKTOP_SRC)/target
	@echo "✓ Cleaned desktop build artifacts"

clean-all: clean clean-desktop
	@rm -rf node_modules
	@rm -rf dist
	@echo "✓ Deep clean complete"

clean-data:
	@echo "⚠️  This will delete ALL user data (vaults, passwords, config)!"
	@read -p "Are you sure? (yes/no): " confirm && [ "$$confirm" = "yes" ] || exit 1
	@rm -rf ~/.pwman
	@echo "✓ User data removed"

# ===========================================
# Dependencies
# ===========================================

deps:
	@go mod download
	@go mod tidy
	@npm install
	@echo "✓ Dependencies installed"

deps-go:
	@go mod download
	@go mod tidy

deps-npm:
	@npm install

# ===========================================
# Installation
# ===========================================

install-cli: build-cli
	@cp $(CLI_PATH) /usr/local/bin/
	@echo "✓ CLI installed to /usr/local/bin/$(CLI_BIN)"

install-server: build-server
	@cp $(SERVER_PATH) /usr/local/bin/
	@echo "✓ Server installed to /usr/local/bin/$(SERVER_BIN)"

uninstall:
	@rm -f /usr/local/bin/$(CLI_BIN)
	@rm -f /usr/local/bin/$(SERVER_BIN)
	@echo "✓ Uninstalled binaries"

# ===========================================
# Help
# ===========================================

help:
	@echo "Password Manager Build System"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Build Targets:"
	@echo "  build            Build CLI (default)"
	@echo "  build-cli        Build CLI tool → $(BUILD_DIR)/$(CLI_BIN)"
	@echo "  build-server     Build API server → $(BUILD_DIR)/$(SERVER_BIN)"
	@echo "  build-desktop    Build Tauri desktop app"
	@echo "  build-all        Build all binaries"
	@echo ""
	@echo "Run Targets:"
	@echo "  run              Run CLI (default)"
	@echo "  run-cli          Run CLI tool"
	@echo "  run-server       Run API server"
	@echo "  run-desktop      Run Tauri in dev mode"
	@echo "  dev              Run development server"
	@echo ""
	@echo "Test Targets:"
	@echo "  test             Run tests"
	@echo "  test-race        Run tests with race detector"
	@echo "  test-coverage    Run tests with coverage"
	@echo "  test-coverage-html Generate HTML coverage report"
	@echo ""
	@echo "Quality Targets:"
	@echo "  lint             Run golangci-lint"
	@echo "  fmt              Format code"
	@echo "  vet              Run go vet"
	@echo "  check            Run all checks (fmt, vet, lint, test)"
	@echo ""
	@echo "Release Targets:"
	@echo "  release-cli      Build CLI for all platforms → $(BUILD_DIR)/"
	@echo "  release-server   Build server for all platforms → $(BUILD_DIR)/"
	@echo "  release          Build all release binaries"
	@echo ""
	@echo "Clean Targets:"
	@echo "  clean            Remove build directory"
	@echo "  clean-desktop    Remove desktop build artifacts"
	@echo "  clean-all        Deep clean (build dir, deps, artifacts)"
	@echo "  clean-data       Remove ALL user data (⚠️ destructive)"
	@echo ""
	@echo "Install Targets:"
	@echo "  install-cli      Install CLI to /usr/local/bin"
	@echo "  install-server   Install server to /usr/local/bin"
	@echo "  uninstall        Remove installed binaries"
	@echo ""
	@echo "Dependencies:"
	@echo "  deps             Install all dependencies"
	@echo "  deps-go          Install Go dependencies"
	@echo "  deps-npm         Install npm dependencies"
	@echo ""
	@echo "Build Output:"
	@echo "  All binaries are built to: $(BUILD_DIR)/"
	@echo ""
	@echo "Examples:"
	@echo "  make build-all              # Build everything to $(BUILD_DIR)/"
	@echo "  make test-race              # Run tests with race detector"
	@echo "  make release                # Build release binaries for all platforms"
	@echo "  make clean                  # Remove $(BUILD_DIR)/ directory"
