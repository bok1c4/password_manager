package identity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateIdentity(t *testing.T) {
	id, err := GenerateIdentity()
	require.NoError(t, err)
	require.NotNil(t, id)

	// Check Ed25519 keys
	assert.NotNil(t, id.SignPublicKey)
	assert.NotNil(t, id.SignPrivateKey)
	assert.Len(t, id.SignPublicKey, 32)
	assert.Len(t, id.SignPrivateKey, 64)

	// Check X25519 keys
	assert.NotEqual(t, [32]byte{}, id.BoxPublicKey)
	assert.NotEqual(t, [32]byte{}, id.BoxPrivateKey)

	// Check fingerprint
	assert.Len(t, id.Fingerprint, 16) // 8 bytes = 16 hex chars
	assert.Regexp(t, "^[0-9a-f]{16}$", id.Fingerprint)
}

func TestX25519KeyDerivation(t *testing.T) {
	id, err := GenerateIdentity()
	require.NoError(t, err)

	// Verify X25519 key is NOT a direct copy of Ed25519 seed
	ed25519Seed := id.SignPrivateKey.Seed()
	x25519Priv := id.BoxPrivateKey[:]

	// Should NOT be direct copy
	assert.NotEqual(t, ed25519Seed[:32], x25519Priv,
		"X25519 key should be hashed, not copied from Ed25519 seed")

	// Verify clamping was applied per RFC 7748
	assert.Equal(t, byte(0), x25519Priv[0]&7,
		"First byte should have bits 0-2 cleared")
	assert.NotEqual(t, byte(0), x25519Priv[31]&64,
		"Last byte should have bit 6 set")
	assert.Equal(t, byte(0), x25519Priv[31]&128,
		"Last byte should have bit 7 cleared")
}

func TestIdentitySaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "identity.key")

	// Generate identity
	id1, err := GenerateIdentity()
	require.NoError(t, err)

	// Save
	err = id1.Save(path)
	require.NoError(t, err)

	// Load
	id2, err := LoadIdentity(path)
	require.NoError(t, err)

	// Verify keys match
	assert.Equal(t, id1.SignPublicKey, id2.SignPublicKey)
	assert.Equal(t, id1.SignPrivateKey.Seed(), id2.SignPrivateKey.Seed())
	assert.Equal(t, id1.BoxPublicKey, id2.BoxPublicKey)
	assert.Equal(t, id1.BoxPrivateKey, id2.BoxPrivateKey)
	assert.Equal(t, id1.Fingerprint, id2.Fingerprint)
}

func TestSignAndVerify(t *testing.T) {
	id, err := GenerateIdentity()
	require.NoError(t, err)

	message := []byte("test message")

	// Sign
	signature := id.Sign(message)
	assert.Len(t, signature, 64) // Ed25519 signatures are 64 bytes

	// Verify
	assert.True(t, id.Verify(message, signature))

	// Verify with wrong message should fail
	wrongMessage := []byte("wrong message")
	assert.False(t, id.Verify(wrongMessage, signature))

	// Verify with wrong signature should fail
	wrongSignature := make([]byte, 64)
	assert.False(t, id.Verify(message, wrongSignature))
}

func TestGetBoxFingerprint(t *testing.T) {
	id, err := GenerateIdentity()
	require.NoError(t, err)

	fp := id.GetBoxFingerprint()
	assert.Len(t, fp, 16)
	assert.Regexp(t, "^[0-9a-f]{16}$", fp)

	// Should be different from Ed25519 fingerprint
	assert.NotEqual(t, id.Fingerprint, fp)
}

func TestGetBoxPublicKeyBytes(t *testing.T) {
	id, err := GenerateIdentity()
	require.NoError(t, err)

	b := id.GetBoxPublicKeyBytes()
	assert.Len(t, b, 32)
	assert.Equal(t, b, id.BoxPublicKey[:])
}

func TestGetBoxPrivateKey(t *testing.T) {
	id, err := GenerateIdentity()
	require.NoError(t, err)

	ptr := id.GetBoxPrivateKey()
	assert.NotNil(t, ptr)
	assert.Equal(t, id.BoxPrivateKey, *ptr)
}

func TestLoadIdentity_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nonexistent.key")

	_, err := LoadIdentity(path)
	assert.Error(t, err)
}

func TestLoadIdentity_InvalidPEM(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "invalid.key")

	// Write invalid data
	err := os.WriteFile(path, []byte("not valid pem"), 0600)
	require.NoError(t, err)

	_, err = LoadIdentity(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}
