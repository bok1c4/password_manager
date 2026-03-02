# AI Agent Instructions

This document provides instructions for AI assistants working on this password manager project.

---

## Project Overview

A cross-platform password manager written in Go with:
- Hybrid encryption (AES-256-GCM + PGP)
- SQLite storage
- P2P-based sync between devices (via libp2p)
- Multi-device support with device-specific keys

---

## Key Files

| File | Purpose |
|------|---------|
| `ARCHITECTURE.md` | System design and data models |
| `PLAN.md` | Implementation tasks breakdown |
| `STATUS.md` | Current development status (update after changes) |
| `AGENTS.md` | This file - AI operating instructions |

---

## Development Workflow

### Before Starting Work

1. Read `STATUS.md` to understand current state
2. Read `PLAN.md` to identify next tasks
3. Check `ARCHITECTURE.md` for design decisions

### After Completing Work

1. Update `STATUS.md`:
   - Update component status
   - Add entry to "Recent Activity"
   - Note any blockers

2. Run tests if applicable:
   ```bash
   make test
   ```

3. Run linter:
   ```bash
   make lint
   ```

---

## Code Conventions

### Go Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- No commented-out code
- Exported functions must have documentation comments

### Project Structure

```
cmd/pwman/          - CLI entry point only
internal/           - Private application code
  cli/              - Cobra command handlers
  crypto/           - Encryption operations
  storage/          - Database operations
  p2p/              - P2P sync logic (libp2p)
  device/           - Device management
  config/           - Configuration
pkg/models/         - Public data models (shared)
```

### Error Handling

- Return errors, don't panic
- Wrap errors with context using `fmt.Errorf`
- User-facing errors should be clear and actionable

### Logging

- Use `fmt` for CLI output
- Prefix messages with status: `[INFO]`, `[ERROR]`, `[WARN]`

---

## Commit Conventions

Format: `<type>: <message>`

Types:
- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation
- `refactor:` Code refactoring
- `test:` Tests
- `chore:` Maintenance

Example: `feat: add password generation command`

---

## Security Considerations

When working with crypto code:
- Never log passwords or keys
- Clear sensitive data from memory when possible
- Use constant-time comparison where relevant
- Never hardcode keys or secrets

---

## Testing Requirements

- Unit tests for crypto functions (must pass)
- Unit tests for storage layer
- Integration tests for CLI commands
- Manual testing for multi-device flows

---

## Dependencies

Adding new dependencies:
1. Check if truly necessary
2. Prefer standard library
3. Use well-maintained packages
4. Run `go mod tidy` after changes

Current key dependencies:
- `github.com/spf13/cobra` - CLI framework
- `github.com/ProtonMail/gopenpgp/v3` - PGP encryption
- `github.com/mattn/go-sqlite3` - SQLite driver
- `github.com/libp2p/go-libp2p` - P2P networking

---

## Common Commands

```bash
# Build
go build -o pwman ./cmd/pwman

# Run
go run ./cmd/pwman [command]

# Test
go test ./...

# Test with coverage
go test -cover ./...

# Lint (requires golangci-lint)
golangci-lint run
```

---

## Multi-Device Flow Reference (P2P)

```
Device A (existing)          Device B (new)
─────────────────            ───────────────
pwman init --name "Arch"     
         │
         │ stores: device_id, public_key
         │
pwman add github.com         
         │
         │ encrypts password
         │ stores AES key encrypted for Device A
         │
pwman p2p start              
         │                     pwman init --name "Mac"
         │                     │
         │◄────────────────────┤ generates own key pair
         │                     │
         │                     pwman p2p start
         │                     │ (mDNS discovers Device A)
         │◄───────────────────►│ auto-connect
         │                     │
         │  HELLO exchange     │
         │  REQUEST_APPROVAL   │
         │                     │
User approves on Device A ──►│
         │                     │
         │  APPROVE_DEVICE     │ (re-encrypted keys)
         │◄───────────────────┤
         │                     │
         │  SYNC_DATA          │ (all entries)
         │◄───────────────────►│
         │                     │ can now decrypt all
```

---

## Import from C++ Reference

C++ database tables:
- `passwords`: id, password (encrypted), aes_key (encrypted), note
- `user_public_keys`: public_key, fingerprint, username

Import process:
1. User provides PostgreSQL connection string
2. User provides private key file + passphrase
3. For each entry:
   - Decrypt AES key using private key
   - Re-encrypt password with AES-256-GCM
   - Encrypt AES key for current Go device
4. Store in SQLite
