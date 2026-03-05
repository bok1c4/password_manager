package main

import (
	"crypto/rsa"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bok1c4/pwman/internal/crypto"
	"github.com/bok1c4/pwman/internal/storage"
	"github.com/bok1c4/pwman/pkg/models"
)

func TestDevicePublicKeyStorage(t *testing.T) {
	tmpDir := t.TempDir()

	vaultName := "test-vault"

	vaultPath := filepath.Join(tmpDir, "vaults", vaultName)
	os.MkdirAll(vaultPath, 0755)

	keyPair, err := crypto.GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	publicKeyPath := filepath.Join(vaultPath, "public.key")
	privateKeyPath := filepath.Join(vaultPath, "private.key")

	if err := crypto.SavePublicKey(keyPair.PublicKey, publicKeyPath); err != nil {
		t.Fatalf("Failed to save public key: %v", err)
	}
	if err := crypto.SavePrivateKey(keyPair.PrivateKey, privateKeyPath); err != nil {
		t.Fatalf("Failed to save private key: %v", err)
	}

	// FIXED: Read actual PEM content and store it (simulating what handleInit should do)
	pemContent, err := os.ReadFile(publicKeyPath)
	if err != nil {
		t.Fatalf("Failed to read public key: %v", err)
	}

	dbPath := filepath.Join(vaultPath, "vault.db")
	db, err := storage.NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	deviceID := "test-device-id"
	deviceName := "Test Device"

	device := models.Device{
		ID:          deviceID,
		Name:        deviceName,
		PublicKey:   string(pemContent), // FIXED: Store actual PEM content, not file path
		Fingerprint: crypto.GetFingerprint(keyPair.PublicKey),
		Trusted:     true,
		CreatedAt:   time.Now(),
	}

	if err := db.UpsertDevice(&device); err != nil {
		t.Fatalf("Failed to upsert device: %v", err)
	}

	retrieved, err := db.GetDevice(deviceID)
	if err != nil {
		t.Fatalf("Failed to get device: %v", err)
	}

	// Verify the public key is the actual PEM content
	if strings.HasPrefix(retrieved.PublicKey, "/") {
		t.Errorf("BUG: Device PublicKey contains file path '%s' instead of actual PEM content", retrieved.PublicKey)
	}

	if !strings.Contains(retrieved.PublicKey, "-----BEGIN") {
		t.Errorf("BUG: Device PublicKey does not contain PEM header. Got: %s", retrieved.PublicKey)
	}

	if retrieved.PublicKey != string(pemContent) {
		t.Errorf("Device PublicKey should match the PEM file content")
	}
}

func TestDevicePublicKeyShouldContainPEMContent(t *testing.T) {
	tmpDir := t.TempDir()

	vaultName := "test-vault"

	vaultPath := filepath.Join(tmpDir, "vaults", vaultName)
	os.MkdirAll(vaultPath, 0755)

	keyPair, err := crypto.GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	publicKeyPath := filepath.Join(vaultPath, "public.key")
	privateKeyPath := filepath.Join(vaultPath, "private.key")

	if err := crypto.SavePublicKey(keyPair.PublicKey, publicKeyPath); err != nil {
		t.Fatalf("Failed to save public key: %v", err)
	}
	if err := crypto.SavePrivateKey(keyPair.PrivateKey, privateKeyPath); err != nil {
		t.Fatalf("Failed to save private key: %v", err)
	}

	pemContent, err := os.ReadFile(publicKeyPath)
	if err != nil {
		t.Fatalf("Failed to read public key: %v", err)
	}

	dbPath := filepath.Join(vaultPath, "vault.db")
	db, err := storage.NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	deviceID := "test-device-id"
	deviceName := "Test Device"

	device := models.Device{
		ID:          deviceID,
		Name:        deviceName,
		PublicKey:   string(pemContent),
		Fingerprint: crypto.GetFingerprint(keyPair.PublicKey),
		Trusted:     true,
		CreatedAt:   time.Now(),
	}

	if err := db.UpsertDevice(&device); err != nil {
		t.Fatalf("Failed to upsert device: %v", err)
	}

	retrieved, err := db.GetDevice(deviceID)
	if err != nil {
		t.Fatalf("Failed to get device: %v", err)
	}

	if !strings.Contains(retrieved.PublicKey, "-----BEGIN") {
		t.Errorf("Device PublicKey should contain PEM header. Got: %s", retrieved.PublicKey)
	}

	if retrieved.PublicKey != string(pemContent) {
		t.Errorf("Device PublicKey should match the PEM file content")
	}
}

func TestEntryCRUDWithEncryption(t *testing.T) {
	tmpDir := t.TempDir()

	vaultName := "test-vault"
	vaultPath := filepath.Join(tmpDir, "vaults", vaultName)
	os.MkdirAll(vaultPath, 0755)

	keyPair, err := crypto.GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	publicKeyPath := filepath.Join(vaultPath, "public.key")
	privateKeyPath := filepath.Join(vaultPath, "private.key")

	if err := crypto.SavePublicKey(keyPair.PublicKey, publicKeyPath); err != nil {
		t.Fatalf("Failed to save public key: %v", err)
	}
	if err := crypto.SavePrivateKey(keyPair.PrivateKey, privateKeyPath); err != nil {
		t.Fatalf("Failed to save private key: %v", err)
	}

	pemContent, err := os.ReadFile(publicKeyPath)
	if err != nil {
		t.Fatalf("Failed to read public key: %v", err)
	}

	dbPath := filepath.Join(vaultPath, "vault.db")
	db, err := storage.NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	deviceID := "test-device-id"
	device := models.Device{
		ID:          deviceID,
		Name:        "Test Device",
		PublicKey:   string(pemContent),
		Fingerprint: crypto.GetFingerprint(keyPair.PublicKey),
		Trusted:     true,
		CreatedAt:   time.Now(),
	}
	if err := db.UpsertDevice(&device); err != nil {
		t.Fatalf("Failed to upsert device: %v", err)
	}

	getPublicKey := func(fingerprint string) (*rsa.PublicKey, error) {
		if fingerprint == device.Fingerprint {
			return keyPair.PublicKey, nil
		}
		return nil, fmt.Errorf("device not found")
	}

	encrypted, err := crypto.HybridEncrypt("my-secret-password", []models.Device{device}, getPublicKey)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	entry := &models.PasswordEntry{
		ID:                "entry-1",
		Version:           1,
		Site:              "github.com",
		Username:          "testuser",
		EncryptedPassword: encrypted.EncryptedPassword,
		EncryptedAESKeys:  encrypted.EncryptedAESKeys,
		Notes:             "Test note",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		UpdatedBy:         deviceID,
	}

	if err := db.CreateEntry(entry); err != nil {
		t.Fatalf("Failed to create entry: %v", err)
	}

	retrieved, err := db.GetEntry(entry.ID)
	if err != nil {
		t.Fatalf("Failed to get entry: %v", err)
	}
	if retrieved.Site != "github.com" {
		t.Errorf("Site mismatch: got %s, want github.com", retrieved.Site)
	}
	if retrieved.Username != "testuser" {
		t.Errorf("Username mismatch: got %s, want testuser", retrieved.Username)
	}

	decrypted, err := crypto.HybridDecrypt(retrieved, keyPair.PrivateKey)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}
	if decrypted != "my-secret-password" {
		t.Errorf("Decrypted password mismatch: got %s, want my-secret-password", decrypted)
	}

	retrieved.Site = "gitlab.com"
	if err := db.UpdateEntry(retrieved); err != nil {
		t.Fatalf("Failed to update entry: %v", err)
	}

	updated, _ := db.GetEntry(entry.ID)
	if updated.Site != "gitlab.com" {
		t.Errorf("Update failed: got %s, want gitlab.com", updated.Site)
	}

	if err := db.DeleteEntry(entry.ID); err != nil {
		t.Fatalf("Failed to delete entry: %v", err)
	}

	entries, _ := db.ListEntries()
	if len(entries) != 0 {
		t.Errorf("Expected 0 entries after delete, got %d", len(entries))
	}
}

func TestListEntriesFiltersDeleted(t *testing.T) {
	tmpDir := t.TempDir()

	vaultName := "test-vault"
	vaultPath := filepath.Join(tmpDir, "vaults", vaultName)
	os.MkdirAll(vaultPath, 0755)

	dbPath := filepath.Join(vaultPath, "vault.db")
	db, err := storage.NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	entry1 := &models.PasswordEntry{
		ID:                "entry-1",
		Site:              "site1.com",
		EncryptedPassword: "enc1",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	deletedAt := time.Now()
	entry2 := &models.PasswordEntry{
		ID:                "entry-2",
		Site:              "site2.com",
		EncryptedPassword: "enc2",
		DeletedAt:         &deletedAt,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	db.CreateEntry(entry1)
	db.CreateEntry(entry2)

	entries, err := db.ListEntries()
	if err != nil {
		t.Fatalf("Failed to list entries: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("Expected 1 active entry, got %d", len(entries))
	}
	if entries[0].Site != "site1.com" {
		t.Errorf("Expected site1.com, got %s", entries[0].Site)
	}
}
