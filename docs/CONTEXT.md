# Project Context - Password Manager

**Version**: 1.0  
**Last Updated**: 2026-03-05  
**Status**: MVP Complete, Security Hardening In Progress

---

## What Success Looks Like

### Product Vision
A secure, open-source password manager that prioritizes:
1. **Zero-knowledge architecture** - We can't access your passwords
2. **True ownership** - Your data stays on your devices
3. **No cloud required** - P2P sync without servers
4. **Cross-platform** - Desktop + CLI, mobile future

### Success Metrics

#### Security (Critical)
- [ ] All CRITICAL vulnerabilities from security audit resolved
- [ ] No plaintext passwords in logs, memory, or network
- [ ] API authentication prevents unauthorized access
- [ ] Rate limiting prevents brute force attacks
- [ ] CORS properly configured

#### Functionality (Must Have)
- [ ] Initialize vault with strong encryption
- [ ] Add/Edit/Delete password entries
- [ ] Copy passwords to clipboard with auto-clear
- [ ] P2P sync between devices on LAN
- [ ] Device approval workflow
- [ ] Multi-vault support
- [ ] CLI and Desktop app both functional

#### Quality (Should Have)
- [ ] Test coverage >80% for critical paths
- [ ] No race conditions (verified with `-race`)
- [ ] No goroutine leaks
- [ ] Documentation complete and up-to-date
- [ ] Code passes linting (`gofmt`, `staticcheck`)

#### User Experience (Nice to Have)
- [ ] Sub-second response times
- [ ] Clear error messages
- [ ] Intuitive pairing flow
- [ ] Import from other password managers

---

## Current State

### ✅ Working
- Hybrid encryption (AES-256-GCM + RSA-2048/4096)
- SQLite storage with device registry
- CLI with full CRUD operations
- Tauri desktop app with React frontend
- P2P sync via libp2p (LAN only)
- Device approval via pairing codes
- Multi-vault support
- Clipboard auto-clear

### ⚠️ Known Issues
- **5 CRITICAL security vulnerabilities** (see SECURITY_REMEDIATION_PLAN.md)
- No API authentication (any local process can access vault)
- CORS allows all origins
- Password transmitted over P2P protocol
- RSA keys are 2048-bit (should be 4096)
- Race conditions in vault access
- Goroutine leaks in P2P handlers

### 🚫 Not Implemented
- Remote sync (relay server needed for non-LAN)
- Mobile apps
- Browser extension
- Password health monitoring
- Cloud backup option
- Hardware key support (YubiKey)

---

## Technical Stack

### Backend
- **Language**: Go 1.25+
- **Storage**: SQLite (github.com/mattn/go-sqlite3)
- **Crypto**: 
  - Standard library (crypto/aes, crypto/rsa)
  - golang.org/x/crypto/scrypt
- **P2P**: libp2p (github.com/libp2p/go-libp2p)
- **CLI**: Cobra (github.com/spf13/cobra)
- **UUID**: github.com/google/uuid

### Frontend
- **Framework**: React 18 + TypeScript
- **State**: Zustand
- **Styling**: Tailwind CSS
- **Desktop**: Tauri v2
- **Routing**: React Router v7
- **API**: Native fetch (REST)

### Build Tools
- **Go**: Native toolchain
- **Frontend**: Vite
- **Desktop**: Cargo (Tauri)
- **Testing**: Go testing + Race detector

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     PASSWORD MANAGER                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌──────────────┐      ┌──────────────┐      ┌──────────┐  │
│  │   Desktop    │      │  Go Server   │      │  Vault   │  │
│  │  (Tauri)     │◀────▶│  (Port: ?)   │◀────▶│  Files   │  │
│  │  React + TS  │ HTTP │              │      │          │  │
│  └──────────────┘      └──────────────┘      └──────────┘  │
│                              │                              │
│                              ▼                              │
│                        ┌──────────────┐                     │
│                        │    P2P       │                     │
│                        │   (libp2p)   │                     │
│                        └──────────────┘                     │
│                              │                              │
│                              ▼                              │
│                        ┌──────────────┐                     │
│                        │ Other Device │                     │
│                        └──────────────┘                     │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Data Flow
1. **Desktop app** → HTTP API → **Go server**
2. **Go server** → Decrypt with private key → **SQLite**
3. **Go server** → P2P protocol → **Other devices**
4. **CLI** → Direct library calls → **SQLite**

---

## Project Structure

```
pwman/
├── cmd/
│   ├── pwman/           # CLI entry point
│   └── server/          # API server entry point
├── internal/
│   ├── api/             # HTTP handlers, middleware
│   ├── cli/             # Cobra CLI commands
│   ├── config/          # Configuration management
│   ├── crypto/          # Encryption/decryption
│   ├── device/          # Device management
│   ├── p2p/             # P2P networking
│   ├── storage/         # SQLite database
│   └── vault/           # Vault operations (refactor)
├── pkg/
│   └── models/          # Shared data models
├── src/                 # Frontend (React + TS)
│   ├── components/      # React components
│   ├── hooks/           # Custom hooks
│   ├── lib/             # API client
│   └── pages/           # Page components
├── src-tauri/           # Tauri configuration
├── docs/                # Documentation
└── agents/              # AI agent instructions
```

---

## Security Model

### Threat Model

**Trusted**:
- User with correct vault password
- User's devices that have been approved
- Local machine (with API auth implemented)

**Untrusted**:
- Network attackers
- Other local processes
- Unapproved devices
- Compromised frontend

### Encryption Architecture

```
Password Entry:
  ┌─────────────────┐
  │   Plaintext     │
  │   Password      │
  └────────┬────────┘
           │ AES-256-GCM
           ▼
  ┌─────────────────┐     ┌─────────────────┐
  │ Encrypted       │     │ AES Key         │
  │ Password        │     │ (random)        │
  └─────────────────┘     └────────┬────────┘
                                   │
                    ┌──────────────┼──────────────┐
                    │ RSA Encrypt  │              │
                    ▼              ▼              ▼
            ┌──────────┐   ┌──────────┐   ┌──────────┐
            │ Device 1 │   │ Device 2 │   │ Device N │
            │ (pubkey) │   │ (pubkey) │   │ (pubkey) │
            └──────────┘   └──────────┘   └──────────┘
```

### Key Hierarchy

1. **Vault Password** → scrypt → Encryption Key
2. **Encryption Key** → AES-256-GCM → Private Key File
3. **Private Key** → RSA Decrypt → AES Keys
4. **AES Keys** → AES-256-GCM Decrypt → Passwords

---

## Development Workflow

### Getting Started

```bash
# 1. Clone and build
go build -o pwman ./cmd/pwman
go build -o pwman-server ./cmd/server

# 2. Initialize vault
./pwman init --name "My Device"

# 3. Start server
./pwman-server

# 4. Run tests
go test ./...
go test -race ./...

# 5. Build frontend
cd src && npm install && npm run build

# 6. Build desktop app
cd src-tauri && cargo build --release
```

### Code Standards

**Go**:
- Use `gofmt` for formatting
- Pass `go vet` and `staticcheck`
- All exported functions documented
- Error handling: return errors, don't panic

**TypeScript**:
- Strict mode enabled
- No `any` types
- All functions typed
- Use functional components

### Git Workflow

```bash
# Commit format
type: description

# Types:
#   feat: New feature
#   fix: Bug fix
#   docs: Documentation
#   refactor: Code refactoring
#   test: Tests
#   security: Security fix
#   perf: Performance

# Examples:
feat: add device approval workflow
fix: handle null pointer in get entry
security: add API authentication
```

---

## Testing Strategy

### Test Levels

1. **Unit Tests** (80%+ coverage target)
   - Crypto functions
   - Storage operations
   - Utility functions

2. **Integration Tests**
   - API endpoints
   - Database operations
   - P2P message handling

3. **Manual Tests**
   - End-to-end workflows
   - P2P device pairing
   - UI interactions

### Critical Paths to Test

- Vault initialization
- Password encryption/decryption
- Device approval
- P2P sync
- Vault switching
- Error handling

See TESTING.md for full test suite.

---

## Deployment

### Current
- Local binary execution
- Server runs on localhost
- No remote deployment

### Future
- Signed binaries
- Package managers (Homebrew, apt, etc.)
- Mobile app stores
- Browser extension stores

---

## Key Decisions

### 1. Why Go for Backend?
- Strong crypto libraries
- Fast, compiled binary
- Easy deployment
- Good SQLite support

### 2. Why Tauri for Desktop?
- Smaller bundle than Electron
- Native Rust performance
- Secure by default
- Modern web stack

### 3. Why P2P Instead of Cloud?
- Zero-knowledge guarantee
- No server infrastructure
- User owns data
- Works offline

### 4. Why RSA-4096?
- Industry standard for asymmetric
- Well-vetted, widely supported
- Good performance/security balance

---

## Resources

### Documentation
- ARCHITECTURE.md - Technical design
- TESTING.md - Testing guide
- SECURITY_REMEDIATION_PLAN.md - Security fixes
- CODER_AGENT.md - AI coding guidelines
- USER_GUIDE.md - End user manual

### External References
- [OWASP Password Storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)
- [NIST SP 800-63B](https://pages.nist.gov/800-63-3/sp800-63b.html)
- [libp2p Specs](https://github.com/libp2p/specs)

---

## Next Milestones

### Week 1-2: Security Hardening
- Fix 5 CRITICAL vulnerabilities
- Fix 5 HIGH vulnerabilities
- Security audit re-test

### Week 3-4: Quality Improvements
- Add comprehensive tests
- Fix race conditions
- Optimize performance

### Month 2: Feature Complete
- Remote P2P via relay
- Import from 1Password/Bitwarden
- Password generator improvements

### Month 3: Polish
- UI/UX improvements
- Documentation
- Beta release

---

**Questions?** Check the specific docs or ask the team.
