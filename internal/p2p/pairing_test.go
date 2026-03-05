package p2p_test

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bok1c4/pwman/internal/crypto"
	"github.com/bok1c4/pwman/internal/storage"
	"github.com/bok1c4/pwman/pkg/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFingerprintIsPublicKeyPrefix(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fingerprint_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	keyPair, err := crypto.GenerateRSAKeyPair(4096)
	require.NoError(t, err)

	fingerprint := crypto.GetFingerprint(keyPair.PublicKey)

	assert.Contains(t, fingerprint, "MIICCgKCAgEA", "fingerprint should start with public key prefix")
	assert.NotEqual(t, len(fingerprint), 36, "fingerprint should not be a UUID (too short)")
}

func TestDeviceInsertionIsIdempotent(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "device_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "vault.db")
	db, err := storage.NewSQLite(dbPath)
	require.NoError(t, err)
	defer db.Close()

	device := models.Device{
		ID:          "test-device-id",
		Name:        "Test Device",
		PublicKey:   "test-public-key",
		Fingerprint: "test-fingerprint",
		Trusted:     true,
		CreatedAt:   time.Now(),
	}

	err = db.UpsertDevice(&device)
	require.NoError(t, err, "first insertion should succeed")

	err = db.UpsertDevice(&device)
	require.NoError(t, err, "second insertion should succeed")

	devices, err := db.ListDevices()
	require.NoError(t, err, "listing devices should succeed")
	assert.Len(t, devices, 1, "should have exactly 1 device after duplicate insertions")

	assert.Equal(t, devices[0].ID, device.ID, "device ID should match")
	assert.Equal(t, devices[0].Fingerprint, device.Fingerprint, "fingerprint should match")
}

func TestVaultSetupCreatesCorrectFingerprint(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vault_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	vaultPath := filepath.Join(tempDir, "vault")
	dbPath := filepath.Join(vaultPath, "vault.db")

	db, err := storage.NewSQLite(dbPath)
	require.NoError(t, err)
	defer db.Close()

	keyPair, err := crypto.GenerateRSAKeyPair(4096)
	require.NoError(t, err)

	privateKeyPath := filepath.Join(vaultPath, "private_key.pem")
	err = crypto.SavePrivateKey(keyPair.PrivateKey, privateKeyPath)
	require.NoError(t, err)

	publicKeyPath := filepath.Join(vaultPath, "public_key.pem")
	err = crypto.SavePublicKey(keyPair.PublicKey, publicKeyPath)
	require.NoError(t, err)

	salt := make([]byte, 32)
	_, err = rand.Read(salt)
	require.NoError(t, err)

	deviceID := fmt.Sprintf("device-%s", uuid.New().String())
	pubKeyBytes, err := os.ReadFile(publicKeyPath)
	require.NoError(t, err)

	expectedFingerprint := crypto.GetFingerprint(keyPair.PublicKey)

	device := models.Device{
		ID:          deviceID,
		Name:        "Test Device",
		PublicKey:   string(pubKeyBytes),
		Fingerprint: expectedFingerprint,
		Trusted:     true,
		CreatedAt:   time.Now(),
	}
	err = db.UpsertDevice(&device)
	require.NoError(t, err)

	devices, err := db.ListDevices()
	require.NoError(t, err)
	assert.Len(t, devices, 1, "should have 1 device")

	storedDevice := devices[0]
	assert.Equal(t, storedDevice.Fingerprint, expectedFingerprint, "stored fingerprint should match computed fingerprint")
	assert.Contains(t, storedDevice.Fingerprint, "MIICCgKCAgEA", "fingerprint should start with public key prefix")
	assert.NotEqual(t, len(storedDevice.Fingerprint), 36, "fingerprint should not be a UUID (too short)")
}
