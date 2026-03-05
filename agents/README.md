# AI Agent Instructions

This document is for AI assistants working on this password manager project.

## Quick Start

1. Read `docs/CONTEXT.md` first
2. Check `docs/STATUS.md` for current state
3. Review `docs/CODER_AGENT.md` for coding standards
4. See `docs/SECURITY_REMEDIATION_PLAN.md` for security priorities

## Available Agent Prompts

- **Builder Agent**: `agents/builder_prompt.md` - For feature implementation and bug fixes
- **Testing Agent**: `agents/testing_prompt.md` - For writing and maintaining tests

## Documentation Structure

```
docs/
├── CONTEXT.md                    # Project overview, success criteria
├── ARCHITECTURE.md               # Technical architecture
├── TESTING.md                    # Testing guide
├── SECURITY_REMEDIATION_PLAN.md  # Security fixes (critical!)
├── STATUS.md                     # Current development status
├── USER_GUIDE.md                 # End user documentation
└── CODER_AGENT.md                # Coding standards

agents/
├── builder_prompt.md             # Builder agent prompt
└── testing_prompt.md             # Testing agent prompt
```

## Communication

- Be concise and specific
- Reference exact file paths and line numbers
- Ask questions when requirements are unclear
- Prioritize security and correctness over features

## Key Principles

1. **Security First** - Never compromise on security
2. **Test Everything** - All code must be tested
3. **Document Changes** - Update docs when changing behavior
4. **Follow Standards** - Consistent code style

## Emergency Contacts

If you find a security vulnerability:
1. Document it immediately
2. Check if it's in SECURITY_REMEDIATION_PLAN.md
3. Prioritize fixing CRITICAL issues first

---

Last Updated: 2026-03-05
