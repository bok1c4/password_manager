#!/bin/bash

# ===========================================
# Password Manager - Development & Startup Scripts
# ===========================================
# 
# This script provides convenient commands for building,
# running, and managing the password manager application.
#
# Usage: ./scripts/start.sh <command>

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_DIR/bin"
CLI_BIN="$BUILD_DIR/pwman"
SERVER_BIN="$BUILD_DIR/pwman-server"
DESKTOP_BIN="$PROJECT_DIR/src-tauri/target/release/pwman"
RELEASE_DIR="$BUILD_DIR"

# ===========================================
# Utility Functions
# ===========================================

print_header() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "${YELLOW}→ $1${NC}"
}

check_binary() {
    if [ -f "$1" ]; then
        return 0
    else
        return 1
    fi
}

wait_for_server() {
    local port="${1:-18475}"
    local max_attempts="${2:-30}"
    
    print_info "Waiting for server on port $port..."
    for i in $(seq 1 $max_attempts); do
        if curl -s "http://localhost:$port/api/health" > /dev/null 2>&1; then
            print_success "Server is ready"
            return 0
        fi
        sleep 1
    done
    
    print_error "Server did not start within ${max_attempts}s"
    return 1
}

# ===========================================
# Build Commands
# ===========================================

build-cli() {
    print_header "Building CLI"
    cd "$PROJECT_DIR"
    
    mkdir -p "$BUILD_DIR"
    
    go build -ldflags "-X main.Version=$(git describe --tags --always 2>/dev/null || echo 'dev')" \
        -o "$CLI_BIN" ./cmd/pwman
    
    print_success "CLI built: $CLI_BIN"
    ls -lh "$CLI_BIN"
}

build-server() {
    print_header "Building API Server"
    cd "$PROJECT_DIR"
    
    mkdir -p "$BUILD_DIR"
    
    go build -ldflags "-X main.Version=$(git describe --tags --always 2>/dev/null || echo 'dev')" \
        -o "$SERVER_BIN" ./cmd/server
    
    print_success "Server built: $SERVER_BIN"
    ls -lh "$SERVER_BIN"
}

build-desktop() {
    print_header "Building Desktop App"
    cd "$PROJECT_DIR"
    
    if [ ! -f "package.json" ]; then
        print_error "package.json not found. Run 'npm install' first."
        exit 1
    fi
    
    npm run tauri build
    
    print_success "Desktop app built: $DESKTOP_BIN"
    ls -lh "$DESKTOP_BIN" 2>/dev/null || print_info "Binary location may vary"
}

build-all() {
    print_header "Building All Binaries"
    
    build-cli
    echo ""
    build-server
    echo ""
    build-desktop
    
    echo ""
    print_header "Build Summary"
    echo "CLI:     $CLI_BIN"
    echo "Server:  $SERVER_BIN"
    echo "Desktop: $DESKTOP_BIN"
    print_success "All binaries built successfully"
}

# ===========================================
# Development Commands
# ===========================================

dev-server() {
    print_header "Starting Development Server"
    cd "$PROJECT_DIR"
    
    print_info "Running server with hot reload..."
    print_info "Server will start on http://localhost:18475"
    print_info "Press Ctrl+C to stop"
    echo ""
    
    go run ./cmd/server
}

dev-frontend() {
    print_header "Starting Frontend Development Server"
    cd "$PROJECT_DIR"
    
    print_info "Starting Vite dev server..."
    print_info "Frontend will be available at http://localhost:1420"
    print_info "Press Ctrl+C to stop"
    echo ""
    
    npm run dev
}

dev-tauri() {
    print_header "Starting Tauri Development Mode"
    cd "$PROJECT_DIR"
    
    print_info "Starting Tauri with hot reload..."
    print_info "Press Ctrl+C to stop"
    echo ""
    
    npm run tauri dev
}

dev-all() {
    print_header "Starting Full Development Stack"
    
    # Kill any existing processes
    pkill -f pwman-server 2>/dev/null || true
    
    # Start server in background
    print_info "Starting API server..."
    dev-server &
    SERVER_PID=$!
    
    # Wait for server
    wait_for_server 18475 30
    
    # Start Tauri
    print_info "Starting Tauri..."
    npm run tauri dev
    
    # Cleanup
    kill $SERVER_PID 2>/dev/null || true
}

# ===========================================
# Production Commands
# ===========================================

start-server() {
    print_header "Starting Production Server"
    
    if ! check_binary "$SERVER_BIN"; then
        print_error "Server binary not found. Run 'build-server' first."
        exit 1
    fi
    
    # Kill existing server
    pkill -f pwman-server 2>/dev/null || true
    sleep 1
    
    print_info "Starting server on port 18475..."
    print_info "Press Ctrl+C to stop"
    echo ""
    
    "$SERVER_BIN"
}

start-desktop() {
    print_header "Starting Desktop Application"
    
    if ! check_binary "$DESKTOP_BIN"; then
        print_error "Desktop binary not found. Run 'build-desktop' first."
        exit 1
    fi
    
    print_info "Starting desktop app..."
    
    # Set environment for better rendering on Linux
    export WEBKIT_DISABLE_COMPOSITING_MODE=1
    export GTK_THEME=Adwaita
    
    "$DESKTOP_BIN"
}

start-all() {
    print_header "Starting Full Production Stack"
    
    # Verify binaries exist
    if ! check_binary "$SERVER_BIN"; then
        print_error "Server binary not found. Run 'build-all' first."
        exit 1
    fi
    
    if ! check_binary "$DESKTOP_BIN"; then
        print_error "Desktop binary not found. Run 'build-all' first."
        exit 1
    fi
    
    # Kill existing processes
    pkill -f pwman-server 2>/dev/null || true
    pkill -f "target/release/pwman" 2>/dev/null || true
    sleep 1
    
    # Start server in background
    start-server &
    SERVER_PID=$!
    
    # Wait for server
    wait_for_server 18475 30
    
    # Start desktop app
    start-desktop
    
    # Cleanup
    kill $SERVER_PID 2>/dev/null || true
}

# ===========================================
# Testing Commands
# ===========================================

test-unit() {
    print_header "Running Unit Tests"
    cd "$PROJECT_DIR"
    
    go test -v ./...
    print_success "Unit tests passed"
}

test-race() {
    print_header "Running Tests with Race Detector"
    cd "$PROJECT_DIR"
    
    go test -race ./...
    print_success "Race detection tests passed"
}

test-coverage() {
    print_header "Running Tests with Coverage"
    cd "$PROJECT_DIR"
    
    go test -cover ./...
    echo ""
    print_info "For detailed HTML report, run: make test-coverage-html"
}

# ===========================================
# Utility Commands
# ===========================================

clean-binaries() {
    print_header "Cleaning Binaries"
    cd "$PROJECT_DIR"
    
    rm -rf "$BUILD_DIR"
    mkdir -p "$BUILD_DIR"
    
    # Restore .gitkeep
    echo "# This file ensures the bin/ directory is tracked by Git" > "$BUILD_DIR/.gitkeep"
    echo "# Binary files are ignored via .gitignore" >> "$BUILD_DIR/.gitkeep"
    
    print_success "Binaries cleaned (bin/ directory reset)"
}

clean-all() {
    print_header "Deep Clean"
    cd "$PROJECT_DIR"
    
    rm -f pwman pwman-server main server
    rm -rf bin/
    rm -rf src-tauri/target
    rm -rf node_modules
    rm -rf dist
    rm -f coverage.out coverage.html
    
    print_success "Deep clean complete"
}

clean-data() {
    print_header "⚠️  Clean User Data"
    
    echo -e "${RED}WARNING: This will permanently delete ALL user data!${NC}"
    echo -e "${RED}This includes:${NC}"
    echo "  - All vaults"
    echo "  - All passwords"
    echo "  - All encryption keys"
    echo "  - All configuration"
    echo ""
    read -p "Type 'DELETE' to confirm: " confirm
    
    if [ "$confirm" = "DELETE" ]; then
        rm -rf ~/.pwman
        print_success "User data removed"
    else
        print_error "Cancelled"
        exit 1
    fi
}

reset-dev() {
    print_header "Reset Development Environment"
    
    clean-binaries
    echo ""
    clean-data
    echo ""
    
    print_info "Rebuilding..."
    build-all
    
    print_success "Development environment reset complete"
}

# ===========================================
# Release Commands
# ===========================================

release() {
    print_header "Building Release Binaries"
    cd "$PROJECT_DIR"
    
    mkdir -p "$RELEASE_DIR"
    
    VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")
    LDFLAGS="-ldflags -X main.Version=$VERSION"
    
    print_info "Building for Linux AMD64..."
    GOOS=linux GOARCH=amd64 go build $LDFLAGS -o "$RELEASE_DIR/pwman-linux-amd64" ./cmd/pwman
    GOOS=linux GOARCH=amd64 go build $LDFLAGS -o "$RELEASE_DIR/pwman-server-linux-amd64" ./cmd/server
    
    print_info "Building for Linux ARM64..."
    GOOS=linux GOARCH=arm64 go build $LDFLAGS -o "$RELEASE_DIR/pwman-linux-arm64" ./cmd/pwman
    GOOS=linux GOARCH=arm64 go build $LDFLAGS -o "$RELEASE_DIR/pwman-server-linux-arm64" ./cmd/server
    
    print_info "Building for macOS AMD64..."
    GOOS=darwin GOARCH=amd64 go build $LDFLAGS -o "$RELEASE_DIR/pwman-darwin-amd64" ./cmd/pwman
    GOOS=darwin GOARCH=amd64 go build $LDFLAGS -o "$RELEASE_DIR/pwman-server-darwin-amd64" ./cmd/server
    
    print_info "Building for macOS ARM64..."
    GOOS=darwin GOARCH=arm64 go build $LDFLAGS -o "$RELEASE_DIR/pwman-darwin-arm64" ./cmd/pwman
    GOOS=darwin GOARCH=arm64 go build $LDFLAGS -o "$RELEASE_DIR/pwman-server-darwin-arm64" ./cmd/server
    
    print_info "Building for Windows AMD64..."
    GOOS=windows GOARCH=amd64 go build $LDFLAGS -o "$RELEASE_DIR/pwman-windows-amd64.exe" ./cmd/pwman
    GOOS=windows GOARCH=amd64 go build $LDFLAGS -o "$RELEASE_DIR/pwman-server-windows-amd64.exe" ./cmd/server
    
    echo ""
    print_success "Release binaries created in $RELEASE_DIR/"
    ls -lh "$RELEASE_DIR/"
}

# ===========================================
# Status Commands
# ===========================================

status() {
    print_header "Project Status"
    
    echo "Binaries (in bin/):"
    check_binary "$CLI_BIN" && echo "  ✓ CLI:     $CLI_BIN" || echo "  ✗ CLI:     not built"
    check_binary "$SERVER_BIN" && echo "  ✓ Server:  $SERVER_BIN" || echo "  ✗ Server:  not built"
    check_binary "$DESKTOP_BIN" && echo "  ✓ Desktop: $DESKTOP_BIN" || echo "  ✗ Desktop: not built"
    
    echo ""
    echo "Data:"
    if [ -d ~/.pwman ]; then
        echo "  ✓ Vault directory exists"
        echo "    Location: ~/.pwman"
        if [ -f ~/.pwman/config.json ]; then
            ACTIVE_VAULT=$(grep active_vault ~/.pwman/config.json | cut -d'"' -f4)
            echo "    Active vault: $ACTIVE_VAULT"
        fi
    else
        echo "  ✗ No vault data found"
    fi
    
    echo ""
    echo "Server:"
    if curl -s http://localhost:18475/api/health > /dev/null 2>&1; then
        echo "  ✓ Server running on port 18475"
    else
        echo "  ✗ Server not running"
    fi
}

# ===========================================
# Help
# ===========================================

show-help() {
    echo "Password Manager - Development & Startup Scripts"
    echo ""
    echo -e "${YELLOW}Usage:${NC} $0 <command>"
    echo ""
    echo -e "${YELLOW}Build Commands:${NC}"
    echo "  build-cli       Build CLI tool"
    echo "  build-server    Build API server"
    echo "  build-desktop   Build Tauri desktop app"
    echo "  build-all       Build all binaries"
    echo ""
    echo -e "${YELLOW}Development Commands:${NC}"
    echo "  dev-server      Run API server in dev mode (with logs)"
    echo "  dev-frontend    Run frontend dev server (Vite)"
    echo "  dev-tauri       Run Tauri in dev mode"
    echo "  dev-all         Run server + Tauri in dev mode"
    echo ""
    echo -e "${YELLOW}Production Commands:${NC}"
    echo "  start-server    Run API server (production binary)"
    echo "  start-desktop   Run desktop app (production binary)"
    echo "  start-all       Run server + desktop (production)"
    echo ""
    echo -e "${YELLOW}Testing Commands:${NC}"
    echo "  test-unit       Run unit tests"
    echo "  test-race       Run tests with race detector"
    echo "  test-coverage   Run tests with coverage"
    echo ""
    echo -e "${YELLOW}Utility Commands:${NC}"
    echo "  clean-binaries  Remove built binaries"
    echo "  clean-all       Deep clean (binaries, deps, build artifacts)"
    echo "  clean-data      Remove ALL user data (⚠️ destructive)"
    echo "  reset-dev       Clean everything and rebuild"
    echo "  status          Show project status"
    echo ""
    echo -e "${YELLOW}Release Commands:${NC}"
    echo "  release         Build release binaries for all platforms"
    echo ""
    echo -e "${YELLOW}Examples:${NC}"
    echo "  $0 build-all              # Build everything"
    echo "  $0 dev-server             # Start development server"
    echo "  $0 start-all              # Run full production stack"
    echo "  $0 test-race              # Run race detection tests"
    echo "  $0 reset-dev              # Clean slate rebuild"
    echo ""
}

# ===========================================
# Main
# ===========================================

case "$1" in
    # Build commands
    build-cli)
        build-cli
        ;;
    build-server)
        build-server
        ;;
    build-desktop)
        build-desktop
        ;;
    build-all)
        build-all
        ;;
    
    # Development commands
    dev-server)
        dev-server
        ;;
    dev-frontend)
        dev-frontend
        ;;
    dev-tauri)
        dev-tauri
        ;;
    dev-all)
        dev-all
        ;;
    
    # Production commands
    start-server)
        start-server
        ;;
    start-desktop)
        start-desktop
        ;;
    start-all)
        start-all
        ;;
    
    # Testing commands
    test-unit)
        test-unit
        ;;
    test-race)
        test-race
        ;;
    test-coverage)
        test-coverage
        ;;
    
    # Utility commands
    clean-binaries)
        clean-binaries
        ;;
    clean-all)
        clean-all
        ;;
    clean-data)
        clean-data
        ;;
    reset-dev)
        reset-dev
        ;;
    status)
        status
        ;;
    
    # Release commands
    release)
        release
        ;;
    
    # Help
    help|--help|-h)
        show-help
        ;;
    
    # Unknown command
    *)
        print_error "Unknown command: $1"
        echo ""
        show-help
        exit 1
        ;;
esac
