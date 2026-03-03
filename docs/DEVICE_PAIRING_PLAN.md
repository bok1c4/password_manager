# Device Pairing Implementation Plan

## Goal
Simplify device pairing with a code-based flow similar to Bitwarden/1Password:
1. Device A generates a pairing code
2. Device B enters the code to join
3. Both devices sync automatically

## Current State
- P2P connection works (mDNS auto-discovers devices on LAN)
- Device approval works but requires vault to be unlocked
- Vault must be unlocked before P2P can start

## Proposed Flow

### Option 1: Local Network Discovery (Recommended for MVP)
```
Device A (has passwords)          Device B (new device)
===================          ==================
1. Open Settings              1. Open Settings
2. Click "Add Device"         2. Click "Join Vault"
                              
3. Shows:                     3. Shows:
   "Waiting for device..."       "Enter code from other device"
   Code: ABC-123-XYZ            [_________]
                                  [Connect]

4. Device B enters code
5. Both devices connect via mDNS/P2P
6. Device A approves automatically
7. Sync happens automatically
```

### Option 2: Manual Code Exchange (More Robust)
```
Device A (has passwords)          Device B (new device)
===================          ==================
1. Generate pairing code         1. Open "Join Vault"
   QR code or text:               2. Enter code + vault URL
   VAULT-ABC123-KEY456            

2. User manually shares          3. Connect via internet
   code (copy/paste)             4. Get approval
```

## Implementation Tasks

### Phase 1: Code-Based Pairing (Local Network)

#### 1.1 Generate Pairing Code
- Generate unique 9-character code (e.g., `VAULT-ABC-123`)
- Code contains: vault ID + random bytes for verification
- Code expires after 5 minutes
- Code can only be used once

**Files to modify:**
- `cmd/server/main.go` - Add `/api/pairing/generate` endpoint
- `internal/storage/` - Store pairing requests

#### 1.2 Join with Code
- Device B enters code
- Device B looks up device via local network (mDNS)
- Establishes P2P connection
- Requests pairing

**Files to modify:**
- `cmd/server/main.go` - Add `/api/pairing/join` endpoint
- `internal/p2p/` - Handle pairing protocol messages

#### 1.3 Auto-Approval
- When Device B connects with valid code, auto-approve
- Re-encrypt all passwords for new device
- Trigger sync

**Files to modify:**
- `cmd/server/main.go` - Modify approval logic
- `internal/p2p/sync.go` - Add re-encryption logic

#### 1.4 Frontend UI
- Add "Add Device" button in Settings
- Show pairing code display
- Add "Join Vault" screen
- Enter code input field

**Files to modify:**
- `src/components/Settings.tsx` - Add pairing UI
- `src/App.tsx` - Add join vault route

---

### Phase 2: Remote Pairing (Future)

#### 2.1 Relay Server (Optional)
- Run a simple relay server
- Devices connect to relay when not on same network
- Trade connection info through relay

#### 2.2 QR Code Scanning (Nice to have)
- Generate QR code from pairing code
- Scan QR with phone camera to add mobile device

---

## Data Structures

### PairingRequest
```go
type PairingRequest struct {
    Code        string    `json:"code"`
    DeviceName  string    `json:"device_name"`
    PublicKey   string    `json:"public_key"`
    RequestedAt time.Time `json:"requested_at"`
}

type PairingCode struct {
    Code       string    `json:"code"`
    VaultID    string    `json:"vault_id"`
    DeviceID   string    `json:"device_id"`
    PublicKey  string    `json:"public_key"`  // Device A's public key
    ExpiresAt  time.Time `json:"expires_at"`
    Used       bool      `json:"used"`
}
```

### API Endpoints

```
POST /api/pairing/generate
  - Creates pairing code for current device
  - Returns: { code: "VAULT-ABC-123", expires_in: 300 }

POST /api/pairing/join
  - Body: { code: "VAULT-ABC-123", device_name: "MacBook" }
  - Returns: { success: true, message: "Connected and approved" }

GET /api/pairing/status
  - Returns: { waiting: true/false, connected_device: "..." }
```

---

## Sync Flow After Pairing

1. Device B connects to Device A via P2P
2. Device A receives pairing request with code
3. Device A validates code
4. Device A auto-approves Device B (or shows confirmation)
5. Device A re-encrypts all passwords for Device B's public key
6. Device A sends all vault data to Device B
7. Device B decrypts and stores passwords
8. Both devices sync on any future changes

---

## Security Considerations

1. **Code expiration** - 5 minutes max
2. **One-time use** - Code can only be used once
3. **Verification** - Code includes vault ID to prevent joining wrong vault
4. **Encryption** - All data still encrypted end-to-end
5. **No private keys shared** - Only public keys exchanged

---

## Implementation Order

1. [ ] Add pairing code generation endpoint
2. [ ] Add pairing join endpoint  
3. [ ] Modify P2P to handle pairing flow
4. [ ] Add auto-approval with code validation
5. [ ] Implement re-encryption on approval
6. [ ] Add frontend UI for pairing
7. [ ] Test end-to-end flow

---

## Similar Implementations for Reference

- **Bitwarden**: Uses QR code or manual entry of "Bitwarden Connect" 
- **1Password**: Uses "Setup Key" printed on paper
- **Enpass**: Uses QR code scanning

All use short-lived, one-time codes that device exchanges over local network or manual entry.
