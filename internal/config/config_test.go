package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVaultPath(t *testing.T) {
	OverrideBasePath = ""

	path := VaultPath("my-vault")
	expected := filepath.Join(DefaultBasePath(), "vaults", "my-vault")
	if path != expected {
		t.Errorf("VaultPath: got %s, want %s", path, expected)
	}
}

func TestPublicKeyPathForVault(t *testing.T) {
	OverrideBasePath = ""

	path := PublicKeyPathForVault("my-vault")
	expected := filepath.Join(DefaultBasePath(), "vaults", "my-vault", "public.key")
	if path != expected {
		t.Errorf("PublicKeyPathForVault: got %s, want %s", path, expected)
	}
}

func TestPrivateKeyPathForVault(t *testing.T) {
	OverrideBasePath = ""

	path := PrivateKeyPathForVault("my-vault")
	expected := filepath.Join(DefaultBasePath(), "vaults", "my-vault", "private.key")
	if path != expected {
		t.Errorf("PrivateKeyPathForVault: got %s, want %s", path, expected)
	}
}

func TestDatabasePathForVault(t *testing.T) {
	OverrideBasePath = ""

	path := DatabasePathForVault("my-vault")
	expected := filepath.Join(DefaultBasePath(), "vaults", "my-vault", "vault.db")
	if path != expected {
		t.Errorf("DatabasePathForVault: got %s, want %s", path, expected)
	}
}

func TestGlobalConfig(t *testing.T) {
	tmpDir := t.TempDir()
	OverrideBasePath = tmpDir

	cfg := &GlobalConfig{
		Vaults:      []string{"vault1", "vault2"},
		ActiveVault: "vault1",
	}

	configPath := filepath.Join(tmpDir, "config.json")

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	loaded, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if len(loaded.Vaults) != 2 {
		t.Errorf("Expected 2 vaults, got %d", len(loaded.Vaults))
	}

	if loaded.ActiveVault != "vault1" {
		t.Errorf("Expected active vault 'vault1', got %s", loaded.ActiveVault)
	}
}

func TestGlobalConfigAddVault(t *testing.T) {
	cfg := &GlobalConfig{
		Vaults: []string{"vault1"},
	}

	cfg.AddVault("vault2")
	if len(cfg.Vaults) != 2 {
		t.Errorf("Expected 2 vaults after AddVault, got %d", len(cfg.Vaults))
	}

	cfg.AddVault("vault1")
	if len(cfg.Vaults) != 2 {
		t.Errorf("Adding duplicate should not increase count, got %d", len(cfg.Vaults))
	}
}

func TestGlobalConfigRemoveVault(t *testing.T) {
	cfg := &GlobalConfig{
		Vaults: []string{"vault1", "vault2"},
	}

	cfg.RemoveVault("vault1")
	if len(cfg.Vaults) != 1 {
		t.Errorf("Expected 1 vault after RemoveVault, got %d", len(cfg.Vaults))
	}

	if cfg.Vaults[0] != "vault2" {
		t.Errorf("Expected remaining vault to be 'vault2', got %s", cfg.Vaults[0])
	}
}

func TestVaultConfig(t *testing.T) {
	tmpDir := t.TempDir()
	OverrideBasePath = tmpDir

	cfg := &VaultConfig{
		DeviceID:   "device-123",
		DeviceName: "Test Device",
		Salt:       "random-salt",
	}

	vaultName := "test-vault"
	if err := cfg.SaveForVault(vaultName); err != nil {
		t.Fatalf("Failed to save vault config: %v", err)
	}

	loaded, err := LoadVaultConfig(vaultName)
	if err != nil {
		t.Fatalf("Failed to load vault config: %v", err)
	}

	if loaded.DeviceID != "device-123" {
		t.Errorf("DeviceID mismatch: got %s, want device-123", loaded.DeviceID)
	}

	if loaded.DeviceName != "Test Device" {
		t.Errorf("DeviceName mismatch: got %s, want Test Device", loaded.DeviceName)
	}
}

func TestSetActiveVault(t *testing.T) {
	tmpDir := t.TempDir()
	OverrideBasePath = tmpDir

	cfg := &GlobalConfig{
		Vaults: []string{"vault1", "vault2"},
	}

	configPath := filepath.Join(tmpDir, "config.json")

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	if err := SetActiveVault("vault2"); err != nil {
		t.Fatalf("Failed to set active vault: %v", err)
	}

	loaded, _ := LoadGlobalConfig()
	if loaded.ActiveVault != "vault2" {
		t.Errorf("Active vault should be 'vault2', got %s", loaded.ActiveVault)
	}
}

func TestVaultExists(t *testing.T) {
	tmpDir := t.TempDir()
	OverrideBasePath = tmpDir

	cfg := &GlobalConfig{
		Vaults: []string{"existing-vault"},
	}
	configPath := filepath.Join(tmpDir, "config.json")
	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0644)

	vaultPath := VaultPath("existing-vault")
	os.MkdirAll(vaultPath, 0755)

	if !VaultExists("existing-vault") {
		t.Error("VaultExists should return true for existing vault in config")
	}

	if VaultExists("non-existing-vault") {
		t.Error("VaultExists should return false for non-existing vault")
	}
}

func TestEnsureVaultDirForVault(t *testing.T) {
	tmpDir := t.TempDir()
	OverrideBasePath = tmpDir

	vaultName := "new-vault"
	if err := EnsureVaultDirForVault(vaultName); err != nil {
		t.Fatalf("Failed to ensure vault dir: %v", err)
	}

	expectedPath := VaultPath(vaultName)
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Vault directory should exist at %s", expectedPath)
	}
}

func TestGetActiveVault(t *testing.T) {
	tmpDir := t.TempDir()
	OverrideBasePath = tmpDir

	cfg := &GlobalConfig{
		Vaults:      []string{"vault1", "vault2"},
		ActiveVault: "vault1",
	}

	configPath := filepath.Join(tmpDir, "config.json")

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	active, err := GetActiveVault()
	if err != nil {
		t.Fatalf("Failed to get active vault: %v", err)
	}

	if active != "vault1" {
		t.Errorf("Expected 'vault1', got %s", active)
	}
}
