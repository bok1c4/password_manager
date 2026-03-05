# Authentication Flow Fix

## Issues Fixed

### 1. ✅ Vault Listing Now Public
**Problem:** `/api/vaults` required authentication, preventing users from seeing vaults before login.

**Solution:** Changed `/api/vaults` to public endpoint (no auth required).

```go
// Before
http.HandleFunc("/api/vaults", auth(vaultHandlers.List))

// After  
http.HandleFunc("/api/vaults", cors(vaultHandlers.List))
```

### 2. ✅ Token Storage in Frontend
**Problem:** Frontend wasn't storing or using the authentication token from unlock.

**Solution:** Updated `src/lib/api.ts` to:
- Store token after successful unlock
- Include `Authorization: Bearer <token>` header in authenticated requests
- Clear token on lock or vault switch

```typescript
// Token management
let authToken: string | null = null;

function setToken(token: string) {
  authToken = token;
}

// Add auth header to requests
if (token && !publicEndpoints.includes(endpoint)) {
  headers['Authorization'] = `Bearer ${token}`;
}
```

---

## Public vs Authenticated Endpoints

### Public Endpoints (No Auth Required)
- `/api/health` - Health check
- `/api/is_initialized` - Check if vault exists
- `/api/vaults` - List available vaults ⭐ **NEW**
- `/api/init` - Initialize new vault
- `/api/unlock` - Unlock vault (returns token)

### Authenticated Endpoints (Token Required)
- `/api/lock` - Lock vault
- `/api/is_unlocked` - Check unlock status
- `/api/entries/*` - All entry operations
- `/api/devices` - List devices
- `/api/vaults/use` - Switch vault
- `/api/vaults/create` - Create vault
- `/api/vaults/delete` - Delete vault
- `/api/p2p/*` - All P2P operations
- `/api/pairing/*` - All pairing operations
- `/api/generate` - Generate password
- `/api/metrics` - Metrics

---

## Authentication Flow

### 1. Initial Load
```typescript
// Frontend checks if vaults exist
const vaults = await api.getVaults();  // Public endpoint
const isInit = await api.isInitialized();  // Public endpoint
```

### 2. User Unlocks Vault
```typescript
// User enters password and clicks unlock
const result = await api.unlock(password);

// Server returns:
{
  "success": true,
  "data": {
    "token": "4qC4I0zkKhU_uxbSp_iV_PqlpyUnEF3yyjF-2IVsL-4="
  }
}

// Frontend stores token automatically
```

### 3. Subsequent Requests
```typescript
// All authenticated requests now include token
const entries = await api.getEntries();
// Request includes: Authorization: Bearer 4qC4I0zkKhU_...

const devices = await api.getDevices();
// Request includes: Authorization: Bearer 4qC4I0zkKhU_...
```

### 4. Lock or Switch Vault
```typescript
// Lock clears token
await api.lock();

// Switch vault clears token (requires re-auth)
await api.useVault('work');
```

---

## Testing

### Test Vault Listing (No Auth)
```bash
curl http://localhost:18475/api/vaults
# Returns list of vaults without needing token
```

### Test Unlock Flow
```bash
# 1. Unlock and get token
TOKEN=$(curl -s -X POST http://localhost:18475/api/unlock \
  -H "Content-Type: application/json" \
  -d '{"password":"your_password"}' | jq -r '.data.token')

# 2. Use token for authenticated requests
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:18475/api/entries

curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:18475/api/devices
```

### Test with Frontend
1. Start server: `./bin/pwman-server`
2. Open app in browser or run Tauri
3. Should see vault list immediately (no auth needed)
4. Unlock vault with password
5. Should now see entries (token auto-included)

---

## Files Changed

1. **cmd/server/main.go**
   - Made `/api/vaults` public (removed auth requirement)

2. **src/lib/api.ts**
   - Added token storage in memory
   - Auto-include token in authenticated requests
   - Clear token on lock/vault switch
   - Updated public endpoints list

---

## Security Notes

- Token is stored in memory (not localStorage) for security
- Token is cleared on lock or vault switch
- Public endpoints are minimal (only what's needed before auth)
- All sensitive operations require authentication

---

## Next Steps

1. ✅ Test vault listing works without auth
2. ✅ Test unlock returns token
3. ✅ Test token is used in subsequent requests
4. ✅ Test entries can be fetched after unlock
5. ✅ Test lock clears token
6. ✅ Test vault switch requires re-auth

All endpoints should now work correctly! 🎉
