# Build System Documentation

Complete guide for building, running, and managing the Password Manager application.

---

## Table of Contents

1. [Quick Start](#quick-start)
2. [Binaries](#binaries)
3. [Build Commands](#build-commands)
4. [Development Commands](#development-commands)
5. [Production Commands](#production-commands)
6. [Testing Commands](#testing-commands)
7. [Utility Commands](#utility-commands)
8. [Release Builds](#release-builds)
9. [Project Structure](#project-structure)

---

## Quick Start

### First Time Setup

```bash
# Install dependencies
make deps

# Build everything
make build-all
# OR
./scripts/start.sh build-all

# Run in development
./scripts/start.sh dev-all
```

### Daily Development

```bash
# Start development server
make dev
# OR
./scripts/start.sh dev-server

# Run tests
make test

# Check code quality
make check
```

---

## Binaries

The project builds three main binaries, the `bin/` directory:

| Binary | Source | Purpose | Location |
|--------|--------|---------|----------|
| `pwman` | `cmd/pwman/main.go` | CLI tool for password management | `bin/pwman` |
| `pwman-server` | `cmd/server/main.go` | HTTP API server | `bin/pwman-server` |
| `pwman-desktop` | `src-tauri/` | Tauri desktop application | `src-tauri/target/release/pwman` |

### Old Binaries (No Longer Used)

The following old binaries are no longer used and should be removed:
- `./main` - Old server binary
- `./server` - Old server binary (duplicate)
- `./pwman` - Now in `bin/` directory
- `./pwman-server` - Now in `bin/` directory

Clean them with:
```bash
make clean
# OR
./scripts/start.sh clean-binaries
```

---

## Build Commands

### Using Make

```bash
# Build CLI (default)
make build
make build-cli

# Build API server
make build-server

# Build desktop app
make build-desktop

# Build all
make build-all
```

### Using start.sh

```bash
./scripts/start.sh build-cli
./scripts/start.sh build-server
./scripts/start.sh build-desktop
./scripts/start.sh build-all
```

### Build Output

```
✓ CLI built: bin/pwman (15M)
✓ Server built: bin/pwman-server (39M)
✓ Desktop app built: ./src-tauri/target/release/pwman
```

---

## Development Commands

### Development Server

Run the API server with logging and hot reload:

```bash
# Using Make
make dev
make run-server

# Using start.sh
./scripts/start.sh dev-server
```

Server starts on: `http://localhost:18475`

### Frontend Development

Run the Vite dev server for frontend development:

```bash
./scripts/start.sh dev-frontend
```

Frontend available at: `http://localhost:1420`

### Tauri Development

Run Tauri in development mode with hot reload:

```bash
./scripts/start.sh dev-tauri
```

### Full Development Stack

Run both server and Tauri in development mode:

```bash
./scripts/start.sh dev-all
```

This will:
1. Start the API server in background
2. Wait for server to be ready
3. Start Tauri in dev mode
4. Clean up server on exit

---

## Production Commands

### Start Production Server

Run the compiled server binary:

```bash
./scripts/start.sh start-server
```

### Start Desktop App

Run the compiled desktop application:

```bash
./scripts/start.sh start-desktop
```

### Full Production Stack

Run both server and desktop app:

```bash
./scripts/start.sh start-all
```

---

## Testing Commands

### Unit Tests

```bash
# Run all tests
make test

# Run with verbose output
go test -v ./...

# Run tests for specific package
go test ./cmd/server/handlers/...
```

### Race Detection

```bash
# Run tests with race detector
make test-race

# Or directly
go test -race ./...
```

### Coverage

```bash
# Run with coverage summary
make test-coverage

# Generate HTML coverage report
make test-coverage-html
open coverage.html
```

### Using start.sh

```bash
./scripts/start.sh test-unit
./scripts/start.sh test-race
./scripts/start.sh test-coverage
```

---

## Utility Commands

### Clean Binaries

Remove built binaries:

```bash
make clean
# OR
./scripts/start.sh clean-binaries
```

Removes:
- `pwman`
- `pwman-server`
- `main` (old)
- `server` (old)
- `bin/` directory

### Deep Clean

Remove everything (binaries, dependencies, build artifacts):

```bash
make clean-all
# OR
./scripts/start.sh clean-all
```

Removes:
- All binaries
- `node_modules/`
- `src-tauri/target/`
- `dist/`
- Coverage files

### Clean User Data

⚠️ **WARNING: This permanently deletes ALL user data!**

```bash
./scripts/start.sh clean-data
```

Removes:
- `~/.pwman/` (all vaults, passwords, keys, config)

You will be prompted to type 'DELETE' to confirm.

### Reset Development Environment

Complete reset and rebuild:

```bash
./scripts/start.sh reset-dev
```

This will:
1. Clean all binaries
2. Remove user data (with confirmation)
3. Rebuild everything

### Check Status

Show project status:

```bash
./scripts/start.sh status
```

Output:
```
Binaries:
  ✓ CLI:     ./pwman
  ✓ Server:  ./pwman-server
  ✗ Desktop: not built

Data:
  ✓ Vault directory exists
    Location: ~/.pwman
    Active vault: personal

Server:
  ✗ Server not running
```

---

## Release Builds

### Build for All Platforms

Build release binaries for Linux, macOS, and Windows:

```bash
make release
# OR
./scripts/start.sh release
```

Creates binaries in `bin/`:
```
bin/
├── pwman-darwin-amd64
├── pwman-darwin-arm64
├── pwman-linux-amd64
├── pwman-linux-arm64
├── pwman-server-darwin-amd64
├── pwman-server-darwin-arm64
├── pwman-server-linux-amd64
├── pwman-server-linux-arm64
├── pwman-server-windows-amd64.exe
├── pwman-windows-amd64.exe
```

### Platform-Specific Builds

```bash
# Linux only
make release-linux

# macOS only
make release-darwin

# CLI only
make release-cli

# Server only
make release-server
```

---

## Project Structure

```
password_manager/
├── cmd/
│   ├── pwman/              # CLI entry point
│   │   └── main.go
│   └── server/             # HTTP API server
│       ├── main.go         # Server entry point (150 lines)
│       ├── handlers/       # Extracted handlers
│       │   ├── auth.go
│       │   ├── entry.go
│       │   ├── vault.go
│       │   ├── device.go
│       │   ├── health.go
│       │   ├── p2p.go
│       │   └── pairing.go
│       └── *_test.go
├── internal/
│   ├── api/                # API utilities
│   │   ├── response.go
│   │   ├── validation.go
│   │   ├── ratelimit.go
│   │   └── auth.go
│   ├── middleware/         # HTTP middleware
│   │   ├── cors.go
│   │   └── auth.go
│   ├── state/              # Server state management
│   │   └── server.go
│   ├── cli/                # CLI commands
│   ├── crypto/             # Encryption
│   ├── storage/            # SQLite database
│   ├── p2p/                # P2P networking
│   └── config/             # Configuration
├── pkg/
│   └── models/             # Shared data models
├── src-tauri/              # Tauri desktop app
├── src/                    # React frontend
├── scripts/
│   └── start.sh            # Development scripts
├── docs/
│   ├── BUILD.md            # This file
│   └── ...
├── Makefile                # Build automation
├── package.json            # NPM dependencies
└── go.mod                  # Go dependencies
```

---

## Common Workflows

### Starting Fresh After Refactoring

```bash
# 1. Clean old binaries and data
./scripts/start.sh clean-binaries
./scripts/start.sh clean-data  # Type 'DELETE' to confirm

# 2. Install dependencies
make deps

# 3. Build everything
./scripts/start.sh build-all

# 4. Run in development
./scripts/start.sh dev-all
```

### Testing Changes

```bash
# 1. Format code
make fmt

# 2. Run linter
make lint

# 3. Run tests
make test-race

# 4. Run full check
make check
```

### Releasing

```bash
# 1. Run all checks
make check

# 2. Build release binaries
make release

# 3. Test release binaries
./bin/pwman-server-linux-amd64 &
curl http://localhost:18475/api/health
pkill pwman-server

# 4. Package for distribution
tar -czf pwman-linux-amd64.tar.gz -C bin pwman-linux-amd64 pwman-server-linux-amd64
```

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PWMAN_PORT` | `18475` | Server port |
| `PWMAN_PORT_RANGE` | - | Port range (e.g., `18475-18485`) |
| `PWMAN_BASE_PATH` | `~/.pwman` | Data directory |
| `WEBKIT_DISABLE_COMPOSITING_MODE` | - | WebKit compositing (Linux) |
| `GTK_THEME` | - | GTK theme (Linux) |

---

## Troubleshooting

### Port Already in Use

```bash
# Kill process on port 18475
fuser -k 18475/tcp

# Or use pkill
pkill -f pwman-server
```

### Binary Not Found

```bash
# Check if binary exists
ls -la pwman pwman-server

# Build if missing
make build-all
```

### Tests Failing

```bash
# Run with verbose output
go test -v ./...

# Check for race conditions
go test -race ./...

# Clean and rebuild
make clean && make build-all
```

### Desktop App Won't Start

```bash
# Check if built
ls -la src-tauri/target/release/pwman

# Build if missing
./scripts/start.sh build-desktop

# Set environment (Linux)
export WEBKIT_DISABLE_COMPOSITING_MODE=1
export GTK_THEME=Adwaita
```

---

## Make vs start.sh

Both `make` and `start.sh` can be used:

| Task | Make | start.sh |
|------|------|----------|
| Build CLI | `make build-cli` | `./scripts/start.sh build-cli` |
| Build all | `make build-all` | `./scripts/start.sh build-all` |
| Run tests | `make test` | `./scripts/start.sh test-unit` |
| Clean | `make clean` | `./scripts/start.sh clean-binaries` |
| Dev server | `make dev` | `./scripts/start.sh dev-server` |
| Status | N/A | `./scripts/start.sh status` |
| Reset | N/A | `./scripts/start.sh reset-dev` |

**Use Make for:**
- Standard builds
- CI/CD pipelines
- Quick commands

**Use start.sh for:**
- Development workflows
- Complex operations (reset, status)
- Interactive operations

---

## Need Help?

```bash
# Show Make help
make help

# Show start.sh help
./scripts/start.sh help

# Check project status
./scripts/start.sh status
```
