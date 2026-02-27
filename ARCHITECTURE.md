# Password Manager - Architecture Document

## Executive Summary

A cross-platform, multi-device password manager with Tauri desktop frontend and Go backend. Uses hybrid encryption (AES-256-GCM + RSA) with Git-based synchronization between devices.

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         SYSTEM OVERVIEW                                 │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   ┌─────────────┐     ┌─────────────┐     ┌─────────────┐              │
│   │   Tauri     │────▶│   Go API    │────▶│   SQLite    │              │
│   │  Frontend  │     │   Server    │     │  Database   │              │
│   │  (React)   │◀────│  (port      │◀────│  (vault.db) │              │
│   └─────────────┘     │   18475)    │     └─────────────┘              │
│                       └─────────────┘                                    │
│                              │                                           │
│                              ▼                                           │
│                       ┌─────────────┐                                    │
│                       │   Git Sync  │                                    │
│                       │  (optional) │                                    │
│                       └─────────────┘                                    │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Data Storage

### Location
- **Linux/macOS**: `~/.pwman/`
- **Windows**: `%APPDATA%/pwman/`

### Files
| File | Purpose |
|------|---------|
| `vault.db` | SQLite database with encrypted passwords |
| `config.json` | Device configuration |
| `private.key.enc` | RSA private key **encrypted with password** |
| `private.key` | (DEPRECATED - was unencrypted) |
| `public.key` | RSA public key |
| `.git/` | Git repository for sync (if enabled) |

---

## Encryption Model

### Hybrid Encryption Flow
```
┌──────────────────────────────────────────────────────────────────────────┐
│                         HYBRID ENCRYPTION                                 │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  PASSWORD ENCRYPTION:                                                    │
│  ┌──────────┐     ┌───────────────┐     ┌─────────────────────┐         │
│  │ Password │────▶│ AES-256-GCM   │────▶│ Encrypted Password  │         │
│  │ (plain)  │     │ (random key)  │     │ (stored in DB)      │         │
│  └──────────┘     └───────────────┘     └─────────────────────┘         │
│                                                                          │
│  KEY ENCRYPTION (per device):                                            │
│  ┌──────────┐     ┌───────────────┐     ┌─────────────────────┐         │
│  │ AES Key │────▶│ RSA Encrypt   │────▶│ Encrypted AES Key   │         │
│  │         │     │ (device pubkey)│     │ (stored per device)│         │
│  └──────────┘     └───────────────┘     └─────────────────────┘         │
│                                                                          │
│  STORAGE FORMAT: {                                                       │
│    encrypted_password: "base64(AES-256-GCM(...))",                       │
│    encrypted_aes_keys: {                                                 │
│      "arch-desktop-fingerprint": "base64(RSA(...))",                   │
│      "macbook-fingerprint": "base64(RSA(...))"                         │
│    }                                                                     │
│  }                                                                       │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

### Vault Password Protection
```
┌──────────────────────────────────────────────────────────────────────────┐
│                     VAULT PASSWORD PROTECTION                             │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  INITIALIZATION (pwman init --name "Device"):                           │
│  1. Generate RSA-4096 key pair                                          │
│  2. Derive key from password using scrypt (KDF)                         │
│  3. Encrypt private key with AES-256-GCM using derived key             │
│  4. Store encrypted private key as private.key.enc                      │
│  5. Store public key as public.key                                       │
│                                                                          │
│  UNLOCK (pwman unlock / frontend):                                       │
│  1. User enters password                                                 │
│  2. Derive key from password (same scrypt params)                       │
│  3. Try to decrypt private.key.enc                                       │
│  4. If success → vault unlocked; If fail → wrong password               │
│                                                                          │
│  SECURITY:                                                              │
│  - Password never stored                                                 │
│  - Private key encrypted at rest                                        │
│  - Wrong password = decryption fails (constant-time check)              │
│  - Auto-lock after inactivity (configurable)                            │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## Data Model

### Device
```go
type Device struct {
    ID          string    `json:"id"`           // UUID
    Name        string    `json:"name"`         // "Arch Desktop", "MacBook Pro"
    PublicKey   string    `json:"public_key"`   // RSA public key (PEM)
    Fingerprint string    `json:"fingerprint"`  // SHA256 of public key (hex)
    CreatedAt   time.Time `json:"created_at"`
    Trusted     bool      `json:"trusted"`      // Approved by existing device
    ApprovalCode string   `json:"approval_code"` // One-time code for device approval
}
```

### PasswordEntry
```go
type PasswordEntry struct {
    ID                string            `json:"id"`
    Version           int64             `json:"version"`
    Site              string            `json:"site"`
    Username          string            `json:"username"`
    EncryptedPassword string            `json:"encrypted_password"`
    EncryptedAESKeys  map[string]string `json:"encrypted_aes_keys"` // fingerprint -> encrypted key
    Notes             string            `json:"notes"`
    CreatedAt         time.Time         `json:"created_at"`
    UpdatedAt         time.Time         `json:"updated_at"`
    UpdatedBy         string            `json:"updated_by"`    // device_id
    DeletedAt         *time.Time        `json:"deleted_at"`    // soft delete
}
```

---

## Database Schema (SQLite)

```sql
-- Device registry
CREATE TABLE devices (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    public_key TEXT NOT NULL,
    fingerprint TEXT UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    trusted BOOLEAN DEFAULT FALSE,
    approval_code TEXT
);

-- Password entries
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

CREATE INDEX idx_entries_site ON entries(site);
CREATE INDEX idx_entries_updated ON entries(updated_at);
```

---

## Multi-Device Flow

### Device Registration & Approval

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    MULTI-DEVICE APPROVAL FLOW                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  EXISTING DEVICE (Arch)              NEW DEVICE (MacBook)             │
│  ────────────────────                ─────────────────                 │
│                                                                         │
│  1. Initialize vault:                                                 │
│     pwman init --name "Arch"                                           │
│     → Creates vault, generates keys, encrypts private key             │
│     → Device marked as trusted                                         │
│                                                                         │
│  2. Add passwords:                                                     │
│     pwman add github.com --user me                                     │
│     → Encrypts for Arch's public key                                  │
│                                                                         │
│  ──────────────────────────────────────────────────────────────────    │
│                                                                         │
│  3. Enable sync:                                                       │
│     pwman sync init --remote git@github.com:you/pwman-sync.git        │
│     pwman sync push                                                    │
│                                                                         │
│  ──────────────────────────────────────────────────────────────────    │
│                                                                         │
│  4. On NEW device (MacBook):                                           │
│     pwman init --name "MacBook"                                        │
│     → Creates own vault with own keys                                  │
│                                                                         │
│  5. Export public key:                                                 │
│     pwman devices export > macbook.pub                                 │
│                                                                         │
│  ──────────────────────────────────────────────────────────────────    │
│                                                                         │
│  6. Copy macbook.pub to Arch, then:                                    │
│     pwman devices add ./macbook.pub                                   │
│     → Adds MacBook as UNTRUSTED device                                 │
│     → Generates one-time APPROVAL CODE: "ABC123"                       │
│     → Shows approval code to user                                      │
│                                                                         │
│  7. Copy approval code to MacBook                                      │
│                                                                         │
│  8. On MacBook, approve itself:                                        │
│     pwman devices approve ABC123                                       │
│     → Marks MacBook as TRUSTED                                         │
│     → Triggers re-encryption of ALL passwords                          │
│     → Now has encrypted AES keys for MacBook                          │
│                                                                         │
│  9. On Arch, push updated vault:                                       │
│     pwman sync push                                                    │
│     → Pushes vault.db with MacBook's encrypted keys                    │
│                                                                         │
│  10. On MacBook, pull:                                                 │
│      pwman sync pull                                                   │
│      → Can now decrypt all passwords!                                  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Adding Password (Multi-Device)
```
pwman add github.com --username user

1. Generate random 32-byte AES key
2. Encrypt password with AES-256-GCM
3. For each trusted device in DB:
   - Encrypt AES key with device's RSA public key
4. Store entry with all encrypted AES keys
5. On next sync, push to all devices
```

---

## Git-Based Sync

### Repository Structure
```
pwman-sync/                    # Git remote repository
├── vault.db                   # SQLite database (encrypted contents)
├── public.key                 # Current device's public key (for new devices)
├── devices/                   # Device public keys
│   ├── arch-desktop.json
│   └── macbook-pro.json
└── .gitignore                 # Ignore private keys
```

### Sync Flow
```
┌─────────────────────────────────────────────────────────────────────────┐
│                           SYNC PROCESS                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  1. git pull (fetch remote changes)                                     │
│                                                                         │
│  2. Detect conflicts:                                                   │
│     - Compare local vs remote entries by (id, version, updated_at)     │
│     - If same entry modified on both: last-write-wins                  │
│                                                                         │
│  3. Merge:                                                              │
│     - Apply remote changes to local DB                                  │
│     - Keep local changes with higher updated_at                         │
│                                                                         │
│  4. Check for new devices:                                             │
│     - If new device added remotely, prompt to approve                   │
│     - Re-encrypt AES keys for new trusted devices                       │
│                                                                         │
│  5. git add + git commit + git push                                     │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Conflict Resolution
| Scenario | Resolution |
|----------|------------|
| Entry modified on one device | Accept the change |
| Entry modified on both devices | Last-write-wins (higher `updated_at`) |
| Entry deleted on one, modified on other | Keep deletion |
| New device added | Prompt for approval, then re-encrypt keys |

---

## Frontend (Tauri/React)

### Component Architecture
```
┌─────────────────────────────────────────────────────────────────────────┐
│                        FRONTEND COMPONENTS                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  App.tsx                                                                │
│  ├── SetupScreen (if not initialized)                                   │
│  │   └── Device name input → init vault                                 │
│  │                                                                       │
│  ├── UnlockScreen (if initialized but locked)                          │
│  │   └── Password input → unlock vault                                 │
│  │                                                                       │
│  └── MainScreen (if unlocked)                                           │
│      ├── Header                                                        │
│      │   └── Lock button                                                │
│      │                                                                  │
│      ├── PasswordList                                                  │
│      │   ├── SearchBar                                                 │
│      │   └── PasswordCard[]                                           │
│      │       ├── Site/Username                                         │
│      │       ├── [Copy] button                                         │
│      │       ├── [Show/Hide] toggle                                    │
│      │       └── [Edit] [Delete]                                       │
│      │                                                                  │
│      └── Settings                                                      │
│          ├── Device info                                               │
│          ├── Devices list                                              │
│          └── Sync status                                               │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Clipboard Flow
```
1. User clicks "Copy" button on password card
2. Frontend calls Tauri command get_decrypted_password(id)
3. Tauri calls Go API → decrypts password
4. Frontend copies to system clipboard via Tauri clipboard API
5. Frontend shows toast: "Password copied (clears in 30s)"
6. Frontend sets timer to clear clipboard after 30 seconds
```

---

## CLI Commands

```bash
# Vault management
pwman init --name "Device Name"          # Initialize new vault
pwman unlock                             # Unlock vault (prompts for password)
pwman lock                               # Lock vault
pwman status                             # Show vault status

# Password management
pwman add <site> --user <username>       # Add password (prompts for password)
pwman add <site> --user <username> --password "secret"
pwman add <site> --user <username> --generate  # Auto-generate password
pwman get <site>                         # Show password
pwman get <site> -c, --clipboard         # Copy to clipboard
pwman list                               # List all entries
pwman list --search "github"             # Search entries
pwman edit <site>                        # Edit entry
pwman delete <site>                      # Delete entry (soft delete)

# Device management  
pwman devices list                       # List devices
pwman devices export                     # Export this device's public key
pwman devices add <file>                 # Add new device (creates untrusted)
pwman devices approve <code>             # Approve this device (generate keys)
pwman devices remove <device-id>         # Remove device

# Sync
pwman sync init --remote <url>           # Initialize sync with remote
pwman sync push                          # Push changes to remote
pwman sync pull                          # Pull changes from remote
pwman sync status                        # Show sync status

# Import
pwman import --from-cpp --db <postgres>  # Import from C++ implementation
```

---

## Dependencies

```go
module github.com/bok1c4/pwman

go 1.23

require (
    github.com/spf13/cobra v1.8.0
    github.com/ProtonMail/gopenpgp/v3 v3.0.0
    github.com/mattn/go-sqlite3 v1.14.22
    github.com/go-git/go-git/v5 v5.12.0
    golang.org/x/crypto v0.25.0
    github.com/atotto/clipboard v0.1.4
    github.com/google/uuid v1.6.0
)
```

---

## Sync Workflow & Device Approval

### Overview

The password manager uses Git-based synchronization to share vaults between devices. When a new device wants to join an existing vault, it must be approved by an already-trusted device.

### Sync Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        SYNC ARCHITECTURE                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   Device A (Trusted)              Git Remote              Device B (New)   │
│   ───────────────────            ───────────              ─────────────────  │
│        │                           │                           │             │
│        │  1. Enable Sync           │                           │             │
│        │ ────────────────────────▶ │                           │             │
│        │                           │                           │             │
│        │  2. Push vault            │                           │             │
│        │ ────────────────────────▶ │                           │             │
│        │                           │                           │             │
│        │                           │  3. Clone/Pull            │             │
│        │                           │ ◀──────────────────────────│             │
│        │                           │                           │             │
│        │                           │  4. Can't decrypt yet!     │             │
│        │                           │   (needs approval)         │             │
│        │                           │                           │             │
│        │  5. See pending          │                           │             │
│        │    approval request      │                           │             │
│        │ ◀────────────────────────│                           │             │
│        │                           │                           │             │
│        │  6. Approve/Decline      │                           │             │
│        │ ────────────────────────▶ │                           │             │
│        │                           │                           │             │
│        │  7. Push updated vault   │                           │             │
│        │    (with new device     │                           │             │
│        │     encrypted keys)     │                           │             │
│        │ ────────────────────────▶ │                           │             │
│        │                           │                           │             │
│        │                           │  8. Pull                  │             │
│        │                           │ ◀──────────────────────────│             │
│        │                           │                           │             │
│        │                           │  9. Now can decrypt!      │             │
│        │                           │                           │             │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Device Approval Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     DEVICE APPROVAL WORKFLOW                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  NEW DEVICE (Device B)              TRUSTED DEVICE (Device A)             │
│  ─────────────────────              ───────────────────────             │
│                                                                             │
│  1. Initialize vault                                                               │
│     pwman init --name "DeviceB"                                               │
│     → Creates own keys, empty vault                                           │
│                                                                             │
│                                    2. Export public key                      │
│                                         pwman devices export > deviceb.pub   │
│                                                                             │
│  3. Add device (on Device A)                                                │
│     pwman devices add deviceb.pub                                            │
│     → Device B added as UNTRUSTED                                            │
│     → Generates approval code: ABC123                                         │
│                                                                             │
│                                    4. User sees pending approval              │
│                                       in Settings                            │
│                                                                             │
│  5. Approve (on Device B)                                                   │
│     pwman devices approve ABC123                                              │
│     → Marks self as TRUSTED                                                  │
│     → Re-encrypts ALL passwords                                              │
│       with new AES keys for all trusted devices                              │
│                                                                             │
│                                    6. Push updated vault                    │
│                                         pwman sync push                      │
│                                                                             │
│  7. Pull vault                                                              │
│     pwman sync pull                                                          │
│     → Now has encrypted AES keys                                            │
│     → Can decrypt all passwords!                                             │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Git Sync Implementation

The sync is implemented using Git to push/pull the vault database:

```go
// Sync operations in internal/sync/sync.go
type GitSync struct {
    repo      *gogit.Repository
    vaultName string
    cfg       *config.Config
    remote    string
}

// Key operations:
// - InitRepo(vaultPath)     : Initialize git repo in vault
// - SetRemote(url)         : Configure remote repository
// - Pull()                  : Pull changes from remote
// - Push()                  : Push changes to remote  
// - CommitAndPush(message)  : Commit and push all changes
```

### Can Git Sync Support Device Approval?

**Yes, but with limitations:**

| Feature | Git Sync Support | Notes |
|---------|-----------------|-------|
| Share encrypted vault | ✅ Full | Database is encrypted, safe to push |
| Device discovery | ⚠️ Manual | Must exchange public keys manually |
| Approval requests | ⚠️ Manual | User must communicate approval code |
| Auto-approve | ❌ Not possible | Git has no real-time notifications |
| Conflict detection | ✅ Full | Git handles merge conflicts |
| Offline support | ✅ Full | Works without network until push |

**Current Workflow:**
1. User manually exchanges public keys (file transfer)
2. Existing device adds new device → gets approval code
3. User communicates code to new device (via chat/email)
4. New device approves itself → re-encryption happens
5. Vault pushed → new device pulls → can decrypt

**Limitation:** The Git-based sync doesn't support real-time approval requests. Users must manually:
- Export/import public keys
- Communicate approval codes
- Know when to sync

**Alternative for Real-time Approvals:**
- Could add a simple signaling server (WebSocket)
- Or use GitHub/GitLab issues as approval channel
- Or manual process as currently implemented

---

## P2P Sync (Future)

### Overview

Peer-to-peer (P2P) sync enables direct communication between devices without any intermediate server. It uses libp2p for NAT traversal and encrypted connections.

### Architecture

```
┌────────────────────────────────────────────────────────────────────────┐
│                         P2P SYNC ARCHITECTURE                         │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  ┌─────────────┐       ┌─────────────┐       ┌─────────────┐        │
│  │   Device A  │       │   Device B  │       │   Device C  │        │
│  │  (Laptop)   │◀─────▶│  (Desktop)  │◀─────▶│   (Phone)   │        │
│  └──────┬──────┘       └──────┬──────┘       └──────┬──────┘        │
│         │                     │                     │                │
│         │         P2P Network (mesh)                │                │
│         │                     │                     │                │
│  ┌──────▼─────────────────────▼─────────────────────▼──────┐        │
│  │                    P2P Manager                           │        │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │        │
│  │  │  Discovery  │  │  Signaling  │  │   Sync     │     │        │
│  │  │  (mDNS/    │  │  (STUN/    │  │  Protocol   │     │        │
│  │  │   DNS-SD)  │  │   TURN/ICE)│  │             │     │        │
│  │  └─────────────┘  └─────────────┘  └─────────────┘     │        │
│  └────────────────────────────────────────────────────────────┘        │
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘
```

### NAT Traversal

```
┌─────────────────────────────────────────────────────────────────┐
│                     NAT TRAVERSAL FLOW                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Device A (behind NAT)          Device B (behind NAT)           │
│  ─────────────────────          ─────────────────────             │
│       │                               │                          │
│       │  1. Both connect to          │                          │
│       │     Public STUN server       │                          │
│       │     (free, public)           │                          │
│       │───────────                   │                          │
│       │           ───────────────────│─────────                 │
│       │                               │                          │
│       │  2. Get external IP:port     │                          │
│       │     via STUN                 │                          │
│       │                               │                          │
│       │  3. Hole punching            │                          │
│       │     (both try to connect)    │                          │
│       │                               │                          │
│       │  4. Direct P2P connection!  │                          │
│       │◀────────────────────────────▶│                          │
│       │                               │                          │
└─────────────────────────────────────────────────────────────────┘
```

### P2P Protocol Messages

| Message | Direction | Purpose |
|---------|-----------|---------|
| HELLO | Bidirectional | Initial handshake, exchange pubkeys |
| REQUEST_SYNC | Bidirectional | Request vault sync |
| SYNC_DATA | Bidirectional | Encrypted vault data |
| REQUEST_APPROVAL | New → Trusted | Request device approval |
| APPROVE_DEVICE | Trusted → New | Approve with encrypted keys |
| REJECT_DEVICE | Trusted → New | Reject device |
| PING/PONG | Bidirectional | Keep-alive |

### Device Approval in P2P

```
┌─────────────────────────────────────────────────────────────────┐
│              P2P DEVICE APPROVAL FLOW                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Device A (trusted)          Device B (new)                    │
│  ────────────────            ───────────────                   │
│       │                           │                              │
│       │  1. Connect P2P          │                              │
│       │◀─────────────────────────▶│                             │
│       │                           │                              │
│       │  2. Send pubkey         │                              │
│       │◀─────────────────────────▶│                             │
│       │                           │                              │
│       │  3. Show approval       │                              │
│       │    request in UI         │                              │
│       │                           │                              │
│  User clicks "Approve"          │                              │
│       │                           │                              │
│       │  4. Re-encrypt all       │                              │
│       │    passwords             │                              │
│       │                           │                              │
│       │  5. Send encrypted      │                              │
│       │    keys                 │                              │
│       │◀─────────────────────────▶│                             │
│       │                           │                              │
│       │  6. Can decrypt!        │                              │
│       │                           │                              │
└─────────────────────────────────────────────────────────────────┘
```

### Comparison: Git vs P2P

| Feature | Git Sync | P2P Sync |
|---------|----------|----------|
| Server needed | Git host | No |
| Real-time | ❌ | ✅ |
| NAT traversal | N/A | ✅ libp2p |
| Device approval | Manual | Auto |
| Offline support | ✅ | ❌ |
| Version history | ✅ | ❌ |
| Complexity | Low | High |
| Setup time | 5 min | 30 min |

### Dependencies (P2P)

```go
require (
    github.com/libp2p/go-libp2p v0.32.0
    github.com/libp2p/go-libp2p-pubsub v0.9.0
    github.com/libp2p/go-libp2p-kad-dht v0.24.0
    github.com/libp2p/go-mdns v0.0.3
    github.com/libp2p/go-relay-client v0.1.0
)
```

---

## Security Considerations

| Threat | Mitigation |
|--------|------------|
| Server/Git breach | Zero-knowledge - all data encrypted |
| Device theft | Private key encrypted with password (scrypt + AES-256-GCM) |
| Brute force | Strong KDF (scrypt) - ~100ms to derive key |
| Database leak | Passwords encrypted with AES-256-GCM |
| Clipboard theft | Auto-clear after 30 seconds |
| P2P eavesdropping | All P2P traffic uses Noise protocol |
| MITM attacks | Peer fingerprint verification |

---

## Future Phases

### Phase 2: Enhanced Sync
- Real-time sync notifications
- Conflict resolution UI
- Selective sync (choose which entries to sync)

### Phase 3: Mobile
- iOS/Android native apps
- Biometric unlock (Face ID / Fingerprint)
- Push notifications

### Phase 4: Web
- Web-based access via PWA
- Browser extension
