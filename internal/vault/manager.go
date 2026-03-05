package vault

import (
	"sync"

	"github.com/bok1c4/pwman/internal/config"
	"github.com/bok1c4/pwman/internal/crypto"
	"github.com/bok1c4/pwman/internal/storage"
)

type Manager struct {
	mu         sync.RWMutex
	vault      *Vault
	isUnlocked bool
}

type Vault struct {
	privateKey *crypto.KeyPair
	storage    *storage.SQLite
	cfg        *config.Config
	vaultName  string
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) GetVault() (*Vault, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.vault == nil {
		return nil, false
	}
	return m.vault, true
}

func (m *Manager) SetVault(v *Vault) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.vault = v
	m.isUnlocked = v != nil
}

func (m *Manager) IsUnlocked() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isUnlocked
}

func (m *Manager) ClearVault() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.vault = nil
	m.isUnlocked = false
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.vault != nil && m.vault.storage != nil {
		err := m.vault.storage.Close()
		m.vault = nil
		m.isUnlocked = false
		return err
	}
	return nil
}

func (v *Vault) GetPrivateKey() *crypto.KeyPair {
	return v.privateKey
}

func (v *Vault) GetStorage() *storage.SQLite {
	return v.storage
}

func (v *Vault) GetConfig() *config.Config {
	return v.cfg
}

func (v *Vault) GetVaultName() string {
	return v.vaultName
}
