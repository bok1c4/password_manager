#!/bin/bash

# ===========================================
# Password Manager - Startup Scripts
# ===========================================

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
SERVER_BIN="$PROJECT_DIR/pwman-server"
TAURI_BIN="$PROJECT_DIR/src-tauri/target/release/pwman"

# ===========================================
# Build Everything
# ===========================================

build-all() {
    echo -e "${YELLOW}Building Go server...${NC}"
    cd "$PROJECT_DIR"
    go build -o pwman-server ./cmd/server
    
    echo -e "${YELLOW}Building Tauri app...${NC}"
    cd "$PROJECT_DIR"
    npm run tauri build
    
    echo -e "${GREEN}Build complete!${NC}"
    echo "  Server: $SERVER_BIN"
    echo "  App:    $TAURI_BIN"
}

# ===========================================
# Development Mode
# ===========================================

dev-server() {
    echo -e "${YELLOW}Starting Go API server...${NC}"
    cd "$PROJECT_DIR"
    go run ./cmd/server
}

dev-frontend() {
    echo -e "${YELLOW}Starting frontend dev server...${NC}"
    cd "$PROJECT_DIR"
    npm run dev
}

dev-tauri() {
    echo -e "${YELLOW}Starting Tauri dev mode...${NC}"
    cd "$PROJECT_DIR"
    npm run tauri dev
}

# ===========================================
# Production Mode
# ===========================================

start-server() {
    if [ ! -f "$SERVER_BIN" ]; then
        echo -e "${RED}Server not found. Run 'build-all' first.${NC}"
        exit 1
    fi
    
    # Kill existing server on port
    fuser -k 18475/tcp 2>/dev/null || true
    
    echo -e "${YELLOW}Starting Go API server on port 18475...${NC}"
    cd "$PROJECT_DIR"
    "$SERVER_BIN"
}

start-app() {
    if [ ! -f "$TAURI_BIN" ]; then
        echo -e "${RED}App not found. Run 'build-all' first.${NC}"
        exit 1
    fi
    
    echo -e "${YELLOW}Starting Password Manager app...${NC}"
    export WEBKIT_DISABLE_COMPOSITING_MODE=1
    export GTK_THEME=Adwaita
    "$TAURI_BIN"
}

# ===========================================
# Full Stack (Server + App)
# ===========================================

start-full() {
    # Kill any existing processes
    pkill -f pwman-server 2>/dev/null || true
    pkill -f "target/release/pwman" 2>/dev/null || true
    sleep 1
    
    # Start server in background
    start-server &
    SERVER_PID=$!
    
    # Wait for server to start and be ready
    echo -e "${YELLOW}Waiting for server...${NC}"
    for i in {1..10}; do
        if curl -s http://localhost:18475/api/is_initialized > /dev/null 2>&1; then
            break
        fi
        sleep 1
    done
    
    # Start app
    start-app
    
    # Cleanup on exit
    kill $SERVER_PID 2>/dev/null || true
}

# ===========================================
# Help
# ===========================================

show-help() {
    echo "Password Manager - Startup Scripts"
    echo ""
    echo "Usage: $0 <command>"
    echo ""
    echo "Commands:"
    echo "  build-all       Build everything (server + app)"
    echo "  dev-server      Run Go API server in development"
    echo "  dev-frontend    Run frontend dev server only"
    echo "  dev-tauri       Run Tauri in development mode"
    echo "  start-server    Run Go API server (production)"
    echo "  start-app       Run desktop app (production)"
    echo "  start-full      Run both server + app"
    echo "  help            Show this help"
    echo ""
    echo "Examples:"
    echo "  $0 build-all      # Build everything first time"
    echo "  $0 start-full    # Run full application"
}

# ===========================================
# Main
# ===========================================

case "$1" in
    build-all)
        build-all
        ;;
    dev-server)
        dev-server
        ;;
    dev-frontend)
        dev-frontend
        ;;
    dev-tauri)
        dev-tauri
        ;;
    start-server)
        start-server
        ;;
    start-app)
        start-app
        ;;
    start-full)
        start-full
        ;;
    help|--help|-h)
        show-help
        ;;
    *)
        echo -e "${RED}Unknown command: $1${NC}"
        show-help
        exit 1
        ;;
esac
