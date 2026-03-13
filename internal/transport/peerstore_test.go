package transport

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPeerStore(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "trusted_peers.json")

	// Create new store
	ps, err := NewPeerStore(path)
	require.NoError(t, err)
	assert.NotNil(t, ps)

	// Should start empty
	assert.Empty(t, ps.ListTrusted())
}

func TestPeerStore_TrustAndIsTrusted(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "trusted_peers.json")

	ps, err := NewPeerStore(path)
	require.NoError(t, err)

	// Initially not trusted
	assert.False(t, ps.IsTrusted("abc123"))

	// Trust a peer
	err = ps.Trust("abc123", "Device-A", "device-id-123")
	require.NoError(t, err)

	// Now trusted
	assert.True(t, ps.IsTrusted("abc123"))

	// Verify peer details
	peer, ok := ps.GetPeer("abc123")
	assert.True(t, ok)
	assert.Equal(t, "Device-A", peer.DeviceName)
	assert.Equal(t, "device-id-123", peer.DeviceID)
	assert.Equal(t, "abc123", peer.Fingerprint)
	assert.WithinDuration(t, time.Now(), peer.PinnedAt, time.Second)
}

func TestPeerStore_Untrust(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "trusted_peers.json")

	ps, err := NewPeerStore(path)
	require.NoError(t, err)

	// Trust then untrust
	ps.Trust("abc123", "Device-A", "device-id-123")
	assert.True(t, ps.IsTrusted("abc123"))

	err = ps.Untrust("abc123")
	require.NoError(t, err)

	// No longer trusted
	assert.False(t, ps.IsTrusted("abc123"))

	// Peer should not exist
	_, ok := ps.GetPeer("abc123")
	assert.False(t, ok)
}

func TestPeerStore_ListTrusted(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "trusted_peers.json")

	ps, err := NewPeerStore(path)
	require.NoError(t, err)

	// Add multiple peers
	ps.Trust("fp1", "Device-1", "id-1")
	ps.Trust("fp2", "Device-2", "id-2")
	ps.Trust("fp3", "Device-3", "id-3")

	// List all
	peers := ps.ListTrusted()
	assert.Len(t, peers, 3)

	// Check that all are present
	fingerprints := make(map[string]bool)
	for _, p := range peers {
		fingerprints[p.Fingerprint] = true
	}
	assert.True(t, fingerprints["fp1"])
	assert.True(t, fingerprints["fp2"])
	assert.True(t, fingerprints["fp3"])
}

func TestPeerStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "trusted_peers.json")

	// Create and populate first store
	ps1, err := NewPeerStore(path)
	require.NoError(t, err)

	ps1.Trust("abc123", "Device-A", "device-id-123")
	ps1.Trust("def456", "Device-B", "device-id-456")

	// Create second store pointing to same file
	ps2, err := NewPeerStore(path)
	require.NoError(t, err)

	// Should load persisted peers
	assert.True(t, ps2.IsTrusted("abc123"))
	assert.True(t, ps2.IsTrusted("def456"))

	peer, ok := ps2.GetPeer("abc123")
	assert.True(t, ok)
	assert.Equal(t, "Device-A", peer.DeviceName)
}

func TestCertFingerprint(t *testing.T) {
	// Test with known data
	data := []byte("test certificate data")
	fp := CertFingerprint(data)

	// Should be 64 hex characters (SHA-256 = 32 bytes = 64 hex chars)
	assert.Len(t, fp, 64)

	// Should be valid hex
	for _, c := range fp {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'))
	}

	// Same data should produce same fingerprint
	fp2 := CertFingerprint(data)
	assert.Equal(t, fp, fp2)

	// Different data should produce different fingerprint
	data2 := []byte("different data")
	fp3 := CertFingerprint(data2)
	assert.NotEqual(t, fp, fp3)
}

func TestPeerStore_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "trusted_peers.json")

	ps, err := NewPeerStore(path)
	require.NoError(t, err)

	// Concurrent writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			fp := fmt.Sprintf("fp%d", i)
			ps.Trust(fp, fmt.Sprintf("Device-%d", i), fmt.Sprintf("id-%d", i))
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// All should be trusted
	assert.Len(t, ps.ListTrusted(), 10)
}
