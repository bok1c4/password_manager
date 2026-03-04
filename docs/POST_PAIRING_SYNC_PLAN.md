# Post-Pairing Sync Implementation Plan

## Current Status

Pairing is now working (code exchange succeeds), but after successful pairing:

1. **No vault created** on the joining device
2. **No sync listener** - joiner stops listening after pairing response
3. **Passwords not synced** - generator sends SYNC_DATA but no one receives it

---

## Issues Found

### Issue 1: Joiner returns immediately after pairing
Location: `handlePairingJoin` (line ~1907)
- After receiving `success=true`, function returns
- P2P message loop ends
- No one listening for SYNC_DATA

### Issue 2: No vault created on joiner
- Joiner doesn't create vault directory structure
- Doesn't store the generator's device info
- Doesn't initialize local database

### Issue 3: Generator timing
- `reEncryptEntriesForDevice` sends SYNC_DATA but joiner may have disconnected

---

## Implementation Plan

### Phase 1: Keep P2P alive after pairing

**File:** `cmd/server/main.go`

**Changes:**
1. After successful pairing response, don't exit immediately
2. Wait for SYNC_DATA message (with timeout)
3. Process received entries and create vault

### Phase 2: Create vault on joiner

**Changes:**
1. Create vault directory structure (`~/.pwman/vaults/<vaultName>/`)
2. Store generator's device info in local database
3. Initialize SQLite database with schema

### Phase 3: Handle incoming sync data

**Changes:**
1. Continue P2P message loop for N seconds after pairing
2. Handle MsgTypeSyncData
3. Store received entries in local database

---

## Detailed Tasks

### Task 1: Modify handlePairingJoin to wait for sync
```go
// After successful pairing response:
log.Printf("[Pairing Join] Waiting for vault sync...")

// Continue listening for sync data
select {
case msg := <-p2pManager.MessageChan():
    if msg.Type == p2p.MsgTypeSyncData {
        // Process entries
        // Create vault
    }
case <-time.After(30 * time.Second):
    log.Printf("[Pairing Join] Timeout waiting for sync")
}
```

### Task 2: Create vault on joiner
```go
func createVaultFromSync(vaultName string, deviceInfo DeviceData, entries []EntryData) error {
    // 1. Create vault directories
    // 2. Initialize SQLite
    // 3. Store device info
    // 4. Store entries
}
```

### Task 3: Send sync request from joiner
After pairing succeeds, joiner should request full sync:
```go
// After pairing success:
msg, _ := p2p.CreateRequestSyncMessage(fullSync=true)
p2pManager.SendMessage(peerID, msg)
```

---

## Expected Flow After Fix

```
Arch (Generator)          MacBook (Joiner)
─────────────             ───────────────
                          generate code
code: ABC-DEF-GHI         
        │                
        │─────── PAIRING_REQUEST ───────▶
        │                (code: ABCDEF...)
        │                
◀────── PAIRING_RESPONSE ────────────────
(success: true)          
        │                
        │  wait 2s for MacBook to be ready
        │                
─────── SYNC_DATA ──────────────────────▶
(entries + device info)  
        │                
        │                create vault
        │                store entries
        │                ✅ Complete
```

---

## Priority

1. **High**: Keep P2P alive to receive sync
2. **High**: Create vault on joiner
3. **Medium**: Handle sync data properly
4. **Low**: Request full sync after pairing

