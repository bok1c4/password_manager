package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

var (
	// OverrideBasePath allows testing with different vault directories
	OverrideBasePath = os.Getenv("PWMAN_BASE_PATH")
)

type GlobalConfig struct {
	ActiveVault string   `json:"active_vault"`
	Vaults      []string `json:"vaults"`
}

type VaultConfig struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	Salt       string `json:"salt,omitempty"`
}

var (
	AppName = "pwman"
)

func DefaultBasePath() string {
	if OverrideBasePath != "" {
		return OverrideBasePath
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, "."+AppName)
}

func VaultsDir() string {
	return filepath.Join(DefaultBasePath(), "vaults")
}

func GlobalConfigPath() string {
	return filepath.Join(DefaultBasePath(), "config.json")
}

func VaultPath(vaultName string) string {
	return filepath.Join(VaultsDir(), vaultName)
}

func VaultConfigPath(vaultName string) string {
	return filepath.Join(VaultPath(vaultName), "config.json")
}

func DatabasePathForVault(vaultName string) string {
	return filepath.Join(VaultPath(vaultName), "vault.db")
}

func PrivateKeyPathForVault(vaultName string) string {
	return filepath.Join(VaultPath(vaultName), "private.key")
}

func PublicKeyPathForVault(vaultName string) string {
	return filepath.Join(VaultPath(vaultName), "public.key")
}

func LoadGlobalConfig() (*GlobalConfig, error) {
	configPath := GlobalConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &GlobalConfig{Vaults: []string{}}, nil
		}
		return nil, fmt.Errorf("failed to read global config: %w", err)
	}

	var cfg GlobalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse global config: %w", err)
	}

	return &cfg, nil
}

func (c *GlobalConfig) Save() error {
	configPath := GlobalConfigPath()

	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (c *GlobalConfig) AddVault(name string) {
	for _, v := range c.Vaults {
		if v == name {
			return
		}
	}
	c.Vaults = append(c.Vaults, name)
}

func (c *GlobalConfig) RemoveVault(name string) {
	newVaults := []string{}
	for _, v := range c.Vaults {
		if v != name {
			newVaults = append(newVaults, v)
		}
	}
	c.Vaults = newVaults
	if c.ActiveVault == name {
		if len(c.Vaults) > 0 {
			c.ActiveVault = c.Vaults[0]
		} else {
			c.ActiveVault = ""
		}
	}
}

func LoadVaultConfig(vaultName string) (*VaultConfig, error) {
	configPath := VaultConfigPath(vaultName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read vault config: %w", err)
	}

	var cfg VaultConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse vault config: %w", err)
	}

	return &cfg, nil
}

func (c *VaultConfig) SaveForVault(vaultName string) error {
	configPath := VaultConfigPath(vaultName)

	if err := os.MkdirAll(VaultPath(vaultName), 0700); err != nil {
		return fmt.Errorf("failed to create vault directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func EnsureVaultDirForVault(vaultName string) error {
	return os.MkdirAll(VaultPath(vaultName), 0700)
}

func GetActiveVault() (string, error) {
	cfg, err := LoadGlobalConfig()
	if err != nil {
		return "", err
	}
	if cfg.ActiveVault == "" && len(cfg.Vaults) > 0 {
		cfg.ActiveVault = cfg.Vaults[0]
		cfg.Save()
	}
	return cfg.ActiveVault, nil
}

func SetActiveVault(name string) error {
	cfg, err := LoadGlobalConfig()
	if err != nil {
		return err
	}

	found := false
	for _, v := range cfg.Vaults {
		if v == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("vault '%s' does not exist", name)
	}

	cfg.ActiveVault = name
	return cfg.Save()
}

func VaultExists(name string) bool {
	cfg, _ := LoadGlobalConfig()
	for _, v := range cfg.Vaults {
		if v == name {
			return true
		}
	}
	return false
}

func GetPlatform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

// Legacy aliases for backward compatibility
type Config = VaultConfig

func Load() (*VaultConfig, error) {
	active, err := GetActiveVault()
	if err != nil {
		return nil, err
	}
	if active == "" {
		return nil, nil
	}
	return LoadVaultConfig(active)
}

func DefaultVaultPath() string {
	active, _ := GetActiveVault()
	if active == "" {
		return DefaultBasePath()
	}
	return VaultPath(active)
}

func PrivateKeyPath() string {
	active, _ := GetActiveVault()
	if active == "" {
		return filepath.Join(DefaultBasePath(), "private.key")
	}
	return PrivateKeyPathForVault(active)
}

func PublicKeyPath() string {
	active, _ := GetActiveVault()
	if active == "" {
		return filepath.Join(DefaultBasePath(), "public.key")
	}
	return PublicKeyPathForVault(active)
}

func DatabasePath() string {
	active, _ := GetActiveVault()
	if active == "" {
		return filepath.Join(DefaultBasePath(), "vault.db")
	}
	return DatabasePathForVault(active)
}

func EnsureVaultDir() error {
	active, err := GetActiveVault()
	if err != nil {
		return err
	}
	if active == "" {
		return os.MkdirAll(DefaultBasePath(), 0700)
	}
	return EnsureVaultDirForVault(active)
}

// Legacy support - convert old config to new format
func MigrateLegacyConfig() error {
	legacyPath := filepath.Join(DefaultBasePath(), "config.json")

	_, err := os.Stat(legacyPath)
	if os.IsNotExist(err) {
		return nil // No legacy config
	}

	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return err
	}

	var legacy VaultConfig
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}

	if legacy.DeviceID == "" {
		return nil
	}

	cfg := &GlobalConfig{
		ActiveVault: "default",
		Vaults:      []string{"default"},
	}

	if err := cfg.Save(); err != nil {
		return err
	}

	legacy.SaveForVault("default")

	os.Rename(legacyPath, legacyPath+".bak")

	return nil
}
