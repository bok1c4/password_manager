# Password Manager

A secure, open-source password manager with P2P sync.

---

## Quick Start

See [QUICKSTART.md](QUICKSTART.md) for quick setup instructions.

## Documentation

| File | Description |
|------|-------------|
| [USER_GUIDE.md](USER_GUIDE.md) | Complete user guide |
| [QUICKSTART.md](QUICKSTART.md) | Quick start guide |
| [ARCHITECTURE.md](ARCHITECTURE.md) | System design & architecture |
| [PLAN.md](PLAN.md) | Implementation plan |
| [STATUS.md](STATUS.md) | Development status |
| [BUILD_DEPS.md](BUILD_DEPS.md) | Build dependencies |

## For Developers

| File | Description |
|------|-------------|
| [AGENTS.md](AGENTS.md) | AI agent instructions |
| [INTEGRATION_GUIDE.md](INTEGRATION_GUIDE.md) | Integration guide |

---

## Features

- **Hybrid Encryption**: AES-256-GCM + PGP
- **Multi-Device**: Sync across devices with approval flow
- **P2P Sync**: Peer-to-peer sync (LAN)
- **Git Sync**: Legacy sync via Git
- **Desktop App**: Tauri + React
- **CLI**: Full command-line interface

---

## Build

```bash
# Build CLI
go build -o pwman ./cmd/pwman

# Build server
go build -o server ./cmd/server

# Build desktop app
cd src-tauri && cargo build --release

# Or use Makefile
make build
```

---

## License

MIT
