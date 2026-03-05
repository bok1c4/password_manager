# Phase 3: Practical Extraction Plan

## Current Status
- ✅ Health handlers extracted and tested (104 lines)
- ✅ Auth handlers extracted and tested (est. 300 lines)
- ⏳ Remaining: ~2,000 lines across 4 handler groups

## Challenge
The remaining handlers are highly interconnected:
- Entry handlers depend on vault state
- P2P handlers depend on p2p manager
- Pairing handlers depend on both vault and p2p
- All handlers access global state

## Solution: Incremental Extraction

### Step 1: Create Handler Wrappers (NOW)
Create handler packages with methods that call existing functions in main.go.

**Pros**:
- ✅ Establishes clean package structure
- ✅ Tests integration immediately
- ✅ No breaking changes
- ✅ Can verify everything works

**Cons**:
- ⚠️ Code still in main.go
- ⚠️ Not fully refactored yet

### Step 2: Move Handler Logic (LATER)
After verifying integration works, gradually move logic from main.go into handlers.

**Pros**:
- ✅ Safe, incremental approach
- ✅ Can test after each move
- ✅ Lower risk

## Implementation Plan

### 1. Entry Handlers (Simple CRUD)
- `GET /api/entries` - List entries
- `POST /api/entries/add` - Add entry
- `POST /api/entries/update` - Update entry
- `POST /api/entries/delete` - Delete entry
- `POST /api/entries/get_password` - Get decrypted password

### 2. Vault Handlers (Management)
- `GET /api/vaults` - List vaults
- `POST /api/vaults/use` - Switch vault
- `POST /api/vaults/create` - Create vault
- `POST /api/vaults/delete` - Delete vault
- `GET /api/sync/status` - Sync status

### 3. Device Handlers (Simple)
- `GET /api/devices` - List devices

### 4. P2P Handlers (Complex)
- All P2P network handlers (~15 endpoints)

### 5. Pairing Handlers (Complex)
- All pairing flow handlers (~5 endpoints)

## Next Steps

Given the scope, I recommend:

**Option A**: Create all handler wrappers now, test integration, then refine
**Option B**: Continue detailed extraction one-by-one (many hours)

**Recommendation**: Option A - Get structure in place, verify it works, then refine.

This approach:
- ✅ Completes Phase 3 faster
- ✅ Verifies integration works
- ✅ Establishes clean structure
- ✅ Allows incremental refinement later

Shall I proceed with Option A?
