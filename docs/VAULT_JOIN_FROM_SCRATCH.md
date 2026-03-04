# Join Vault From Scratch - Architecture

## Current Problem
- Joiner must have vault already created and unlocked
- No vault data is transferred during pairing
- User must set up vault manually on each device

## Target Flow

```
GENERATOR (Arch)                    JOINER (MacBook)
───────────────                     ─────────────────
1. User has vault "work"           0. No vault exists
   with password "secret123"
                                    1. User enters:
   2. Generate code                   - Code: K8Y-C9Z-YPB
   3. P2P listening                  - Password: secret123
                                       (same as generator!)
                                    2. Send PAIRING_REQUEST
                                       (includes: code + password hash)
                                      
4. Validate code                   
   (password matches expected)       
                                    5. PAIRING_RESPONSE
                                       - vault_name: "work"
                                       - all_devices[]
                                       - all_entries[]
                                       (entries encrypted for joiner's pubkey)

6. Re-encrypt entries              
   for joiner's pubkey              
                                    7. Create vault "work"
                                       - Create dirs
   8. Wait for joiner ready         - Initialize SQLite
   9. Send SYNC_DATA                - Store devices
      (all entries + devices)        - Store entries
                                       - Unlock with password
                                    10. ✅ Vault ready!
```

## Key Changes Required

### 1. Modify PairingRequestPayload
```go
type PairingRequestPayload struct {
    Code       string `json:"code"`
    DeviceID   string `json:"device_id"`
    DeviceName string `json:"device_name"`
    Password   string `json:"password,omitempty"` // For verification
}
```

### 2. Modify PairingResponsePayload  
```go
type PairingResponsePayload struct {
    Success     bool     `json:"success"`
    VaultName   string   `json:"vault_name"`     // NEW: vault name
    DeviceID    string   `json:"device_id"`
    DeviceName  string   `json:"device_name"`
    PublicKey   string   `json:"public_key"`
    Fingerprint string   `json:"fingerprint"`
    Error       string   `json:"error,omitempty"`
}
```

### 3. Generator Side Changes
- On pairing request: verify password matches (or skip for now)
- Send vault name in response
- Send ALL devices and entries in SYNC_DATA (not just re-encrypted ones)

### 4. Joiner Side Changes
- Accept password from user in join request
- If vault doesn't exist, create it:
  - Create vault directory
  - Initialize with password (derive key, encrypt private key)
  - Initialize SQLite database
- Store received devices and entries
- Unlock vault with provided password

## Implementation Tasks

### Task 1: Update API to accept password in join request
File: Frontend/Tauri
- Add password field to pairing join form

### Task 2: Update PairingRequestPayload  
File: internal/p2p/messages.go
- Add Password field

### Task 3: Generator validates password (optional)
File: cmd/server/main.go
- Optional: verify password matches stored hash

### Task 4: Generator sends vault name
File: cmd/server/main.go
- Include vault_name in PAIRING_RESPONSE

### Task 5: Joiner creates vault if not exists
File: cmd/server/main.go
- If vault doesn't exist, create it
- Initialize with provided password
- Store received data

### Task 6: Full sync on joiner
File: cmd/server/main.go
- Receive all devices
- Receive all entries
- Store in local database

## Security Considerations

1. **Password transmission**: Currently sent in plain text over P2P
   - Future: Hash the password and verify, or use PAKE
   
2. **Private key on joiner**: New device generates its own key pair
   - Generator sends entries re-encrypted for joiner's public key
   - Joiner can decrypt with its private key

3. **Vault password**: Same on all devices (user enters same password)
   - Used to encrypt/decrypt private key locally
   - Not stored anywhere, just used for key derivation

## Files to Modify

1. `internal/p2p/messages.go` - Add Password to request, VaultName to response
2. `cmd/server/main.go` - handlePairingJoin, handlePairingGenerate
3. Frontend - Add password input to join form

