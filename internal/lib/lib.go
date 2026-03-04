package lib

/*
#cgo CFLAGS: -g -Wall
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/bok1c4/pwman/internal/config"
	"github.com/bok1c4/pwman/internal/crypto"
	"github.com/bok1c4/pwman/internal/storage"
	"github.com/bok1c4/pwman/pkg/models"
	"github.com/google/uuid"
)

var (
	vault     *Vault
	vaultLock sync.Mutex
)

type Vault struct {
	privateKey *rsa.PrivateKey
	storage    *storage.SQLite
	cfg        *config.Config
}

type Entry struct {
	ID                string            `json:"id"`
	Site              string            `json:"site"`
	Username          string            `json:"username"`
	Password          string            `json:"password,omitempty"`
	EncryptedPassword string            `json:"encrypted_password"`
	EncryptedAESKeys  map[string]string `json:"encrypted_aes_keys"`
	Notes             string            `json:"notes"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	UpdatedBy         string            `json:"updated_by"`
}

type Device struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	PublicKey   string    `json:"public_key"`
	Fingerprint string    `json:"fingerprint"`
	Trusted     bool      `json:"trusted"`
	CreatedAt   time.Time `json:"created_at"`
}

type SyncStatus struct {
	Initialized    bool  `json:"initialized"`
	LastSync       int64 `json:"last_sync"`
	PendingChanges int   `json:"pending_changes"`
}

func getVaultPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "."
	}
	return home + "/.pwman"
}

func getConfigPath() string {
	return getVaultPath() + "/config.json"
}

func getPrivateKeyPath() string {
	return getVaultPath() + "/private.key"
}

func getPublicKeyPath() string {
	return getVaultPath() + "/public.key"
}

func getDatabasePath() string {
	return getVaultPath() + "/vault.db"
}

//export InitVault
func InitVault(name string, password string) string {
	vaultLock.Lock()
	defer vaultLock.Unlock()

	if vault != nil {
		return `{"error": "vault already initialized"}`
	}

	_, err := os.Stat(getConfigPath())
	if err == nil {
		return `{"error": "vault already exists"}`
	}

	vaultPath := getVaultPath()
	if err := os.MkdirAll(vaultPath, 0700); err != nil {
		return fmt.Sprintf(`{"error": "failed to create vault: %s"}`, err.Error())
	}

	keyPair, err := crypto.GenerateRSAKeyPair(2048)
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to generate keys: %s"}`, err.Error())
	}

	privateKeyPath := getPrivateKeyPath()
	publicKeyPath := getPublicKeyPath()

	if err := crypto.SavePrivateKey(keyPair.PrivateKey, privateKeyPath); err != nil {
		return fmt.Sprintf(`{"error": "failed to save private key: %s"}`, err.Error())
	}

	if err := crypto.SavePublicKey(keyPair.PublicKey, publicKeyPath); err != nil {
		return fmt.Sprintf(`{"error": "failed to save public key: %s"}`, err.Error())
	}

	deviceID := uuid.New().String()
	cfg := &config.Config{
		DeviceID:   deviceID,
		DeviceName: name,
	}

	active, _ := config.GetActiveVault()
	cfgBytes, _ := json.Marshal(cfg)
	os.WriteFile(config.VaultConfigPath(active), cfgBytes, 0600)

	db, err := storage.NewSQLite(getDatabasePath())
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to create database: %s"}`, err.Error())
	}

	device := models.Device{
		ID:          deviceID,
		Name:        name,
		PublicKey:   publicKeyPath,
		Fingerprint: crypto.GetFingerprint(keyPair.PublicKey),
		Trusted:     true,
		CreatedAt:   time.Now(),
	}

	if err := db.UpsertDevice(&device); err != nil {
		return fmt.Sprintf(`{"error": "failed to save device: %s"}`, err.Error())
	}

	vault = &Vault{
		privateKey: keyPair.PrivateKey,
		storage:    db,
		cfg:        cfg,
	}

	return fmt.Sprintf(`{"success": true, "device_id": "%s"}`, deviceID)
}

//export UnlockVault
func UnlockVault(password string) bool {
	vaultLock.Lock()
	defer vaultLock.Unlock()

	if vault != nil && vault.privateKey != nil {
		return true
	}

	_, err := os.Stat(getConfigPath())
	if err != nil {
		return false
	}

	privateKey, err := crypto.LoadPrivateKey(getPrivateKeyPath())
	if err != nil {
		return false
	}

	db, err := storage.NewSQLite(getDatabasePath())
	if err != nil {
		return false
	}

	cfgBytes, _ := os.ReadFile(getConfigPath())
	var cfg config.Config
	json.Unmarshal(cfgBytes, &cfg)

	vault = &Vault{
		privateKey: privateKey,
		storage:    db,
		cfg:        &cfg,
	}

	return true
}

//export LockVault
func LockVault() {
	vaultLock.Lock()
	defer vaultLock.Unlock()
	vault = nil
}

//export IsUnlocked
func IsUnlocked() bool {
	vaultLock.Lock()
	defer vaultLock.Unlock()
	return vault != nil && vault.privateKey != nil
}

//export IsInitialized
func IsInitialized() bool {
	_, err := os.Stat(getConfigPath())
	return err == nil
}

//export GetEntries
func GetEntries() string {
	vaultLock.Lock()
	defer vaultLock.Unlock()

	if vault == nil || vault.storage == nil {
		return `[]`
	}

	entries, err := vault.storage.ListEntries()
	if err != nil {
		return `[]`
	}

	result := make([]Entry, len(entries))
	for i, e := range entries {
		result[i] = Entry{
			ID:                e.ID,
			Site:              e.Site,
			Username:          e.Username,
			EncryptedPassword: e.EncryptedPassword,
			EncryptedAESKeys:  e.EncryptedAESKeys,
			Notes:             e.Notes,
			CreatedAt:         e.CreatedAt,
			UpdatedAt:         e.UpdatedAt,
			UpdatedBy:         e.UpdatedBy,
		}
	}

	data, _ := json.Marshal(result)
	return string(data)
}

//export AddEntry
func AddEntry(site, username, password, notes string) string {
	vaultLock.Lock()
	defer vaultLock.Unlock()

	if vault == nil || vault.storage == nil {
		return `{"error": "vault not unlocked"}`
	}

	devices, err := vault.storage.ListDevices()
	if err != nil {
		return fmt.Sprintf(`{"error": "%s"}`, err.Error())
	}

	var trustedDevices []models.Device
	for _, d := range devices {
		if d.Trusted {
			trustedDevices = append(trustedDevices, d)
		}
	}

	if len(trustedDevices) == 0 {
		pubKey := &vault.privateKey.PublicKey
		activeVault, _ := config.GetActiveVault()
		trustedDevices = append(trustedDevices, models.Device{
			ID:          vault.cfg.DeviceID,
			Fingerprint: crypto.GetFingerprint(pubKey),
			PublicKey:   config.PublicKeyPathForVault(activeVault),
		})
	}

	getPublicKey := func(fingerprint string) (*rsa.PublicKey, error) {
		for _, d := range trustedDevices {
			if d.Fingerprint == fingerprint {
				return crypto.LoadPublicKey(d.PublicKey)
			}
		}
		return nil, fmt.Errorf("device not found")
	}

	encrypted, err := crypto.HybridEncrypt(password, trustedDevices, getPublicKey)
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to encrypt: %s"}`, err.Error())
	}

	entry := models.PasswordEntry{
		ID:                uuid.New().String(),
		Version:           1,
		Site:              site,
		Username:          username,
		EncryptedPassword: encrypted.EncryptedPassword,
		EncryptedAESKeys:  encrypted.EncryptedAESKeys,
		Notes:             notes,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		UpdatedBy:         vault.cfg.DeviceID,
	}

	if err := vault.storage.CreateEntry(&entry); err != nil {
		return fmt.Sprintf(`{"error": "%s"}`, err.Error())
	}

	return fmt.Sprintf(`{"success": true, "id": "%s"}`, entry.ID)
}

//export DeleteEntry
func DeleteEntry(id string) bool {
	vaultLock.Lock()
	defer vaultLock.Unlock()

	if vault == nil || vault.storage == nil {
		return false
	}

	return vault.storage.DeleteEntry(id) == nil
}

//export GetDevices
func GetDevices() string {
	vaultLock.Lock()
	defer vaultLock.Unlock()

	if vault == nil || vault.storage == nil {
		return `[]`
	}

	devices, err := vault.storage.ListDevices()
	if err != nil {
		return `[]`
	}

	result := make([]Device, len(devices))
	for i, d := range devices {
		result[i] = Device{
			ID:          d.ID,
			Name:        d.Name,
			PublicKey:   d.PublicKey,
			Fingerprint: d.Fingerprint,
			Trusted:     d.Trusted,
			CreatedAt:   d.CreatedAt,
		}
	}

	data, _ := json.Marshal(result)
	return string(data)
}

//export GetSyncStatus
func GetSyncStatus() string {
	// TODO: Integrate with P2P status
	return fmt.Sprintf(`{"mode": "p2p", "connected_peers": 0, "last_sync": null}`)
}

//export GeneratePassword
func GeneratePassword(length int) string {
	if length < 4 {
		length = 16
	}
	password, err := crypto.GenerateStrongPassword(length)
	if err != nil {
		return ""
	}
	return password
}

//export GetConfig
func GetConfig() string {
	if vault == nil || vault.cfg == nil {
		return `{}`
	}

	data, _ := json.Marshal(vault.cfg)
	return string(data)
}
