# Testing Guide - Password Manager

**Version**: 1.0  
**Last Updated**: 2026-03-05  
**Goal**: Comprehensive testing strategy for all components

---

## Quick Start

```bash
# Run all tests
go test ./...

# Run with race detector
go test -race ./...

# Run with coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific package
go test ./internal/crypto/...
go test ./internal/storage/...

# Verbose output
go test -v ./...

# Run benchmarks
go test -bench=. ./...
```

---

## Test Categories

### 1. Unit Tests

**Location**: `*_test.go` files next to source files

**Coverage Requirements**:
| Package | Target | Critical Paths |
|---------|--------|----------------|
| crypto | 90%+ | Encrypt/decrypt, key generation |
| storage | 85%+ | CRUD, transactions, queries |
| config | 80%+ | Load/save, migration |
| p2p | 70%+ | Message handling, connections |
| api | 75%+ | Handlers, middleware |

#### Crypto Tests

```go
// internal/crypto/crypto_test.go

func TestAESEncryptDecrypt(t *testing.T) {
    key, _ := GenerateAESKey()
    plaintext := []byte("secret password")
    
    encrypted, err := AESEncrypt(plaintext, key)
    if err != nil {
        t.Fatalf("encrypt failed: %v", err)
    }
    
    decrypted, err := AESDecrypt(encrypted, key)
    if err != nil {
        t.Fatalf("decrypt failed: %v", err)
    }
    
    if !bytes.Equal(decrypted, plaintext) {
        t.Error("decrypted text doesn't match")
    }
}

func TestAESDecryptWithWrongKey(t *testing.T) {
    key1, _ := GenerateAESKey()
    key2, _ := GenerateAESKey()
    plaintext := []byte("secret")
    encrypted, _ := AESEncrypt(plaintext, key1)
    
    _, err := AESDecrypt(encrypted, key2)
    if err == nil {
        t.Error("should fail with wrong key")
    }
}

func TestHybridEncryption(t *testing.T) {
    keyPair, _ := GenerateRSAKeyPair(4096)
    devices := []models.Device{
        {Fingerprint: GetFingerprint(keyPair.PublicKey), Trusted: true},
    }
    
    getPublicKey := func(fp string) (*rsa.PublicKey, error) {
        return keyPair.PublicKey, nil
    }
    
    encrypted, err := HybridEncrypt("password", devices, getPublicKey)
    if err != nil {
        t.Fatalf("hybrid encrypt failed: %v", err)
    }
    
    decrypted, err := HybridDecrypt(&models.PasswordEntry{
        EncryptedPassword: encrypted.EncryptedPassword,
        EncryptedAESKeys:  encrypted.EncryptedAESKeys,
    }, keyPair.PrivateKey)
    
    if err != nil || decrypted != "password" {
        t.Error("hybrid decrypt failed")
    }
}
```

#### Storage Tests

```go
// internal/storage/sqlite_test.go

func TestDeviceCRUD(t *testing.T) {
    db, _ := NewSQLite("test.db")
    defer db.Close()
    defer os.Remove("test.db")
    
    device := &models.Device{
        ID:          uuid.New().String(),
        Name:        "Test",
        Fingerprint: "fp123",
        Trusted:     true,
    }
    
    // Create
    if err := db.UpsertDevice(device); err != nil {
        t.Fatalf("upsert failed: %v", err)
    }
    
    // Read
    retrieved, err := db.GetDevice(device.ID)
    if err != nil {
        t.Fatalf("get failed: %v", err)
    }
    if retrieved.Name != device.Name {
        t.Error("name mismatch")
    }
    
    // Update
    device.Name = "Updated"
    db.UpsertDevice(device)
    retrieved, _ = db.GetDevice(device.ID)
    if retrieved.Name != "Updated" {
        t.Error("update failed")
    }
    
    // Delete
    db.DeleteDevice(device.ID)
    _, err = db.GetDevice(device.ID)
    if err == nil {
        t.Error("should have been deleted")
    }
}

func TestEntryEncryptionKeys(t *testing.T) {
    db, _ := NewSQLite("test.db")
    defer db.Close()
    
    entry := &models.PasswordEntry{
        ID:                uuid.New().String(),
        Site:              "github.com",
        EncryptedPassword: "enc123",
        EncryptedAESKeys: map[string]string{
            "device1": "key1",
            "device2": "key2",
        },
    }
    
    db.CreateEntry(entry)
    retrieved, _ := db.GetEntry(entry.ID)
    
    if len(retrieved.EncryptedAESKeys) != 2 {
        t.Errorf("expected 2 keys, got %d", len(retrieved.EncryptedAESKeys))
    }
}
```

### 2. Integration Tests

**Location**: `tests/integration_test.go`

```go
package tests

import (
    "testing"
    "github.com/bok1c4/pwman/internal/api"
    "github.com/bok1c4/pwman/internal/storage"
)

func TestFullWorkflow(t *testing.T) {
    // Setup
    db, _ := storage.NewSQLite(":memory:")
    defer db.Close()
    
    // 1. Initialize vault
    // 2. Add device
    // 3. Add entry
    // 4. Verify encryption
    // 5. Decrypt and verify
}

func TestAPIEndpoints(t *testing.T) {
    // Start test server
    // Test each endpoint
    // Verify responses
}
```

### 3. Race Condition Tests

```bash
# Run with race detector
go test -race ./...

# Specific test with race
go test -race -run TestConcurrentAccess ./internal/storage/...
```

Example:
```go
func TestConcurrentVaultAccess(t *testing.T) {
    vault := setupTestVault()
    
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(n int) {
            defer wg.Done()
            
            // Concurrent reads
            entry, err := vault.GetEntry("test")
            if err != nil && err != storage.ErrNotFound {
                t.Errorf("concurrent read failed: %v", err)
            }
            
            // Occasional writes
            if n%10 == 0 {
                vault.UpdateEntry(&models.PasswordEntry{...})
            }
        }(i)
    }
    wg.Wait()
}
```

### 4. Benchmarks

```go
func BenchmarkAESEncrypt(b *testing.B) {
    key, _ := GenerateAESKey()
    plaintext := []byte("test password for benchmarking")
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        AESEncrypt(plaintext, key)
    }
}

func BenchmarkRSAEncrypt(b *testing.B) {
    keyPair, _ := GenerateRSAKeyPair(4096)
    plaintext := make([]byte, 32) // AES key size
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        RSAEncrypt(plaintext, keyPair.PublicKey)
    }
}

func BenchmarkScrypt(b *testing.B) {
    password := "test password"
    salt, _ := GenerateSalt()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        DeriveKey(password, salt)
    }
}
```

---

## Test Data

### Fixtures

Create `testdata/` directories:

```
internal/crypto/testdata/
├── valid_private_key.pem
├── valid_public_key.pem
├── corrupted_key.pem
└── wrong_format_key.txt
```

### Test Helpers

```go
// internal/testutil/helpers.go
package testutil

import (
    "testing"
    "github.com/bok1c4/pwman/internal/crypto"
    "github.com/bok1c4/pwman/pkg/models"
)

func GenerateTestDevice(t *testing.T) (*models.Device, *crypto.KeyPair) {
    t.Helper()
    
    keyPair, err := crypto.GenerateRSAKeyPair(2048)
    if err != nil {
        t.Fatalf("failed to generate keys: %v", err)
    }
    
    device := &models.Device{
        ID:          "test-device-1",
        Name:        "Test Device",
        Fingerprint: crypto.GetFingerprint(keyPair.PublicKey),
        PublicKey:   "test-key",
        Trusted:     true,
    }
    
    return device, keyPair
}

func GenerateTestEntry(t *testing.T, device *models.Device) *models.PasswordEntry {
    t.Helper()
    
    return &models.PasswordEntry{
        ID:                "test-entry-1",
        Site:              "test.com",
        Username:          "testuser",
        EncryptedPassword: "encrypted123",
        EncryptedAESKeys: map[string]string{
            device.Fingerprint: "aes-key-123",
        },
    }
}
```

---

## Manual Testing

### Critical Paths

#### Path 1: Vault Initialization
```bash
# 1. Clean state
rm -rf ~/.pwman

# 2. Initialize
./pwman init --name "Test Device"
# Expected: Password prompt, device ID displayed

# 3. Verify files exist
ls ~/.pwman/
# Expected: vault.db, config.json, private.key, public.key

# 4. Check permissions
stat ~/.pwman/private.key
# Expected: 0600 permissions
```

#### Path 2: Add and Retrieve Password
```bash
# 1. Add entry
./pwman add github.com --username testuser
# Expected: Password prompt, "Password added" confirmation

# 2. List entries
./pwman list
# Expected: github.com entry visible

# 3. Get password
./pwman get github.com
# Expected: Password displayed

# 4. Copy to clipboard
./pwman get github.com --clipboard
# Expected: "Copied to clipboard", clipboard cleared after 30s
```

#### Path 3: P2P Device Pairing
```bash
# Device A (Generator)
./pwman init --name "Generator"
./pwman add test.com -u user -p password123
./pwman p2p start
# Note the pairing code displayed

# Device B (Joiner) - on same LAN
./pwman init --name "Joiner"
./pwman p2p start
./pwman pairing join <CODE>

# Verify
./pwman list
# Expected: test.com entry visible
```

#### Path 4: Multi-Vault
```bash
# Create work vault
./pwman vault create work
./pwman vault use work
./pwman init --name "Work Device"
./pwman add work.com -u workuser

# Switch to personal
./pwman vault create personal
./pwman vault use personal
./pwman init --name "Personal Device"
./pwman add personal.com -u me

# Verify isolation
./pwman list
# Expected: Only personal.com

./pwman vault use work
./pwman list
# Expected: Only work.com
```

### Edge Cases

#### Large Vault
```bash
# Create 1000 entries
for i in {1..1000}; do
    ./pwman add "site$i.com" --username "user$i" --password "pass$i"
done

# Test operations
./pwman list | wc -l  # Should be 1000
./pwman get site500.com  # Should be fast
```

#### Concurrent Access
```bash
# Terminal 1: Start server
./pwman-server

# Terminal 2 & 3: Simultaneous operations
./pwman add site1.com -u user1 &
./pwman add site2.com -u user2 &
wait
```

#### Network Failure
```bash
# Start P2P
./pwman p2p start

# Add entry
./pwman add test.com -u test

# Disconnect network
# ./pwman p2p status should show disconnected

# Reconnect network
# Should auto-reconnect (if implemented)
```

---

## Security Testing

### Password Exposure

```bash
# Check logs for password
grep -r "password" /var/log/
# Expected: No plaintext passwords

# Check process memory
cat /proc/$(pgrep pwman)/maps
# Use tools like volatility for deeper analysis

# Check clipboard
xclip -o -selection clipboard
# Should be empty after 30s
```

### API Authentication

```bash
# Without token
curl http://localhost:18475/api/entries
# Expected: 401 Unauthorized

# With token
curl -H "Authorization: Bearer <TOKEN>" http://localhost:18475/api/entries
# Expected: 200 OK with data
```

### Rate Limiting

```bash
# Flood API
for i in {1..100}; do
    curl -H "Authorization: Bearer <TOKEN>" http://localhost:18475/api/entries &
done
# Expected: Some requests return 429 Too Many Requests
```

---

## Frontend Testing

### Unit Tests (Jest)

```typescript
// src/lib/api.test.ts
import { api } from './api';
import fetchMock from 'jest-fetch-mock';

describe('API', () => {
  beforeEach(() => {
    fetchMock.resetMocks();
  });

  test('unlock sends correct request', async () => {
    fetchMock.mockResponseOnce(JSON.stringify({ success: true }));
    
    await api.unlock('testpassword');
    
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/unlock'),
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ password: 'testpassword' }),
      })
    );
  });

  test('handles API errors', async () => {
    fetchMock.mockResponseOnce(
      JSON.stringify({ success: false, error: 'wrong password' })
    );
    
    await expect(api.unlock('wrong')).rejects.toThrow('wrong password');
  });
});
```

### E2E Tests (Playwright)

```typescript
// e2e/vault.spec.ts
import { test, expect } from '@playwright/test';

test('user can unlock vault', async ({ page }) => {
  await page.goto('http://localhost:1420');
  
  // Enter password
  await page.fill('input[type="password"]', 'testpassword');
  await page.click('button:has-text("Unlock")');
  
  // Verify unlocked
  await expect(page.locator('text=Vault unlocked')).toBeVisible();
});

test('user can add password entry', async ({ page }) => {
  // Unlock first
  await unlockVault(page);
  
  // Add entry
  await page.click('button:has-text("Add")');
  await page.fill('input[name="site"]', 'github.com');
  await page.fill('input[name="username"]', 'testuser');
  await page.fill('input[name="password"]', 'secret123');
  await page.click('button:has-text("Save")');
  
  // Verify added
  await expect(page.locator('text=github.com')).toBeVisible();
});
```

---

## Performance Testing

### Load Testing

```bash
# Using Apache Bench
ab -n 10000 -c 100 http://localhost:18475/api/entries

# Using k6
k6 run loadtest.js
```

```javascript
// loadtest.js
import http from 'k6/http';
import { check } from 'k6';

export const options = {
  stages: [
    { duration: '30s', target: 100 },
    { duration: '1m', target: 100 },
    { duration: '30s', target: 0 },
  ],
};

export default function () {
  const res = http.get('http://localhost:18475/api/entries');
  check(res, {
    'status is 200': (r) => r.status === 200,
    'response time < 200ms': (r) => r.timings.duration < 200,
  });
}
```

### Memory Profiling

```bash
# Run with memory profiling
go test -bench=. -memprofile=mem.out ./internal/crypto/
go tool pprof mem.out

# In pprof:
# top10 - show top memory consumers
# list <function> - show detailed breakdown
# web - generate visual graph
```

---

## Continuous Integration

### GitHub Actions

```yaml
# .github/workflows/test.yml
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.25'
      
      - name: Run tests
        run: go test -race -cover ./...
      
      - name: Check coverage
        run: |
          go test -coverprofile=coverage.out ./...
          go tool cover -func=coverage.out | grep total
      
      - name: Lint
        uses: golangci/golangci-lint-action@v3
      
      - name: Build
        run: |
          go build ./cmd/pwman
          go build ./cmd/server
```

---

## Test Checklist

### Before Release

- [ ] All unit tests pass
- [ ] Race detector clean
- [ ] Coverage >80% for critical paths
- [ ] Manual critical paths tested
- [ ] Security tests pass
- [ ] Performance benchmarks acceptable
- [ ] Integration tests pass
- [ ] Frontend E2E tests pass

### Security Testing

- [ ] No passwords in logs
- [ ] API authentication works
- [ ] Rate limiting active
- [ ] CORS properly configured
- [ ] SQL injection attempts blocked
- [ ] Clipboard cleared after timeout
- [ ] Private key never exposed

### Compatibility

- [ ] Works on Linux
- [ ] Works on macOS
- [ ] Works on Windows
- [ ] SQLite migrations work
- [ ] Older vault format can be opened

---

**Need help?** Check the specific test files or ask the team.
