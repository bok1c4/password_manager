package crypto

import (
	"crypto/rsa"
	"os"
	"path/filepath"
	"testing"

	"github.com/bok1c4/pwman/pkg/models"
)

func TestGenerateRSAKeyPair(t *testing.T) {
	keyPair, err := GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key pair: %v", err)
	}

	if keyPair.PrivateKey == nil {
		t.Fatal("Private key is nil")
	}

	if keyPair.PublicKey == nil {
		t.Fatal("Public key is nil")
	}
}

func TestSaveAndLoadKeys(t *testing.T) {
	tmpDir := t.TempDir()
	privateKeyPath := filepath.Join(tmpDir, "private.key")
	publicKeyPath := filepath.Join(tmpDir, "public.key")

	keyPair, err := GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	if err := SavePrivateKey(keyPair.PrivateKey, privateKeyPath); err != nil {
		t.Fatalf("Failed to save private key: %v", err)
	}

	if err := SavePublicKey(keyPair.PublicKey, publicKeyPath); err != nil {
		t.Fatalf("Failed to save public key: %v", err)
	}

	loadedPrivateKey, err := LoadPrivateKey(privateKeyPath)
	if err != nil {
		t.Fatalf("Failed to load private key: %v", err)
	}

	loadedPublicKey, err := LoadPublicKey(publicKeyPath)
	if err != nil {
		t.Fatalf("Failed to load public key: %v", err)
	}

	if loadedPrivateKey.N.Cmp(keyPair.PrivateKey.N) != 0 {
		t.Error("Loaded private key doesn't match original")
	}

	if loadedPublicKey.N.Cmp(keyPair.PublicKey.N) != 0 {
		t.Error("Loaded public key doesn't match original")
	}
}

func TestRSAEncryptDecrypt(t *testing.T) {
	keyPair, err := GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	plaintext := []byte("Hello, World! This is a test message.")

	ciphertext, err := RSAEncrypt(plaintext, keyPair.PublicKey)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	decrypted, err := RSADecrypt(ciphertext, keyPair.PrivateKey)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypted text doesn't match original: got %s, want %s", decrypted, plaintext)
	}
}

func TestGenerateAESKey(t *testing.T) {
	key, err := GenerateAESKey()
	if err != nil {
		t.Fatalf("Failed to generate AES key: %v", err)
	}

	if len(key) != AESKeySize {
		t.Errorf("Key length mismatch: got %d, want %d", len(key), AESKeySize)
	}
}

func TestAESEncryptDecrypt(t *testing.T) {
	key, err := GenerateAESKey()
	if err != nil {
		t.Fatalf("Failed to generate AES key: %v", err)
	}

	plaintext := []byte("My secret password!")

	encrypted, err := AESEncrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	if encrypted == "" {
		t.Fatal("Encrypted string is empty")
	}

	decrypted, err := AESDecrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypted text doesn't match original")
	}
}

func TestAESDecryptWithWrongKey(t *testing.T) {
	key1, _ := GenerateAESKey()
	key2, _ := GenerateAESKey()

	plaintext := []byte("Secret message")
	encrypted, _ := AESEncrypt(plaintext, key1)

	_, err := AESDecrypt(encrypted, key2)
	if err == nil {
		t.Error("Should have failed with wrong key")
	}
}

func TestHybridEncryptDecrypt(t *testing.T) {
	keyPair, err := GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	fingerprint := GetFingerprint(keyPair.PublicKey)

	tmpDir := t.TempDir()
	publicKeyPath := filepath.Join(tmpDir, "public.key")
	publicKeyPath2 := filepath.Join(tmpDir, "public2.key")

	SavePublicKey(keyPair.PublicKey, publicKeyPath)
	SavePublicKey(keyPair.PublicKey, publicKeyPath2)

	device := models.Device{
		ID:          "device-1",
		Name:        "Test Device",
		Fingerprint: fingerprint,
		Trusted:     true,
		PublicKey:   publicKeyPath,
	}

	devices := []models.Device{device}

	getPublicKey := func(fp string) (*rsa.PublicKey, error) {
		for _, d := range devices {
			if d.Fingerprint == fp {
				return LoadPublicKey(d.PublicKey)
			}
		}
		return nil, os.ErrNotExist
	}

	password := "MySuperSecretPassword123!"

	encrypted, err := HybridEncrypt(password, devices, getPublicKey)
	if err != nil {
		t.Fatalf("Failed to hybrid encrypt: %v", err)
	}

	if encrypted.EncryptedPassword == "" {
		t.Error("Encrypted password is empty")
	}

	if len(encrypted.EncryptedAESKeys) == 0 {
		t.Error("No encrypted AES keys")
	}

	decrypted, err := HybridDecrypt(&models.PasswordEntry{
		EncryptedPassword: encrypted.EncryptedPassword,
		EncryptedAESKeys:  encrypted.EncryptedAESKeys,
	}, keyPair.PrivateKey)
	if err != nil {
		t.Fatalf("Failed to hybrid decrypt: %v", err)
	}

	if decrypted != password {
		t.Errorf("Decrypted password doesn't match: got %s, want %s", decrypted, password)
	}
}

func TestGetFingerprint(t *testing.T) {
	keyPair1, _ := GenerateRSAKeyPair(2048)
	keyPair2, _ := GenerateRSAKeyPair(2048)

	fp1 := GetFingerprint(keyPair1.PublicKey)
	fp2 := GetFingerprint(keyPair2.PublicKey)

	if fp1 == "" {
		t.Error("Fingerprint is empty")
	}

	if fp1 == fp2 {
		t.Error("Different keys should have different fingerprints")
	}
}
