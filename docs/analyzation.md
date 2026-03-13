# P2P Vault Sync - System Analysis

## Executive Summary

This document provides a comprehensive analysis of the P2P Vault Sync password manager system, focusing on how device pairing is implemented. The system uses libp2p for peer-to-peer communication with mDNS-based discovery, RSA key pairs for identity, and a hybrid encryption scheme for secure vault synchronization.

---

## 1. Architecture Overview

### 1.1 High-Level System Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                          DEVICE A (Generator)                                    │
│  ┌──────────────────┐                                                           │
│  │   HTTP API       │  ← REST endpoints for UI/CLI interaction                  │
│  │   Handlers       │                                                           │
│  └────────┬─────────┘                                                           │
│           │                                                                      │
│  ┌────────▼─────────┐      ┌──────────────┐      ┌──────────────────────┐      │
│  │  PairingHandler  │──────│ ServerState  │──────│   P2PManager         │      │
│  └────────┬─────────┘      └──────────────┘      └──────────┬───────────┘      │
│           │                                                  │                  │
│  ┌────────▼─────────┐                                       │ libp2p           │
│  │  DeviceManager   │←────── mDNS Discovery                 │ TCP/Noise        │
│  │  (Vault Logic)   │←────── Peer Routing                   │ Stream Handler   │
│  └────────┬─────────┘                                       │                  │
│           │                                                  ▼                  │
│  ┌────────▼─────────┐                             ┌──────────────────────┐      │
│  │ Crypto Module    │                             │ Peer B (Joiner)      │      │
│  │ RSA+AES Hybrid   │←────────────────────────────│ (same architecture)  │      │
│  └──────────────────┘                             └──────────────────────┘      │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### 1.2 Project Structure

```
/Users/user/Desktop/password_manager/
├── cmd/
│   ├── server/                    # HTTP server daemon
│   │   ├── main.go               # Server entry point
│   │   └── handlers/
│   │       ├── pairing.go        # Pairing HTTP handlers + P2P logic
│   │       ├── p2p.go           # P2P HTTP handlers + event loop
│   │       ├── device.go        # Device management handlers
│   │       └── ...
│   └── pwman/                    # CLI client
│       └── main.go
├── internal/
│   ├── device/
│   │   └── manager.go           # Device management operations
│   ├── p2p/
│   │   ├── p2p.go              # libp2p implementation
│   │   └── messages.go         # Message types and creation
│   ├── crypto/
│   │   ├── crypto.go           # RSA/AES encryption
│   │   └── hybrid.go           # Hybrid encryption scheme
│   ├── storage/
│   │   ├── interface.go        # Storage interface
│   │   └── sqlite.go          # SQLite implementation
│   ├── state/
│   │   └── server.go          # Server state management
│   └── config/
│       └── config.go          # Configuration management
└── pkg/models/
    └── models.go              # Data models
```

---

## 2. Core Components

### 2.1 Identity System (Device Identity)

**Location:** `internal/device/manager.go`, `internal/crypto/crypto.go`

Each device generates a long-lived **RSA-4096 keypair** on first vault initialization:

```go
// internal/device/manager.go:41
keyPair, err := crypto.GenerateRSAKeyPair(4096)
```

**Key Characteristics:**
- **Algorithm:** RSA-4096 (not Ed25519)
- **Storage:** Private key encrypted with user's master password using scrypt + AES-256-GCM
- **Fingerprint:** Base64-encoded PKCS#1 public key
- **Device ID:** UUID generated per vault

```go
// internal/crypto/crypto.go:84-86
func GetFingerprint(key *rsa.PublicKey) string {
    return base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(key))
}
```

**File Locations:**
- Private key: `~/.pwman/vaults/<vault>/private.key`
- Public key: `~/.pwman/vaults/<vault>/public.key`
- Salt file: `~/.pwman/vaults/<vault>/private.key.salt`

### 2.2 Discovery System

**Location:** `internal/p2p/p2p.go:210-220`

The system uses **libp2p's mDNS discovery** for local network peer discovery:

```go
func (p *P2PManager) startMDNS() error {
    mdns := mdns.NewMdnsService(p.host, "pwman", p)
    p.discovery = mdns
    return mdns.Start()
}
```

**How it works:**
1. Each device creates a libp2p host with a randomly assigned TCP port
2. mDNS service advertises the `_pwman._tcp` service on the local network
3. Peers automatically discover each other via multicast DNS
4. Peer addresses are stored in the peerstore with 30-minute TTL

**Key Configuration:**
```go
opts := []libp2p.Option{
    libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),  // Random port
    libp2p.Transport(tcp.NewTCPTransport),
    libp2p.Security(noise.ID, noise.New),            // Noise protocol
    libp2p.NATPortMap(),
    libp2p.EnableRelay(),
}
```

**Important:** This is NOT traditional mDNS/UDP on port 5353 as described in the desired workflow. It uses libp2p's mDNS implementation which operates differently.

### 2.3 Pairing System

**Location:** `cmd/server/handlers/pairing.go`

The pairing system uses a **9-character alphanumeric code** (not TOTP):

```go
// cmd/server/handlers/pairing.go:47-55
func generatePairingCode() string {
    const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
    code := make([]byte, 9)
    for i := 0; i < 9; i++ {
        n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
        code[i] = chars[n.Int64()]
    }
    return fmt.Sprintf("%s-%s-%s", string(code[0:3]), string(code[3:6]), string(code[6:9]))
}
```

**Code Format:** `XXX-XXX-XXX` (9 characters, groups of 3)

**Expiration:** 5 minutes

#### Pairing Flow

1. **Generator (Device A) creates pairing code:**
   - Generates a 9-char code stored in ServerState
   - Auto-starts P2P manager if not running
   - Code expires after 5 minutes

2. **Joiner (Device B) initiates join:**
   - User enters the pairing code
   - Joiner auto-starts P2P and discovers peers
   - Sends `PAIRING_REQUEST` message to all discovered peers

3. **Generator validates and responds:**
   - Validates code exists and isn't expired/used
   - Adds joiner as trusted device
   - Re-encrypts all vault entries for the new device
   - Sends `PAIRING_RESPONSE` with vault name and metadata

4. **Joiner receives and completes:**
   - Creates new vault with received metadata
   - Generates its own RSA keypair
   - Sends updated `PAIRING_REQUEST` with public key
   - Waits for `READY_FOR_SYNC` signal
   - Requests and receives vault sync data

### 2.4 Encryption System

**Location:** `internal/crypto/hybrid.go`

The system uses a **hybrid encryption scheme** combining RSA and AES:

```
Password Entry Encryption Flow:
┌─────────────────────────────────────────────────────────────┐
│ 1. Generate random AES-256 key per entry                    │
│ 2. Encrypt password with AES-256-GCM                       │
│ 3. Encrypt AES key with RSA public key of each device       │
│ 4. Store: EncryptedPassword + map[DeviceFingerprint]EncKey │
└─────────────────────────────────────────────────────────────┘
```

**Data Structure:**
```go
type EncryptedData struct {
    EncryptedPassword string              // AES-GCM encrypted
    EncryptedAESKeys  map[string]string   // RSA encrypted per device
}
```

**Security Properties:**
- Each password entry has its own AES key
- Only trusted devices can decrypt (via their RSA private key)
- When a new device joins, all entries must be re-encrypted

### 2.5 Transport Layer

**Location:** `internal/p2p/p2p.go:96-164`

The system uses **libp2p with Noise security protocol** (not mutual TLS):

```go
opts := []libp2p.Option{
    libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", 0)),
    libp2p.Transport(tcp.NewTCPTransport),
    libp2p.Security(noise.ID, noise.New),  // Noise Protocol
    libp2p.NATPortMap(),
    libp2p.EnableRelay(),
}
```

**Key Differences from Desired Workflow:**
- Uses **Noise protocol** instead of mutual TLS with Ed25519 certificates
- No certificate pinning or TOFU (Trust On First Use)
- No manual fingerprint verification

**Communication Protocol:**
- Custom JSON protocol over libp2p streams
- Newline-delimited messages
- Message types: PAIRING_REQUEST, PAIRING_RESPONSE, SYNC_DATA, etc.

---

## 3. State Management

**Location:** `internal/state/server.go`

The ServerState manages:

```go
type ServerState struct {
    vault            *Vault                    // Current unlocked vault
    p2pManager       *p2p.P2PManager          // P2P connections
    pairingCodes     map[string]PairingCode   // Active pairing codes
    pairingRequests  map[string]PairingRequest // Pending join requests
    pendingApprovals map[string]PendingApproval // Devices awaiting approval
    pairingState     *PairingState            // Current pairing operation
}
```

**Thread Safety:** All state access uses mutex locks for concurrency safety.

---

## 4. Message Protocol

**Location:** `internal/p2p/messages.go`

### 4.1 Message Types

| Type | Direction | Purpose |
|------|-----------|---------|
| `PAIRING_REQUEST` | Joiner → Generator | Initiate pairing with code |
| `PAIRING_RESPONSE` | Generator → Joiner | Accept/reject pairing |
| `READY_FOR_SYNC` | Generator → Joiner | Signal re-encryption complete |
| `REQUEST_SYNC` | Joiner → Generator | Request vault data |
| `SYNC_DATA` | Generator → Joiner | Send vault entries |
| `SYNC_ACK` | Joiner → Generator | Confirm sync received |

### 4.2 Message Structure

```go
type SyncMessage struct {
    Type      string `json:"type"`
    PeerID    string `json:"peer_id"`
    Payload   []byte `json:"payload"`
    Timestamp int64  `json:"timestamp"`
}
```

---

## 5. Database Schema

**Location:** `internal/storage/sqlite.go:17-54`

```sql
-- Device information
devices (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    public_key TEXT NOT NULL,
    fingerprint TEXT UNIQUE NOT NULL,
    created_at TIMESTAMP,
    trusted BOOLEAN DEFAULT FALSE
)

-- Password entries
entries (
    id TEXT PRIMARY KEY,
    version INTEGER DEFAULT 1,
    site TEXT NOT NULL,
    username TEXT,
    encrypted_password TEXT NOT NULL,
    notes TEXT,
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    updated_by TEXT REFERENCES devices(id),
    deleted_at TIMESTAMP
)

-- Per-device encryption keys
encrypted_keys (
    entry_id TEXT REFERENCES entries(id) ON DELETE CASCADE,
    device_fingerprint TEXT NOT NULL,
    encrypted_aes_key TEXT NOT NULL,
    PRIMARY KEY (entry_id, device_fingerprint)
)

-- Vault metadata
vault_meta (key TEXT PRIMARY KEY, value TEXT)
```

---

## 6. Detailed Pairing Sequence

```
┌─────────┐                                          ┌─────────┐
│ Device A│                                          │ Device B│
│Generator│                                          │ Joiner  │
└────┬────┘                                          └────┬────┘
     │                                                    │
     │ 1. User clicks "Generate Pairing Code"            │
     │ 2. Generates 9-char code (XXX-XXX-XXX)            │
     │ 3. Stores in ServerState.pairingCodes             │
     │ 4. Auto-starts P2P manager                        │
     │ 5. Waits for pairing requests                     │
     │                                                    │
     │◄───────────────────────────────────────────────────│ 6. User enters code
     │                                                    │ 7. Auto-starts P2P
     │                                                    │ 8. Discovers peers via mDNS
     │                                                    │
     │◄─────────── PAIRING_REQUEST ──────────────────────│ 9. Sends to all peers
     │   {code, deviceID, deviceName, publicKey}         │
     │                                                    │
     │ 10. Validates code exists & not expired           │
     │ 11. Adds Device B to trusted devices             │
     │ 12. Re-encrypts all entries for Device B         │
     │ 13. Updates database                             │
     │                                                    │
     │─────────── PAIRING_RESPONSE ─────────────────────►│
     │   {success: true, vaultName, ...}                │
     │                                                    │
     │                                                    │ 14. Creates new vault
     │                                                    │ 15. Generates RSA keys
     │                                                    │
     │◄─────────── PAIRING_REQUEST (updated) ────────────│ 16. Sends with publicKey
     │                                                    │
     │ 17. Updates Device B with public key             │
     │ 18. Sends READY_FOR_SYNC                         │
     │                                                    │
     │─────────── READY_FOR_SYNC ────────────────────────►│
     │                                                    │
     │                                                    │ 19. Sends REQUEST_SYNC
     │◄─────────── REQUEST_SYNC ──────────────────────────│
     │                                                    │
     │ 20. Sends all encrypted entries                  │
     │                                                    │
     │─────────── SYNC_DATA ─────────────────────────────►│
     │   {entries[], devices[]}                         │
     │                                                    │
     │                                                    │ 21. Stores entries
     │                                                    │ 22. Sends SYNC_ACK
     │◄─────────── SYNC_ACK ──────────────────────────────│
     │                                                    │
     │ 23. Both devices stop P2P                        │
     │                                                    │
```

---

## 7. Configuration System

**Location:** `internal/config/config.go`

The system uses a two-level configuration:

### 7.1 Global Config (`~/.pwman/config.json`)
```json
{
  "active_vault": "default",
  "vaults": ["default", "work"]
}
```

### 7.2 Vault Config (`~/.pwman/vaults/<name>/config.json`)
```json
{
  "device_id": "uuid",
  "device_name": "My Laptop",
  "salt": "base64..."
}
```

### 7.3 File Structure
```
~/.pwman/
├── config.json              # Global config
└── vaults/
    └── <vault_name>/
        ├── config.json      # Vault-specific config
        ├── vault.db         # SQLite database
        ├── private.key      # Encrypted RSA private key
        ├── private.key.salt # scrypt salt
        └── public.key       # RSA public key
```

---

## 8. Security Analysis

### 8.1 What's Implemented

✅ **Hybrid encryption** - AES-256 for passwords, RSA-4096 for key exchange  
✅ **Private key encryption** - Protected by user password + scrypt  
✅ **Trusted device model** - Only approved devices can decrypt  
✅ **Code expiration** - 5-minute timeout on pairing codes  
✅ **Single-use codes** - Codes marked as used after successful pairing  

### 8.2 What's Missing (Compared to Desired Workflow)

❌ **TOTP-based pairing** - Uses random code instead of time-based  
❌ **Ed25519 identity keys** - Uses RSA-4096 instead  
❌ **Mutual TLS with certificate pinning** - Uses libp2p Noise protocol  
❌ **mDNS on port 5353** - Uses libp2p's internal mDNS  
❌ **Manual fingerprint verification** - No visual fingerprint comparison  
❌ **Rate limiting on pairing attempts** - No brute force protection  
❌ **Vector clocks** - Simple version numbers, no conflict resolution  

---

## 9. Key Files Reference

| File | Purpose |
|------|---------|
| `internal/p2p/p2p.go:96` | P2P initialization and libp2p host |
| `internal/p2p/p2p.go:210` | mDNS discovery service |
| `internal/p2p/p2p.go:222` | Stream handler for incoming messages |
| `internal/p2p/messages.go:11` | Message type constants |
| `cmd/server/handlers/pairing.go:57` | Generate pairing code handler |
| `cmd/server/handlers/pairing.go:159` | Join vault handler |
| `cmd/server/handlers/pairing.go:591` | Handle incoming pairing requests |
| `internal/crypto/hybrid.go:16` | Hybrid encryption function |
| `internal/crypto/hybrid.go:56` | Hybrid decryption function |
| `internal/device/manager.go:141` | Add device to vault |
| `internal/state/server.go:55` | Server state structure |

---

## 10. Testing & Integration

**Test Files:**
- `cmd/server/pairing_test.go` - Pairing endpoint tests
- `cmd/server/device_test.go` - Device management tests
- `cmd/server/integration_test.go` - End-to-end integration tests
- `internal/p2p/pairing_test.go` - P2P pairing logic tests
- `internal/p2p/messages_test.go` - Message serialization tests

---

## 11. Summary

The current implementation is a functional P2P password manager with:

1. **Libp2p-based networking** with mDNS discovery
2. **RSA-4096 + AES-256 hybrid encryption**
3. **9-character pairing codes** with 5-minute expiration
4. **Automatic re-encryption** when new devices join
5. **SQLite backend** for local storage
6. **HTTP API** for UI/CLI interaction

However, it differs significantly from the desired workflow in several key security aspects, most notably the lack of TOTP-based pairing, mutual TLS with certificate pinning, and manual fingerprint verification.
