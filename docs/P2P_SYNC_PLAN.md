# P2P Pairing & Sync Implementation Plan

## Current Architecture

### Components
1. **main.go** - HTTP API server with pairing flow
2. **internal/p2p/sync.go** - SyncHandler for data exchange (not connected to pairing)
3. **internal/crypto/hybrid.go** - HybridEncrypt/HybridDecrypt for re-encryption
4. **internal/storage/sqlite.go** - Database operations

### Current Flow (Broken)
```
Pairing Flow (main.go)          Sync Flow (sync.go)
─────────────────────          ───────────────────
Generate Code                  HELLO messages
  ↓                              ↓
Store in pairingCodes          Exchange device info
  ↓                              ↓
Peer connects ──────────────────→ Connected!
  ↓                              ↓
Send validation ──────────────→ Receive validation
  ↓                              ↓
Add to pendingApprovals        (NOT CONNECTED!)
  ↓                              ↓
???                            ???
```

## The Problem

The pairing flow adds devices to `pendingApprovals` in main.go but:
1. Doesn't trigger re-encryption of entries
2. Doesn't send re-encrypted data to new device
3. SyncHandler in sync.go is not integrated with pairing

## Required Implementation

### Phase 1: Connect Pairing → Sync

**After successful pairing validation on generating side:**

1. Mark device as trusted in database
2. Load all existing entries
3. For each entry:
   - Decrypt with current device's private key
   - Re-encrypt with new device's public key
4. Send re-encrypted entries via P2P to new device
5. New device receives and stores entries

### Phase 2: Re-encryption Logic

```go
// On generating device (has original entries)
func reEncryptForNewDevice(newDevicePublicKey string) error {
    entries := vault.storage.ListEntries()
    
    for _, entry := range entries {
        // 1. Decrypt with owner's private key
        password, err := crypto.HybridDecrypt(entry, vault.privateKey.PrivateKey)
        if err != nil {
            return err
        }
        
        // 2. Re-encrypt for new device
        newDevice := models.Device{
            ID:          newDevice.ID,
            Name:        newDevice.Name,
            PublicKey:   newDevicePublicKey,
            Fingerprint: newDevice.Fingerprint,
            Trusted:     true,
        }
        
        encrypted, err := crypto.HybridEncrypt(password, []models.Device{newDevice}, loadPublicKey)
        if err != nil {
            return err
        }
        
        // 3. Store in encrypted_keys table
        // 4. Send to new device via P2P
    }
}
```

### Phase 3: New Device Receiving

```go
// On joining device
func handleIncomingSyncData(payload SyncDataPayload) error {
    for _, entry := range payload.Entries {
        // 1. Store entry in database
        vault.storage.CreateEntry(&entry)
        
        // 2. Store encrypted AES keys for this device
    }
}
```

## Implementation Steps

### Step 1: Add re-encryption function to main.go
- Create function `reEncryptEntriesForDevice(deviceID, devicePublicKey string) error`
- Use existing `crypto.HybridEncrypt` 
- Update encrypted_keys table

### Step 2: Trigger re-encryption after successful pairing
In `handlePairingResponse` after validation succeeds:
```go
// On generating side
if response.Success {
    // 1. Add device to trusted devices
    vault.storage.UpsertDevice(&models.Device{
        ID: response.DeviceID,
        Name: response.DeviceName,
        PublicKey: response.PublicKey,
        Fingerprint: response.Fingerprint,
        Trusted: true,
    })
    
    // 2. Re-encrypt all entries for new device
    reEncryptEntriesForDevice(response.DeviceID, response.PublicKey)
    
    // 3. Send sync data
    p2pManager.SendMessage(peerID, syncMessage)
}
```

### Step 3: Handle incoming sync on new device
```go
// In P2P message handler
if msg.Type == p2p.MsgTypeSyncData {
    var payload SyncDataPayload
    json.Unmarshal(msg.Payload, &payload)
    
    for _, entry := range payload.Entries {
        vault.storage.CreateEntry(entry)
    }
}
```

## Data Flow Diagram

```
┌─────────────────┐         ┌─────────────────┐
│   Arch Linux    │         │    MacBook     │
│  (has vault)    │         │  (new device)  │
└────────┬────────┘         └────────┬────────┘
         │                            │
         │  1. Generate Code         │
         │  2. P2P Starts            │
         │                            │  3. Enter Code
         │                            │  4. P2P Starts
         │  5. Peer Connects         │◄─────────────
         │◄──────────────────────────┘
         │
         │  6. Validate Code
         │  7. Add to trusted devices
         │  8. RE-ENCRYPT all entries
         │     for MacBook's public key
         │
         │───────────────────────────►
         │  9. Send re-encrypted
         │     entries via P2P
         │
         │                            │ 10. Receive &
         │                            │     Store entries
         │                            │
         ▼                            ▼
    ✅ Synced!                  ✅ Synced!
```

## Files to Modify

1. **cmd/server/main.go**
   - Add `reEncryptEntriesForDevice()` function
   - Call after successful pairing validation
   - Add message handler for incoming sync data

2. **internal/crypto/hybrid.go** (may need extension)
   - Add function to encrypt for single device

3. **internal/storage/sqlite.go** (may need extension)
   - Add method to update encrypted keys for entry

## Message Types (Already Exist)

- `MsgTypeSyncData` - Send entries and devices
- `MsgTypeApproveDevice` - Contains re-encrypted entries

## Testing Checklist

- [ ] Generate code on device A
- [ ] Join from device B
- [ ] Device A validates code
- [ ] Device A re-encrypts all entries
- [ ] Device A sends to device B
- [ ] Device B receives and stores
- [ ] Both devices show same passwords
- [ ] New password on A syncs to B
- [ ] New password on B syncs to A
