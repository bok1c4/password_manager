package device

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"math/big"
	"time"

	"github.com/bok1c4/pwman/internal/config"
	"github.com/bok1c4/pwman/internal/crypto"
	"github.com/bok1c4/pwman/internal/storage"
	"github.com/bok1c4/pwman/pkg/models"
	"github.com/google/uuid"
)

type Manager struct {
	storage    storage.Storage
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	deviceID   string
	deviceName string
}

func NewManager(storage storage.Storage, privateKey *rsa.PrivateKey, deviceID, deviceName string) *Manager {
	return &Manager{
		storage:    storage,
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
		deviceID:   deviceID,
		deviceName: deviceName,
	}
}

func InitVault(cfg *config.Config, password string) error {
	if err := config.EnsureVaultDir(); err != nil {
		return fmt.Errorf("failed to create vault directory: %w", err)
	}

	keyPair, err := crypto.GenerateRSAKeyPair(4096)
	if err != nil {
		return fmt.Errorf("failed to generate key pair: %w", err)
	}

	deviceID := uuid.New().String()

	salt, err := crypto.EncryptPrivateKeyAndSave(keyPair.PrivateKey, password, config.PrivateKeyPath())
	if err != nil {
		return fmt.Errorf("failed to encrypt and save private key: %w", err)
	}

	if err := crypto.SavePublicKey(keyPair.PublicKey, config.PublicKeyPath()); err != nil {
		return fmt.Errorf("failed to save public key: %w", err)
	}

	cfg.DeviceID = deviceID
	cfg.Salt = base64.StdEncoding.EncodeToString(salt)
	active, _ := config.GetActiveVault()
	if err := cfg.SaveForVault(active); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	db, err := storage.NewSQLite(config.DatabasePath())
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}
	defer db.Close()

	device := &models.Device{
		ID:          deviceID,
		Name:        cfg.DeviceName,
		PublicKey:   config.PublicKeyPath(),
		Fingerprint: crypto.GetFingerprint(keyPair.PublicKey),
		CreatedAt:   time.Now(),
		Trusted:     true,
	}

	if err := db.UpsertDevice(device); err != nil {
		return fmt.Errorf("failed to save device: %w", err)
	}

	if err := db.UpsertMeta(models.MetaVaultID, deviceID); err != nil {
		return fmt.Errorf("failed to save vault ID: %w", err)
	}

	return nil
}

func LoadVault(password string) (*Manager, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}
	if cfg == nil {
		return nil, nil, fmt.Errorf("vault not initialized. Run 'pwman init' first")
	}

	privateKey, err := crypto.LoadAndDecryptPrivateKey(password, config.PrivateKeyPath())
	if err != nil {
		return nil, nil, fmt.Errorf("wrong password or corrupted key")
	}

	db, err := storage.NewSQLite(config.DatabasePath())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	return NewManager(db, privateKey, cfg.DeviceID, cfg.DeviceName), cfg, nil
}

func ValidatePassword(password string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if cfg == nil {
		return fmt.Errorf("vault not initialized")
	}

	_, err = crypto.LoadAndDecryptPrivateKey(password, config.PrivateKeyPath())
	if err != nil {
		return fmt.Errorf("wrong password")
	}
	return nil
}

func IsVaultInitialized() bool {
	cfg, _ := config.Load()
	return cfg != nil
}

func (m *Manager) GetDevice() (*models.Device, error) {
	return m.storage.GetDevice(m.deviceID)
}

func (m *Manager) ListDevices() ([]models.Device, error) {
	return m.storage.ListDevices()
}

func (m *Manager) AddDevice(publicKeyData []byte, name string) (*models.Device, error) {
	device := &models.Device{
		ID:          uuid.New().String(),
		Name:        name,
		PublicKey:   string(publicKeyData),
		Fingerprint: crypto.GetFingerprint(m.publicKey),
		CreatedAt:   time.Now(),
		Trusted:     false,
	}

	if err := m.storage.UpsertDevice(device); err != nil {
		return nil, fmt.Errorf("failed to add device: %w", err)
	}

	return device, nil
}

func (m *Manager) TrustDevice(deviceID string) error {
	device, err := m.storage.GetDevice(deviceID)
	if err != nil {
		return fmt.Errorf("failed to get device: %w", err)
	}

	device.Trusted = true
	return m.storage.UpsertDevice(device)
}

func (m *Manager) GetTrustedDevices() ([]models.Device, error) {
	devices, err := m.storage.ListDevices()
	if err != nil {
		return nil, err
	}

	var trusted []models.Device
	for _, d := range devices {
		if d.Trusted {
			trusted = append(trusted, d)
		}
	}
	return trusted, nil
}

func (m *Manager) GetStorage() storage.Storage {
	return m.storage
}

func (m *Manager) GetPrivateKey() *rsa.PrivateKey {
	return m.privateKey
}

func (m *Manager) GetDeviceID() string {
	return m.deviceID
}

func (m *Manager) AddDeviceWithApproval(publicKeyData []byte, name string) (*models.Device, string, error) {
	approvalCode := generateApprovalCode()

	device := &models.Device{
		ID:           uuid.New().String(),
		Name:         name,
		PublicKey:    string(publicKeyData),
		Fingerprint:  crypto.GetFingerprint(m.publicKey),
		CreatedAt:    time.Now(),
		Trusted:      false,
		ApprovalCode: approvalCode,
	}

	if err := m.storage.UpsertDevice(device); err != nil {
		return nil, "", fmt.Errorf("failed to add device: %w", err)
	}

	return device, approvalCode, nil
}

func (m *Manager) ApproveDevice(approvalCode string) (string, error) {
	devices, err := m.storage.ListDevices()
	if err != nil {
		return "", err
	}

	for _, d := range devices {
		if d.ApprovalCode == approvalCode {
			d.Trusted = true
			d.ApprovalCode = ""
			if err := m.storage.UpsertDevice(&d); err != nil {
				return "", err
			}
			return d.ID, nil
		}
	}

	return "", fmt.Errorf("invalid approval code")
}

func (m *Manager) ReEncryptForNewDevice(deviceFingerprint string) error {
	entries, err := m.storage.ListEntries()
	if err != nil {
		return err
	}

	devices, err := m.storage.ListDevices()
	if err != nil {
		return err
	}

	var trustedDevices []models.Device
	for _, d := range devices {
		if d.Trusted {
			trustedDevices = append(trustedDevices, d)
		}
	}

	getPublicKey := func(fingerprint string) (*rsa.PublicKey, error) {
		for _, d := range trustedDevices {
			if d.Fingerprint == fingerprint {
				return crypto.LoadPublicKey(d.PublicKey)
			}
		}
		return nil, fmt.Errorf("device not found")
	}

	for _, entry := range entries {
		password, err := crypto.HybridDecrypt(&entry, m.privateKey)
		if err != nil {
			continue
		}

		encrypted, err := crypto.HybridEncrypt(password, trustedDevices, getPublicKey)
		if err != nil {
			continue
		}

		entry.EncryptedPassword = encrypted.EncryptedPassword
		entry.EncryptedAESKeys = encrypted.EncryptedAESKeys
		entry.Version++
		entry.UpdatedAt = time.Now()
		entry.UpdatedBy = m.deviceID

		if err := m.storage.UpdateEntry(&entry); err != nil {
			continue
		}
	}

	return nil
}

func generateApprovalCode() string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, 6)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		result[i] = chars[n.Int64()]
	}
	return string(result)
}
