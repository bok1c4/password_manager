# Password Manager - Architecture Document

## Executive Summary

A cross-platform, multi-device password manager with Tauri desktop frontend and Go backend. Uses hybrid encryption (AES-256-GCM + RSA) with P2P-based synchronization between devices via libp2p.

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
│                       │    P2P      │                                    │
│                       │   Sync      │                                    │
│                       │  (libp2p)   │                                    │
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
│                    MULTI-DEVICE APPROVAL FLOW (P2P)                     │
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
│  3. Start P2P on Arch:                                                │
│     pwman p2p start                                                   │
│     → Listens on TCP + mDNS discovery                                 │
│                                                                         │
│  ──────────────────────────────────────────────────────────────────    │
│                                                                         │
│  4. On NEW device (MacBook):                                           │
│     pwman init --name "MacBook"                                        │
│     → Creates own vault with own keys                                  │
│                                                                         │
│  5. Start P2P on MacBook:                                              │
│     pwman p2p start                                                   │
│     → Discovers Arch via mDNS (same network)                          │
│     → Auto-connects                                                   │
│                                                                         │
│  6. HELLO exchange:                                                    │
│     → MacBook sends HELLO with device info + vault ID                │
│     → Arch sees MacBook is new (not in trusted devices)              │
│                                                                         │
│  7. Approval request:                                                  │
│     → MacBook sends REQUEST_APPROVAL                                   │
│     → Arch shows approval prompt in UI                                 │
│                                                                         │
│  User clicks "Approve" on Arch:                                        │
│                                                                         │
│  8. Approve device:                                                    │
│     pwman p2p approve <device-id>                                      │
│     → Re-encrypts ALL passwords for MacBook                          │
│     → Sends APPROVE_DEVICE with encrypted keys                        │
│                                                                         │
│  9. Sync data:                                                         │
│     → Both devices exchange SYNC_DATA                                 │
│     → MacBook now has all entries + encrypted keys                   │
│     → Can decrypt all passwords!                                       │
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
5. Broadcast ENTRY_UPDATE to all connected peers
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

## P2P-Based Sync

### Sync Flow
```
┌─────────────────────────────────────────────────────────────────────────┐
│                           SYNC PROCESS                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  1. Device A starts P2P (listens on TCP + mDNS for discovery)          │
│                                                                         │
│  2. Device B starts P2P (same)                                          │
│                                                                         │
│  3. Device B connects to Device A via:                                 │
│     - mDNS (LAN discovery)                                             │
│     - Manual peer address (if remote)                                  │
│                                                                         │
│  4. HELLO exchange: exchange device info + vault ID                    │
│                                                                         │
│  5. If new device: REQUEST_APPROVAL → user approves                     │
│                                                                         │
│  6. SYNC_DATA exchange: share entries + devices                        │
│                                                                         │
│  7. Real-time updates: ENTRY_UPDATE broadcasts on changes              │
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

# P2P Sync (LAN-only for now)
pwman p2p start                          # Start P2P (listen + mDNS discovery)
pwman p2p stop                          # Stop P2P
pwman p2p status                        # Show P2P status
pwman p2p peers                         # List connected peers
pwman p2p connect <address>             # Connect to peer by address
pwman p2p approve <device-id>            # Approve pending device
pwman p2p reject <device-id>             # Reject pending device

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
    github.com/libp2p/go-libp2p v0.32.0
    golang.org/x/crypto v0.25.0
    github.com/atotto/clipboard v0.1.4
    github.com/google/uuid v1.6.0
)
```

---

## P2P Sync

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

### NAT Traversal (Current Limitation)

```
┌─────────────────────────────────────────────────────────────────┐
│                     CURRENT LIMITATION                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Device A (behind NAT)          Device B (behind NAT)           │
│  ─────────────────────          ─────────────────────             │
│       │                               │                          │
│       │  1. Both devices              │                          │
│       │     start P2P locally        │                          │
│       │───────────                   │                          │
│       │           ───────────────────│─────────                 │
│       │                               │                          │
│       │  2. mDNS discovers          │                          │
│       │     peers on LAN only!      │                          │
│       │                               │                          │
│       │  ❌ Cannot connect          │                          │
│       │     across NATs (yet)        │                          │
│                                                                 │
│  SOLUTION: Same network (LAN) OR manual peer address           │
│  FUTURE:    Relay server or Tor onion services                  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### P2P Protocol Messages

| Message | Direction | Purpose |
|---------|-----------|---------|
| HELLO | Bidirectional | Initial handshake, exchange device info |
| REQUEST_SYNC | Bidirectional | Request vault sync |
| SYNC_DATA | Bidirectional | Encrypted vault data |
| REQUEST_APPROVAL | New → Trusted | Request device approval |
| APPROVE_DEVICE | Trusted → New | Approve with encrypted keys |
| REJECT_DEVICE | Trusted → New | Reject device |
| ENTRY_UPDATE | Bidirectional | Real-time entry changes |
| ENTRY_DELETE | Bidirectional | Real-time entry deletion |
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

### P2P Sync Modes

| Mode | Description | Requirements |
|------|-------------|--------------|
| LAN (mDNS) | Local network discovery via mDNS | Same network |
| Direct | Manual peer address connection | External IP or port forwarding |
| Relay (Future) | Via libp2p circuit relay | Relay server |

### Current Capabilities

| Feature | Status |
|---------|--------|
| LAN discovery (mDNS) | ✅ Implemented |
| Direct peer connection | ✅ Implemented |
| Real-time sync | ✅ Implemented |
| Device approval | ✅ Implemented |
| NAT traversal | ⚠️ Limited |
| Remote relay | 🔜 Future |

### Dependencies (P2P)

```go
require (
    github.com/libp2p/go-libp2p v0.32.0
    github.com/libp2p/go-libp2p-pubsub v0.9.0
    github.com/libp2p/go-libp2p-kad-dht v0.24.0
    github.com/libp2p/go-mdns v0.0.3
)
```

**Note:** Relay client (for remote P2P) is planned for future implementation.

---

## Security Considerations

| Threat | Mitigation |
|--------|------------|
| Device theft | Private key encrypted with password (scrypt + AES-256-GCM) |
| Brute force | Strong KDF (scrypt) - ~100ms to derive key |
| Database leak | Passwords encrypted with AES-256-GCM |
| Clipboard theft | Auto-clear after 30 seconds |
| P2P eavesdropping | All P2P traffic uses Noise protocol |
| MITM attacks | Peer fingerprint verification |
| Network interception | Zero-knowledge - vault data encrypted end-to-end |

---

## Future Phases

### Phase 2: Remote P2P (Priority)
- Relay server for NAT traversal
- Tor onion services for serverless remote sync
- Pairing code flow (simpler UX)

### Phase 3: Enhanced Sync
- Real-time sync notifications
- Conflict resolution UI
- Selective sync (choose which entries to sync)

### Phase 4: Mobile
- iOS/Android native apps
- Biometric unlock (Face ID / Fingerprint)
- Push notifications

### Phase 5: Web
- Web-based access via PWA
- Browser extension
