# Coder Agent Documentation

**For**: AI Coding Assistants  
**Version**: 1.0  
**Last Updated**: 2026-03-05

---

## Your Role

You are an expert Go and TypeScript developer working on a secure password manager. Your primary concerns are **security**, **correctness**, and **simplicity**.

---

## Before Writing Any Code

### 1. Read Required Documentation

```bash
# Always read these first:
cat docs/CONTEXT.md          # Project overview and success criteria
cat docs/ARCHITECTURE.md     # Technical architecture  
cat docs/SECURITY_REMEDIATION_PLAN.md  # Active security issues
cat docs/TESTING.md          # Testing requirements
```

### 2. Check Current Status

```bash
# See what's currently in progress
cat docs/STATUS.md

# Check git status
git status
git log --oneline -5
```

### 3. Understand the Scope

Ask yourself:
- What is the minimal change needed?
- Does this touch security-critical code?
- Are there existing tests I should update?
- Does this change affect the API or data format?

---

## Code Standards

### Go Standards

#### Formatting
```bash
# Always run before committing
gofmt -w .
go vet ./...
staticcheck ./...
```

#### Code Style
```go
// Good: Clear, documented, error handling
func EncryptPassword(password string, key []byte) (string, error) {
    if len(password) == 0 {
        return "", fmt.Errorf("password cannot be empty")
    }
    if len(key) != 32 {
        return "", fmt.Errorf("invalid key size: expected 32, got %d", len(key))
    }
    
    block, err := aes.NewCipher(key)
    if err != nil {
        return "", fmt.Errorf("failed to create cipher: %w", err)
    }
    
    // ... rest of implementation
}

// Bad: Panics, no validation, undocumented
func EncryptPassword(password string, key []byte) string {
    block, _ := aes.NewCipher(key)  // Ignoring error!
    // ... panic on error
}
```

#### Error Handling
- Always return errors, never panic
- Wrap errors with context: `fmt.Errorf("context: %w", err)`
- Never log sensitive data (passwords, keys)
- Check all error returns

```go
// Good
result, err := someOperation()
if err != nil {
    return fmt.Errorf("failed to perform operation: %w", err)
}

// Bad
result, _ := someOperation()  // Ignoring error!
```

#### Security-Critical Rules
```go
// NEVER do this:
log.Printf("Password: %s", password)        // ❌ Never log passwords
log.Printf("Key: %x", key)                  // ❌ Never log keys
return fmt.Errorf("wrong password: %s", pwd) // ❌ Don't echo input

// ALWAYS do this:
log.Printf("Operation completed")           // ✅ Generic messages
return fmt.Errorf("authentication failed")  // ✅ Generic errors
```

### TypeScript/React Standards

#### Type Safety
```typescript
// Good: Strict typing
interface Entry {
  id: string;
  site: string;
  username: string;
  encrypted_password: string;
}

async function getEntry(id: string): Promise<Entry | null> {
  const response = await api.get(`/entries/${id}`);
  if (!response.success) {
    return null;
  }
  return response.data as Entry;
}

// Bad: Using any
async function getEntry(id: any): Promise<any> {
  const response = await api.get(`/entries/${id}`);
  return response.data;
}
```

#### React Components
```typescript
// Good: Functional components, typed props
interface PasswordCardProps {
  entry: Entry;
  onCopy: (id: string) => void;
}

export const PasswordCard: React.FC<PasswordCardProps> = ({ 
  entry, 
  onCopy 
}) => {
  return (
    <div className="password-card">
      <h3>{entry.site}</h3>
      <button onClick={() => onCopy(entry.id)}>Copy</button>
    </div>
  );
};
```

---

## Security Checklist

Before submitting any code, verify:

### Input Validation
- [ ] All user inputs validated (length, format, type)
- [ ] No SQL injection (use parameterized queries)
- [ ] No command injection
- [ ] File paths sanitized

### Authentication & Authorization
- [ ] API endpoints require authentication
- [ ] Tokens validated properly
- [ ] CORS configured correctly
- [ ] Rate limiting applied

### Cryptography
- [ ] No hardcoded keys or passwords
- [ ] Random values from crypto/rand, not math/rand
- [ ] Proper key sizes (AES-256, RSA-4096)
- [ ] No logging of sensitive data

### Data Protection
- [ ] Passwords never in plaintext
- [ ] Clipboard cleared after timeout
- [ ] Memory cleared when possible
- [ ] Secure file permissions (0600 for keys)

### Error Handling
- [ ] No stack traces to client
- [ ] Generic error messages
- [ ] Errors don't leak sensitive info
- [ ] All errors logged appropriately

---

## Testing Requirements

### Unit Tests
Every non-trivial function must have tests:

```go
func TestEncryptDecrypt(t *testing.T) {
    key := generateTestKey()
    plaintext := "test password"
    
    encrypted, err := Encrypt(plaintext, key)
    if err != nil {
        t.Fatalf("encrypt failed: %v", err)
    }
    
    decrypted, err := Decrypt(encrypted, key)
    if err != nil {
        t.Fatalf("decrypt failed: %v", err)
    }
    
    if decrypted != plaintext {
        t.Errorf("decrypted %q, want %q", decrypted, plaintext)
    }
}

func TestEncryptWithWrongKey(t *testing.T) {
    key1 := generateTestKey()
    key2 := generateTestKey()
    plaintext := "test"
    
    encrypted, _ := Encrypt(plaintext, key1)
    _, err := Decrypt(encrypted, key2)
    
    if err == nil {
        t.Error("expected error with wrong key")
    }
}
```

### Race Detection
```bash
# Always run with race detector
go test -race ./...
```

### Integration Tests
```go
func TestAPIWorkflow(t *testing.T) {
    // Setup test server
    server := setupTestServer()
    defer server.Close()
    
    // Test full workflow
    // 1. Initialize vault
    // 2. Unlock
    // 3. Add entry
    // 4. Get password
    // 5. Verify
}
```

---

## Common Patterns

### Handler Pattern
```go
// internal/api/handlers.go

func handleGetEntries(w http.ResponseWriter, r *http.Request) {
    // 1. Validate auth
    token := r.Header.Get("Authorization")
    if !auth.ValidateToken(token) {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }
    
    // 2. Check vault unlocked
    if !vault.IsUnlocked() {
        http.Error(w, "vault locked", http.StatusServiceUnavailable)
        return
    }
    
    // 3. Get data
    entries, err := storage.ListEntries()
    if err != nil {
        log.Printf("[ERROR] Failed to list entries: %v", err)
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }
    
    // 4. Return response
    json.NewEncoder(w).Encode(Response{
        Success: true,
        Data: entries,
    })
}
```

### Middleware Pattern
```go
// internal/api/middleware.go

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        token := r.Header.Get("Authorization")
        if !auth.ValidateToken(token) {
            w.WriteHeader(http.StatusUnauthorized)
            json.NewEncoder(w).Encode(Response{
                Success: false,
                Error: "authentication required",
            })
            return
        }
        next(w, r)
    }
}
```

### Storage Pattern
```go
// internal/storage/sqlite.go

type SQLite struct {
    db *sql.DB
    mu sync.RWMutex
}

func (s *SQLite) GetEntry(id string) (*models.PasswordEntry, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    var entry models.PasswordEntry
    err := s.db.QueryRow("SELECT ... FROM entries WHERE id = ?", id).Scan(
        &entry.ID,
        &entry.Site,
        // ...
    )
    if err == sql.ErrNoRows {
        return nil, ErrNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("database error: %w", err)
    }
    
    return &entry, nil
}
```

---

## Database Operations

### Querying
```go
// Good: Parameterized queries
rows, err := db.Query("SELECT * FROM entries WHERE site = ?", site)

// Bad: String concatenation (SQL injection risk)
rows, err := db.Query("SELECT * FROM entries WHERE site = '" + site + "'")
```

### Transactions
```go
func (s *SQLite) CreateEntry(entry *models.PasswordEntry) error {
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()
    
    // Insert entry
    _, err = tx.Exec("INSERT INTO entries ...", entry.ID, ...)
    if err != nil {
        return err
    }
    
    // Insert keys
    for fp, key := range entry.EncryptedAESKeys {
        _, err = tx.Exec("INSERT INTO encrypted_keys ...", entry.ID, fp, key)
        if err != nil {
            return err
        }
    }
    
    return tx.Commit()
}
```

---

## API Design

### Request/Response Format
```json
// Request
{
  "site": "github.com",
  "username": "user",
  "password": "secret"
}

// Success Response
{
  "success": true,
  "data": {
    "id": "uuid",
    "site": "github.com"
  }
}

// Error Response
{
  "success": false,
  "error": "invalid password",
  "code": "INVALID_PASSWORD"
}
```

### Endpoint Structure
```go
// Pattern: /api/<resource>/<action>

// CRUD
GET    /api/entries           // List all
POST   /api/entries/add       // Create
POST   /api/entries/update    // Update
POST   /api/entries/delete    // Delete

// Actions
POST   /api/entries/get_password  // Special action

// Status
GET    /api/p2p/status       // Get status
POST   /api/p2p/start        // Action
```

---

## Debugging

### Logging
```go
// Good: Structured, appropriate level
log.Printf("[INFO] Vault unlocked for device: %s", deviceID)
log.Printf("[ERROR] Failed to decrypt: %v", err)
log.Printf("[DEBUG] Processing entry: %s", entry.ID)

// Bad
fmt.Println("here")  // No context
log.Print(err)       // No prefix
```

### Error Investigation
```bash
# Run with verbose logging
PWMAN_LOG_LEVEL=debug ./pwman-server

# Check race conditions
go run -race ./cmd/server

# Profile performance
go test -bench=. -cpuprofile=cpu.out ./...
go tool pprof cpu.out
```

---

## Git Workflow

### Commit Messages
```
Format: <type>: <description>

Types:
  feat:     New feature
  fix:      Bug fix
  docs:     Documentation
  refactor: Code restructuring
  test:     Tests
  security: Security fix
  perf:     Performance improvement

Examples:
  feat: add device approval workflow
  fix: handle nil pointer in get entry
  security: add API authentication
  refactor: split server into packages
```

### Pre-commit Checklist
```bash
# Run these before every commit
gofmt -w .
go vet ./...
go test -race ./...
git diff --check  # Check for whitespace errors
```

---

## Performance Guidelines

### Avoid Unnecessary Allocations
```go
// Good: Reuse buffers
var buf bytes.Buffer
for i := 0; i < n; i++ {
    buf.Reset()
    buf.WriteString(data[i])
    process(buf.Bytes())
}

// Bad: Allocating in loop
for i := 0; i < n; i++ {
    buf := bytes.NewBufferString(data[i])  // New allocation each iteration
    process(buf.Bytes())
}
```

### Use Concurrency Safely
```go
// Good: Proper synchronization
type SafeCounter struct {
    mu    sync.Mutex
    count int
}

func (c *SafeCounter) Inc() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.count++
}

// Bad: Data race
type UnsafeCounter struct {
    count int
}

func (c *UnsafeCounter) Inc() {
    c.count++  // Race condition!
}
```

---

## Common Mistakes to Avoid

### 1. Ignoring Errors
```go
// Bad
file, _ := os.Open("data.txt")  // Ignoring error!
defer file.Close()

// Good
file, err := os.Open("data.txt")
if err != nil {
    return fmt.Errorf("failed to open file: %w", err)
}
defer file.Close()
```

### 2. Resource Leaks
```go
// Bad: Missing defer
func process() {
    db, _ := sql.Open("sqlite3", "test.db")
    // db never closed!
}

// Good
defunc process() error {
    db, err := sql.Open("sqlite3", "test.db")
    if err != nil {
        return err
    }
    defer db.Close()
    // ... use db ...
    return nil
}
```

### 3. Race Conditions
```go
// Bad: Concurrent access without synchronization
var counter int

go func() { counter++ }()
go func() { counter++ }()

// Good
var counter int64

go func() { atomic.AddInt64(&counter, 1) }()
go func() { atomic.AddInt64(&counter, 1) }()
```

### 4. Timing Attacks
```go
// Bad: Non-constant time comparison
if password == storedPassword {
    // vulnerable to timing attacks
}

// Good: Constant time comparison
if subtle.ConstantTimeCompare([]byte(password), []byte(storedPassword)) == 1 {
    // timing attack resistant
}
```

---

## When to Ask for Help

Ask for clarification when:
1. Requirements are unclear
2. Security implications are uncertain
3. Breaking changes to API/data format
4. Unclear which package/function to modify
5. Test strategy unclear

Always provide context:
- What you're trying to achieve
- What you've tried
- Specific error messages
- Relevant code snippets

---

## Resources

- **Go**: https://golang.org/doc/effective_go.html
- **Security**: https://github.com/OWASP/Go-SCP
- **Testing**: https://golang.org/pkg/testing/
- **Concurrency**: https://golang.org/doc/articles/race_detector.html

---

**Remember**: Security > Performance > Features. When in doubt, choose the safer option.
