# Claude Code Analysis Prompt: pwman Security Overhaul Review

## Context

You are analyzing a **P2P password manager** called `pwman` that has undergone a comprehensive 4-phase security overhaul. The backend (Go) implementation is complete and production-ready. Now we need to assess the frontend (Rust/Tauri) and provide integration guidance.

## Project Structure

```
/home/user/Projects/fun-projects/password_manager/
├── docs/
│   ├── ARCHITECTURE.md          # Current system architecture
│   ├── REFACTORING_SUMMARY.md   # What was built in 4 phases
│   └── README.md                # Original project readme
├── src-tauri/                   # RUST FRONTEND (needs analysis)
│   ├── src/
│   │   ├── main.rs
│   │   ├── commands.rs
│   │   └── app_state.rs
│   └── Cargo.toml
├── internal/                    # Go backend (COMPLETE)
│   ├── pairing/                 # TOTP implementation
│   ├── transport/               # TLS + certificate pinning
│   ├── discovery/               # mDNS discovery
│   ├── identity/                # Ed25519/X25519 keys
│   ├── sync/                    # Lamport clocks
│   └── p2p/                     # Dual-mode P2P (libp2p + TLS)
└── cmd/server/                  # HTTP API handlers
```

## Your Task

Perform a comprehensive analysis of:
1. **Backend Implementation** (Go) - Review completeness and correctness
2. **System Architecture** - Assess overall design
3. **Frontend Code** (Rust/Tauri) - Identify what needs updating
4. **Integration Points** - Frontend ↔ Backend communication

## Key Documents to Review

**Required Reading (in order):**
1. `docs/ARCHITECTURE.md` - Current system design
2. `docs/REFACTORING_SUMMARY.md` - What was built and why
3. `src-tauri/src/` - Current Rust frontend code
4. `cmd/server/handlers/` - HTTP API endpoints

## Analysis Framework

### 1. Backend Assessment (Go)

Review each new package and provide:
- ✅ **Correctness**: Is the implementation sound?
- ✅ **Security**: Are there any vulnerabilities?
- ✅ **Completeness**: Is everything implemented as described?
- ⚠️ **Issues**: Any bugs, missing error handling, or edge cases?
- 💡 **Improvements**: Suggestions for better code

**Focus Areas:**
- `internal/pairing/totp.go` - TOTP generation
- `internal/transport/peerstore.go` - Certificate pinning
- `internal/identity/identity.go` - Ed25519/X25519 keys
- `internal/sync/clock.go` - Lamport clocks
- `internal/p2p/p2p.go` - Dual-mode P2P implementation

### 2. Architecture Review

Assess the overall system:
- **Security Model**: Is the threat model adequately addressed?
- **Scalability**: Will this architecture handle growth?
- **Maintainability**: Is the code organized and documented?
- **Integration Points**: How do components interact?

### 3. Frontend Analysis (Rust/Tauri)

**Critical Task:** Identify all frontend changes needed to support the new backend.

Current frontend location: `src-tauri/src/`

**Review These Files:**
- `main.rs` - Application entry point
- `commands.rs` - Tauri commands (frontend → backend)
- `app_state.rs` - Application state management
- `Cargo.toml` - Dependencies

**Questions to Answer:**
1. What API endpoints does the frontend currently use?
2. What data structures does it expect?
3. What needs to change for TOTP pairing (6-digit codes, not 9-char)?
4. How should certificate fingerprint verification be implemented?
5. What state needs to track the new security features?
6. Are there any hardcoded assumptions that break with new backend?

### 4. Integration Assessment

Analyze the frontend-backend interface:
- **HTTP API**: Which endpoints changed?
- **Data Flow**: How does data flow between Rust frontend and Go backend?
- **State Sync**: How is state synchronized?
- **Error Handling**: How should new errors be handled?

## Specific Deliverables

Provide your analysis in this format:

### 1. Executive Summary
- Overall system health (1-2 paragraphs)
- Critical issues (if any)
- Frontend readiness level

### 2. Backend Code Review
For each package, provide:
```
Package: internal/pairing
Status: ✅ Production Ready | ⚠️ Needs Work | ❌ Critical Issues
Findings:
- [Specific finding 1]
- [Specific finding 2]
Recommendations:
- [Recommendation 1]
```

### 3. Architecture Assessment
```
Strengths:
- [Strength 1]
- [Strength 2]

Concerns:
- [Concern 1]
- [Concern 2]

Questions:
- [Question that needs clarification]
```

### 4. Frontend Update Requirements

**CRITICAL SECTION:** Provide detailed, actionable guidance.

```
## Required Changes

### 1. Pairing Flow Updates
Current: [What exists now]
Required: [What needs to change]
Implementation:
```rust
// Example code showing the change
```

### 2. API Endpoint Changes
Changed Endpoints:
- POST /api/pairing/generate
  - Old response: {code: "XXX-XXX-XXX", expires_in: 300}
  - New response: {code: "123456", expires_in: 60}
  - Frontend impact: [What to change]

### 3. State Management Updates
New State Required:
- certificateFingerprint: String
- trustedPeers: Vec<PinnedPeer>
- logicalClock: u64

Implementation Guide:
```rust
// How to update app_state.rs
```

### 4. UI Changes Needed
- [UI change 1 with mock description]
- [UI change 2 with mock description]

### 5. Dependency Updates
Cargo.toml changes:
```toml
# Add these dependencies
[dependencies]
# ...
```
```

### 5. Testing Recommendations
- What tests should be added to frontend?
- How to test TOTP integration?
- How to test certificate pinning?

### 6. Migration Guide
Step-by-step instructions for updating the frontend:
1. Step 1
2. Step 2
3. ...

## Constraints & Assumptions

- **Backend is complete and frozen** - Do not suggest backend changes unless critical bugs
- **Maintain backward compatibility** where possible
- **Security is priority #1** - All changes must maintain or improve security
- **Rust/Tauri 1.5+** - Use modern Tauri patterns
- **No breaking changes** to user workflows unless necessary

## Success Criteria

Your analysis is successful if:
✅ All 4 phases are correctly understood
✅ Backend implementation is thoroughly reviewed
✅ Frontend gaps are clearly identified
✅ Specific, actionable code examples are provided
✅ Migration path is clear and feasible
✅ Security implications are considered

## Output Format

Structure your response as a comprehensive technical document with:
1. Executive Summary (top)
2. Detailed findings (middle)
3. Code examples and migration guide (bottom)

Use code blocks for all code examples. Be specific with line numbers and file paths where possible.

## Questions to Consider

As you analyze, keep these questions in mind:

1. **Security**: Does the frontend properly handle sensitive data (keys, passwords)?
2. **UX**: How will users experience the new TOTP and fingerprint verification flows?
3. **State**: Is frontend state consistent with backend state?
4. **Error Handling**: Are all new error cases handled gracefully?
5. **Testing**: How can we verify the integration works correctly?
6. **Documentation**: What needs to be documented for developers?

## Scope Boundaries

**IN SCOPE:**
- Backend code review (all of `internal/`)
- Architecture assessment
- Frontend analysis (`src-tauri/`)
- Integration guidance
- Testing recommendations

**OUT OF SCOPE:**
- Implementing the changes (you're analyzing, not coding)
- Performance benchmarking
- Deployment configuration
- CI/CD setup
- Non-Rust frontends

## Start Your Analysis

Begin by reading the documentation, then analyze the code, and finally provide your comprehensive assessment.

Remember: The goal is to provide clear, actionable guidance for updating the Rust frontend to work with the new secure backend.
