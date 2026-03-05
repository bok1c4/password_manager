# Architecture Document - Password Manager

**Version**: 2.0  
**Last Updated**: 2026-03-05  
**Status**: Security Hardening Phase

---

## Overview

A secure, cross-platform password manager with P2P synchronization. Uses hybrid encryption (AES-256-GCM + RSA) with no cloud dependency.

### Key Principles

1. **Zero Knowledge**: We can't decrypt your passwords
2. **User Ownership**: Data stays on your devices
3. **No Cloud Required**: P2P sync works offline
4. **Defense in Depth**: Multiple layers of security

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         SYSTEM OVERVIEW                                 │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   ┌─────────────┐     ┌─────────────┐     ┌─────────────┐              │
│   │   Desktop   │────▶│   Go API    │────▶│   SQLite    │              │
│   │   (Tauri)   │     │   Server    │     │  Database   │              │
│   │  React + TS │◀────│  (Auth)     │◀────│  (Vault)    │              │
│   └─────────────┘     └──────┬──────┘     └─────────────┘              │
│                              │                                          │
│                              ▼                                          │
│   ┌─────────────┐     ┌─────────────┐                                  │
│   │    CLI      │     │    P2P      │                                  │
│   │  (Cobra)    │     │  (libp2p)   │                                  │
│   └─────────────┘     └──────┬──────┘                                  │
│                              │                                          │
│                              ▼                                          │
│                        ┌─────────────┐                                 │
│                        │Other Device │                                 │
│                        └─────────────┘                                 │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Component Architecture

### 1. Desktop Application (Tauri + React)

**Technology Stack**:
- **Frontend**: React 18, TypeScript, Tailwind CSS
- **Desktop**: Tauri v2 (Rust-based)
- **State**: Zustand
- **Routing**: React Router v7

**Responsibilities**:
- User interface (vault unlock, password list, settings)
- Clipboard operations
- API communication with Go server
- P2P status display

**Security**:
- Runs in sandboxed WebView
- Communicates with backend via HTTP API
- No direct file system access

### 2. Go API Server

**Technology Stack**:
- **Language**: Go 1.25+
- **HTTP**: Standard library
- **Database**: SQLite
- **Crypto**: Standard library + golang.org/x/crypto

**Responsibilities**:
- Vault lifecycle (init, unlock, lock)
- Password CRUD operations
- Device management
- P2P orchestration
- Authentication (token-based)

**Architecture Pattern**: Handler-based HTTP API

```go
// Middleware chain
http.HandleFunc("/api/entries", 
    corsHandler(
        authMiddleware(
            rateLimitMiddleware(
                handleGetEntries
            )
        )
    )
)
```

### 3. CLI Application

**Technology Stack**:
- **Framework**: Cobra
- **Input**: Terminal (passwords via secure prompt)

**Responsibilities**:
- Scriptable operations
- Quick command-line access
- Import/export tools

### 4. P2P Network Layer

**Technology Stack**:
- **Library**: libp2p (go-libp2p)
- **Transport**: TCP
- **Security**: Noise protocol
- **Discovery**: mDNS (LAN)

**Responsibilities**:
- Device discovery (LAN only)
- Encrypted connections
- Message routing
- Sync protocol

**Message Types**:
- `HELLO` - Initial handshake
- `PAIRING_REQUEST` - Device pairing
- `PAIRING_RESPONSE` - Pairing approval/rejection
- `REQUEST_SYNC` - Request vault data
- `SYNC_DATA` - Transfer entries/devices
- `ENTRY_UPDATE` - Real-time updates

### 5. Storage Layer

**Technology Stack**:
- **Database**: SQLite 3
- **Driver**: github.com/mattn/go-sqlite3

**Schema**:
```sql
-- Devices table
CREATE TABLE devices (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    public_key TEXT NOT NULL,
    fingerprint TEXT UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    trusted BOOLEAN DEFAULT FALSE
);

-- Entries table
CREATE TABLE entries (
    id TEXT PRIMARY KEY,
    version INTEGER NOT NULL DEFAULT 1,
    site TEXT NOT NULL,
    username TEXT,
    encrypted_password TEXT NOT NULL,
    notes TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_by TEXT REFERENCES devices(id),
    deleted_at TIMESTAMP
);

-- Encrypted AES keys (one per device per entry)
CREATE TABLE encrypted_keys (
    entry_id TEXT REFERENCES entries(id) ON DELETE CASCADE,
    device_fingerprint TEXT NOT NULL,
    encrypted_aes_key TEXT NOT NULL,
    PRIMARY KEY (entry_id, device_fingerprint)
);

-- Vault metadata
CREATE TABLE vault_meta (
    key TEXT PRIMARY KEY,
    value TEXT
);
```

### 6. Cryptography Layer

**Technology Stack**:
- **Standard Library**: crypto/aes, crypto/rsa, crypto/cipher
- **KDF**: scrypt (golang.org/x/crypto/scrypt)
- **Encoding**: Base64, PEM

**Algorithms**:
- **Symmetric**: AES-256-GCM
- **Asymmetric**: RSA-4096 (upgraded from 2048)
- **Key Derivation**: scrypt (N=32768, r=8, p=1)

**Key Hierarchy**:
```
Vault Password
      │
      ▼ (scrypt)
Encryption Key (32 bytes)
      │
      ▼ (AES-256-GCM)
Private Key File (encrypted)
      │
      ▼ (RSA Decrypt)
AES Keys (per entry)
      │
      ▼ (AES-256-GCM Decrypt)
Passwords (plaintext)
```

---

## Data Flow

### Adding a Password

```
1. User enters password in UI
   ↓
2. Frontend sends POST /api/entries/add
   ↓
3. Server generates random AES key (32 bytes)
   ↓
4. Server encrypts password with AES-256-GCM
   ↓
5. For each trusted device:
   - Load device's public key
   - Encrypt AES key with RSA-OAEP
   - Store encrypted key in database
   ↓
6. Store encrypted password + all encrypted AES keys
   ↓
7. Broadcast ENTRY_UPDATE to connected peers
   ↓
8. Return success to frontend
```

### Retrieving a Password

```
1. User clicks "Show Password" in UI
   ↓
2. Frontend sends POST /api/entries/get_password
   ↓
3. Server retrieves entry from database
   ↓
4. Server gets this device's fingerprint from public key
   ↓
5. Server looks up encrypted AES key for this fingerprint
   ↓
6. Server decrypts AES key with RSA private key
   ↓
7. Server decrypts password with AES key
   ↓
8. Server returns plaintext password to frontend
   ↓
9. Frontend displays password (or copies to clipboard)
```

### P2P Device Pairing

```
Device A (Generator)          Device B (Joiner)
────────────────────          ────────────────
1. Generate pairing code
   (9 characters, expires in 5 min)
   ↓
2. Start P2P (mDNS discovery)
                               3. Initialize vault
                               4. Start P2P
                               5. Call POST /api/pairing/join
                                  with code
                                  ↓
6. Receive PAIRING_REQUEST
   with joiner's public key
   ↓
7. Validate pairing code
   ↓
8. Add joiner as trusted device
   ↓
9. Re-encrypt all passwords
   for both devices
   ↓
10. Send PAIRING_RESPONSE
    with approval
                               11. Receive approval
                               12. Send REQUEST_SYNC
                                  ↓
13. Send SYNC_DATA
    (all entries + devices)
                               14. Receive and store data
                               15. Can now decrypt passwords!
```

---

## Security Architecture

### Threat Model

**Assets to Protect**:
- User passwords (highest priority)
- Private keys
- Vault metadata
- Sync communications

**Threat Actors**:
1. **Local attacker** - Other processes on same machine
2. **Network attacker** - MITM, eavesdropping
3. **Malicious peer** - Untrusted device trying to join
4. **Physical attacker** - Device theft

### Security Controls

| Threat | Control | Implementation |
|--------|---------|----------------|
| Local process access | API authentication | Bearer token required |
| CSRF/XSS from web | CORS restriction | Origin whitelist |
| Brute force password | Rate limiting | 1 req/sec for unlock |
| Private key theft | Encryption at rest | AES-256-GCM with scrypt |
| Network eavesdropping | P2P encryption | Noise protocol |
| Device theft | Password required | Private key encrypted |
| Clipboard theft | Auto-clear | Clears after 30s |
| SQL injection | Parameterized queries | No string concatenation |

### Authentication Flow

```
1. Client: POST /api/unlock {password: "..."}
   ↓
2. Server: Derive key from password + salt
   ↓
3. Server: Try to decrypt private.key.enc
   ↓
4. If success:
      - Load vault
      - Generate auth token
      - Return {token: "..."}
   If fail:
      - Return 401 Unauthorized
   ↓
5. Client: Store token, use in all requests
   ↓
6. All subsequent requests:
      Client: GET /api/entries
      Headers: Authorization: Bearer <token>
   ↓
7. Server: Validate token
   ↓
8. If valid: Process request
   If invalid/expired: Return 401
```

### Encryption Details

**AES-256-GCM**:
- Key size: 32 bytes (256 bits)
- Nonce: 12 bytes (random per encryption)
- Tag: 16 bytes (authentication)
- Output: nonce + ciphertext + tag (Base64 encoded)

**RSA-4096**:
- Key size: 4096 bits (upgrade from 2048)
- Padding: PKCS#1v15 (legacy) → OAEP (future)
- Max plaintext: ~470 bytes (for key encryption)

**Scrypt**:
- N: 32768 (CPU/memory cost)
- r: 8 (block size)
- p: 1 (parallelization)
- Output: 32 bytes

---

## Multi-Vault Support

### Directory Structure

```
~/.pwman/
├── config.json              # Global config (vault list, active)
└── vaults/
    ├── personal/
    │   ├── config.json      # Vault-specific config
    │   ├── vault.db         # SQLite database
    │   ├── private.key.enc  # Encrypted private key
    │   └── public.key       # Public key
    └── work/
        ├── config.json
        ├── vault.db
        ├── private.key.enc
        └── public.key
```

### Isolation

Each vault has:
- Separate database
- Separate key pair
- Separate device registry
- No shared state

Switching vaults:
1. Close current vault (lock)
2. Update global config (active_vault)
3. Open new vault (requires unlock)

---

## Configuration

### Global Config (~/.pwman/config.json)

```json
{
  "active_vault": "personal",
  "vaults": ["personal", "work"]
}
```

### Vault Config (~/.pwman/vaults/<name>/config.json)

```json
{
  "device_id": "uuid",
  "device_name": "My Device",
  "salt": "base64..."
}
```

### Environment Variables

- `PWMAN_PORT` - API server port (default: random)
- `PWMAN_BASE_PATH` - Override vault directory
- `PWMAN_LOG_LEVEL` - Logging verbosity

---

## API Endpoints

### Authentication
- `POST /api/unlock` - Unlock vault, get token
- `POST /api/lock` - Lock vault, invalidate token
- `GET /api/is_unlocked` - Check vault status

### Vault Management
- `GET /api/vaults` - List vaults
- `POST /api/vaults/use` - Switch vault
- `POST /api/vaults/create` - Create new vault
- `POST /api/vaults/delete` - Delete vault

### Password Entries
- `GET /api/entries` - List all entries
- `POST /api/entries/add` - Add entry
- `POST /api/entries/update` - Update entry
- `POST /api/entries/delete` - Soft delete entry
- `POST /api/entries/get_password` - Decrypt password

### Devices
- `GET /api/devices` - List devices
- `POST /api/p2p/approve` - Approve device
- `POST /api/p2p/reject` - Reject device

### P2P
- `GET /api/p2p/status` - P2P status
- `POST /api/p2p/start` - Start P2P
- `POST /api/p2p/stop` - Stop P2P
- `GET /api/p2p/peers` - List peers
- `POST /api/p2p/connect` - Connect to peer
- `POST /api/p2p/disconnect` - Disconnect peer

### Pairing
- `POST /api/pairing/generate` - Generate pairing code
- `POST /api/pairing/join` - Join with code
- `GET /api/pairing/status` - Check pairing status

---

## Error Handling

### Error Categories

1. **Client Errors (4xx)**:
   - 400 Bad Request - Invalid input
   - 401 Unauthorized - Invalid/missing token
   - 403 Forbidden - Not permitted
   - 404 Not Found - Resource doesn't exist
   - 429 Too Many Requests - Rate limited

2. **Server Errors (5xx)**:
   - 500 Internal Server Error - Unexpected error
   - 503 Service Unavailable - Vault locked

### Error Response Format

```json
{
  "success": false,
  "error": "human readable message",
  "code": "ERROR_CODE"  // For programmatic handling
}
```

### Error Codes

- `VAULT_LOCKED` - Vault needs unlock
- `INVALID_PASSWORD` - Wrong password
- `DEVICE_NOT_FOUND` - Unknown device
- `ENTRY_NOT_FOUND` - Unknown entry
- `RATE_LIMITED` - Too many requests
- `PAIRING_EXPIRED` - Code expired
- `PAIRING_INVALID` - Wrong code

---

## Performance Considerations

### Optimization Strategies

1. **Database**:
   - Indexes on frequently queried columns
   - Connection pooling
   - Prepared statements

2. **Crypto**:
   - Cache derived keys (briefly)
   - Batch operations where possible
   - Async P2P operations

3. **Network**:
   - Lazy loading of entries
   - Compression for P2P sync
   - Incremental sync

### Benchmarks

Target performance:
- Unlock: <500ms
- Add entry: <100ms
- Get password: <50ms
- P2P sync (100 entries): <5s

---

## Future Architecture

### Planned Enhancements

1. **Remote Sync**:
   - Relay server for NAT traversal
   - Tor onion services
   - QR code pairing

2. **Mobile**:
   - iOS/Android native apps
   - Biometric authentication
   - Push notifications

3. **Browser Extension**:
   - Autofill integration
   - Password generation
   - Form detection

4. **Cloud Backup** (Optional):
   - End-to-end encrypted backups
   - User-controlled encryption keys
   - Optional, disabled by default

---

## Development Guidelines

### Adding New Features

1. Update data models (pkg/models/)
2. Add storage methods (internal/storage/)
3. Add crypto functions (internal/crypto/)
4. Add API handlers (internal/api/)
5. Add CLI commands (internal/cli/)
6. Add frontend components (src/components/)
7. Write tests for all layers
8. Update documentation

### Security Checklist

- [ ] No passwords in logs
- [ ] No secrets in error messages
- [ ] Input validation on all boundaries
- [ ] Rate limiting on sensitive endpoints
- [ ] Authentication required
- [ ] CORS properly configured
- [ ] SQL injection prevented (parameterized queries)
- [ ] Race conditions checked (`go test -race`)

---

**Related Documents**:
- CONTEXT.md - Project overview
- TESTING.md - Testing guide
- SECURITY_REMEDIATION_PLAN.md - Security fixes
- CODER_AGENT.md - AI coding guidelines
