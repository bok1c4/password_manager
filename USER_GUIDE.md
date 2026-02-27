# Password Manager - User Guide

A secure password manager with hybrid encryption (AES-256-GCM + PGP) and P2P sync.

---

## Table of Contents

1. [Installation](#installation)
2. [Quick Start](#quick-start)
3. [Desktop App](#desktop-app)
4. [CLI Usage](#cli-usage)
5. [P2P Sync Setup](#p2p-sync-setup)
6. [Multi-Device Flow](#multi-device-flow)
7. [Git Sync (Legacy)](#git-sync-legacy)
8. [Security](#security)
9. [Troubleshooting](#troubleshooting)

---

## Installation

### Option 1: Build from Source

```bash
# Clone and build
go build -o pwman ./cmd/pwman

# Or use Makefile
make build
```

### Option 2: Desktop App (Tauri)

```bash
# Install dependencies
npm install

# Build the desktop app
make tauri
```

The desktop app is located at: `src-tauri/target/release/pwman`

---

## Quick Start

### First Time Setup (Desktop App)

1. **Start the API server** (required for desktop app):
   ```bash
   ./server
   ```
   The server runs on `http://localhost:18475`

2. **Launch the desktop app**:
   ```bash
   ./src-tauri/target/release/pwman
   ```

3. **Initialize your vault**:
   - Enter your device name (e.g., "Desktop")
   - Create a strong password
   - Click "Create Vault"

4. **Add passwords**:
   - Click "+" to add new entries
   - Fill in site, username, password
   - Click "Save"

---

## Desktop App

The Tauri desktop app provides a GUI with:

| Feature | Description |
|---------|-------------|
| Password List | View all saved passwords |
| Add/Edit/Delete | Manage password entries |
| Copy to Clipboard | Click copy icon, auto-clears after 30s |
| Multiple Vaults | Support for separate vaults |
| P2P Sync | Peer-to-peer device sync |
| Device Management | Add/trust new devices |

### Starting the App

1. Start the Go API server first:
   ```bash
   ./server
   ```

2. Run the desktop app:
   ```bash
   ./src-tauri/target/release/pwman
   ```

### Vault Management

- **Switch vaults**: Use the vault dropdown in the top-right
- **Create new vault**: Click vault name → "Create New Vault"
- Each vault has its own:
  - Password database
  - Encryption keys
  - Device trust relationships

---

## CLI Usage

### Initialize Vault

```bash
./pwman init --name "My Desktop"
```

This creates:
- `~/.pwman/` - Vault directory
- `~/.pwman/vault.db` - SQLite database  
- `~/.pwman/config.json` - Configuration
- `~/.pwman/private.key` - Your private key (encrypted with your password)
- `~/.pwman/public.key` - Your public key

### Add a Password

```bash
# With password
./pwman add github.com -u myusername -p mypassword

# Prompt for password
./pwman add github.com -u myusername

# Generate secure password
./pwman add github.com -u myusername --generate
./pwman add github.com -u myusername -g -l 32  # 32 characters
```

### Get a Password

```bash
./pwman get github.com

# Copy to clipboard (auto-clears after 30s)
./pwman get github.com -c
```

### List Entries

```bash
./pwman list
```

### Edit a Password

```bash
./pwman edit github.com -p newpassword
```

### Delete a Password

```bash
./pwman delete github.com
```

---

## P2P Sync Setup

P2P sync allows direct device-to-device communication without a central server.

### Prerequisites

1. Both devices must be running the API server
2. Devices must be on the same network (LAN) or reachable via NAT

### Step 1: Start P2P on Device A

```bash
# Start the API server
./server

# In another terminal, start P2P
./pwman p2p start

# Get your peer ID
./pwman p2p status
```

### Step 2: Start P2P on Device B

```bash
./server
./pwman p2p start
```

### Step 3: Connect Devices

**Option A: Via IP Address (Same Network)**

On Device B, connect to Device A:
```bash
./pwman p2p connect /ip4/192.168.1.100/tcp/0/p2p/QmPeerID...
```

**Option B: Using mDNS (Auto-Discovery)**

If on the same network, devices may auto-discover via mDNS.

### Step 4: Approve Device

When Device B connects, Device A will see a pending approval request:

**Via CLI:**
```bash
# List pending approvals
./pwman p2p approvals

# Approve device
./pwman p2p approve <device-id>

# Or reject
./pwman p2p reject <device-id> --reason "Not authorized"
```

**Via Desktop App:**
1. Open Settings tab
2. See "Pending Approvals" section
3. Click "Approve" or "Reject"

### Step 5: Sync

Once approved, passwords sync automatically between trusted devices.

```bash
# Manual sync
./pwman p2p sync
```

---

## Multi-Device Flow

### Overview

```
DEVICE A                    DEVICE B
─────────                   ─────────

1. init --name "Arch"      
   → Creates vault          

2. add github.com          
   → Encrypts for A         
                           
3. Start P2P               
   → Starts server          
                           
                           4. init --name "Mac"
                              → Creates own vault
                              
                           5. Start P2P
                           
6. p2p connect to B        
   → Connection request     

                           7. See approval request
                           8. Approve Device A
                           
9. p2p sync                
   → Transfers encrypted   
     passwords              
                           
                           10. Can now decrypt 
                               all passwords!
```

### Detailed Steps

#### On Device A (First Device)

```bash
# 1. Initialize
./pwman init --name "Arch"

# 2. Add passwords
./pwman add github.com -u me -p secret123

# 3. Start P2P
./pwman p2p start

# 4. Get peer info
./pwman p2p status
# Output: Peer ID: QmABCD123...
```

#### On Device B (New Device)

```bash
# 1. Initialize
./pwman init --name "MacBook"

# 2. Start P2P
./pwman p2p start

# 3. Connect to Device A
./pwman p2p connect /ip4/192.168.1.X/tcp/0/p2p/QmABCD123...
```

#### Approval (Either Device)

```bash
# 4. Approve the other device
./pwman p2p approve <peer-id>

# 5. Sync
./pwman p2p sync
```

Now both devices can access the same passwords!

---

## Git Sync (Legacy)

If P2P isn't suitable, you can use Git-based sync.

### Initialize Sync

```bash
./pwman sync init https://github.com/username/pwman-vault.git
```

### Sync Commands

```bash
# Pull changes
./pwman sync pull

# Push changes
./pwman sync push -m "Added new password"

# Full sync
./pwman sync -m "Updated passwords"

# Check status
./pwman sync status
```

### Git Sync Flow

```
DEVICE A                    DEVICE B
─────────                   ─────────

1. init --name "A"         
2. add github.com          
3. sync init repo          
4. sync push               
                           
                           5. sync init repo
                           6. sync pull
                           7. init --name "B"
                           8. devices add A.pub
                           9. devices trust <id>
                           10. sync pull
                           
                           → Now has passwords!
```

---

## Security

### Encryption

- **AES-256-GCM** for password encryption
- **RSA-4096** (PGP) for key exchange
- **Scrypt** (N=16384, r=8, p=1) for key derivation

### Private Key

Your private key is stored at `~/.pwman/private.key` and encrypted with your password.

**NEVER share your private key!**

### Device Trust

When you add a new device:
1. The new device can only see encrypted passwords
2. You must explicitly approve the device
3. Upon approval, all passwords are re-encrypted for the new device
4. Use approval codes to verify the connection

### Clipboard Security

- Copy to clipboard auto-clears after **30 seconds**
- Use `-c` flag with `get` command

---

## File Locations

| Path | Description |
|------|-------------|
| `~/.pwman/` | Vault directory |
| `~/.pwman/vault.db` | SQLite database |
| `~/.pwman/config.json` | Configuration |
| `~/.pwman/private.key` | Private key (SECRET - never share) |
| `~/.pwman/public.key` | Public key (safe to share) |

---

## Troubleshooting

### "Vault not initialized"

Run `pwman init --name "Your Device"` first.

### "Failed to decrypt password"

- Ensure you're using a trusted device
- The password may have been added by an untrusted device

### "Failed to get entry"

- Check the site name is correct
- Entry may have been deleted

### P2P Connection Issues

1. **Check firewall**: Ensure ports are open
2. **Same network**: For direct connection, devices must be on same LAN
3. **NAT traversal**: P2P uses NAT traversal, but some networks may block it
4. **Check peer IDs**: Ensure you're using the correct peer ID

### Desktop App Won't Start

1. Make sure the API server is running:
   ```bash
   ./server
   ```

2. Check if port 18475 is available

### Lost Password

If you forget your vault password:
- **There is no recovery** - your passwords are encrypted with your password
- The private key cannot be decrypted without your password
- This is intentional for security

---

## Commands Reference

### CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize new vault |
| `add` | Add password entry |
| `get` | Get password |
| `list` | List all entries |
| `edit` | Edit entry |
| `delete` | Delete entry |
| `devices` | Manage devices |
| `vaults` | Manage vaults |
| `p2p` | P2P sync commands |
| `sync` | Git sync commands |
| `import` | Import from other formats |

### P2P Commands

| Command | Description |
|---------|-------------|
| `p2p start` | Start P2P server |
| `p2p stop` | Stop P2P server |
| `p2p status` | Show P2P status |
| `p2p connect <addr>` | Connect to peer |
| `p2p disconnect` | Disconnect from peer |
| `p2p peers` | List connected peers |
| `p2p approvals` | List pending approvals |
| `p2p approve <id>` | Approve device |
| `p2p reject <id>` | Reject device |
| `p2p sync` | Sync with peers |

### Device Commands

| Command | Description |
|---------|-------------|
| `devices list` | List devices |
| `devices export` | Export public key |
| `devices add` | Add new device |
| `devices trust` | Trust device |
| `devices revoke` | Revoke device |
