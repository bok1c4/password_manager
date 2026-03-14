package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"

	"github.com/bok1c4/pwman/pkg/models"
	"golang.org/x/crypto/nacl/box"
)

type EncryptedData struct {
	EncryptedPassword string
	EncryptedAESKeys  map[string]string
}

func HybridEncrypt(password string, devices []models.Device, getPublicKey func(string) (*rsa.PublicKey, error)) (*EncryptedData, error) {
	aesKey, err := GenerateAESKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate AES key: %w", err)
	}

	encryptedPassword, err := AESEncrypt([]byte(password), aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt password: %w", err)
	}

	encryptedKeys := make(map[string]string)
	for _, device := range devices {
		if !device.Trusted {
			continue
		}

		publicKey, err := getPublicKey(device.Fingerprint)
		if err != nil {
			return nil, fmt.Errorf("failed to get public key for device %s: %w", device.ID, err)
		}

		encryptedKey, err := RSAEncrypt(aesKey, publicKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt AES key for device %s: %w", device.ID, err)
		}

		encryptedKeys[device.Fingerprint] = base64.StdEncoding.EncodeToString(encryptedKey)
	}

	if len(encryptedKeys) == 0 {
		return nil, fmt.Errorf("no trusted devices to encrypt for")
	}

	return &EncryptedData{
		EncryptedPassword: encryptedPassword,
		EncryptedAESKeys:  encryptedKeys,
	}, nil
}

func HybridDecrypt(entry *models.PasswordEntry, privateKey *rsa.PrivateKey) (string, error) {
	fingerprint := GetFingerprint(&privateKey.PublicKey)
	encryptedKeyBase64, ok := entry.EncryptedAESKeys[fingerprint]
	if !ok {
		return "", fmt.Errorf("no encrypted key for this device")
	}

	encryptedKey, err := base64.StdEncoding.DecodeString(encryptedKeyBase64)
	if err != nil {
		return "", fmt.Errorf("failed to decode encrypted key: %w", err)
	}

	aesKey, err := RSADecrypt(encryptedKey, privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt AES key: %w", err)
	}

	password, err := AESDecrypt(entry.EncryptedPassword, aesKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt password: %w", err)
	}

	return string(password), nil
}

// BoxEncryptedData holds data encrypted with NaCl box
type BoxEncryptedData struct {
	EncryptedPassword string            `json:"encrypted_password"`
	BoxKeys           map[string]string `json:"box_keys"` // base64(ephemeralPub + nonce + encryptedKey)
}

// HybridEncryptBox encrypts password using NaCl box for device keys
func HybridEncryptBox(password string, devices []models.Device,
	getBoxPublicKey func(string) (*[32]byte, error)) (*BoxEncryptedData, error) {

	// Generate random AES key
	aesKey, err := GenerateAESKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate AES key: %w", err)
	}

	// Encrypt password with AES-GCM
	encryptedPassword, err := AESEncrypt([]byte(password), aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt password: %w", err)
	}

	// Encrypt AES key for each trusted device using NaCl box
	boxKeys := make(map[string]string)
	for _, device := range devices {
		if !device.Trusted {
			continue
		}

		boxPubKey, err := getBoxPublicKey(device.Fingerprint)
		if err != nil {
			return nil, fmt.Errorf("failed to get box public key for device %s: %w",
				device.ID, err)
		}

		// Generate ephemeral keypair for this encryption
		ephemeralPub, ephemeralPriv, err := box.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
		}

		// Generate random nonce
		var nonce [24]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return nil, fmt.Errorf("failed to generate nonce: %w", err)
		}

		// Encrypt AES key
		encryptedKey := box.Seal(nil, aesKey, &nonce, boxPubKey, ephemeralPriv)

		// Combine: ephemeralPub + nonce + encryptedKey
		combined := make([]byte, 0, 32+24+len(encryptedKey))
		combined = append(combined, ephemeralPub[:]...)
		combined = append(combined, nonce[:]...)
		combined = append(combined, encryptedKey...)

		boxKeys[device.Fingerprint] = base64.StdEncoding.EncodeToString(combined)
	}

	if len(boxKeys) == 0 {
		return nil, fmt.Errorf("no trusted devices to encrypt for")
	}

	return &BoxEncryptedData{
		EncryptedPassword: encryptedPassword,
		BoxKeys:           boxKeys,
	}, nil
}

// HybridDecryptBox decrypts password using NaCl box
func HybridDecryptBox(entry *models.PasswordEntry, boxPrivKey *[32]byte) (string, error) {
	// Get fingerprint from X25519 private key
	var boxPubKey [32]byte
	// Note: We can't easily derive the public key from private key in X25519
	// without the base point multiplication. For now, we'll need to store
	// or look up the fingerprint.
	_ = boxPubKey

	// For now, try all box keys until one decrypts successfully
	for fingerprint, boxKeyBase64 := range entry.BoxKeys {
		combined, err := base64.StdEncoding.DecodeString(boxKeyBase64)
		if err != nil {
			continue
		}

		if len(combined) < 32+24 {
			continue
		}

		// Extract ephemeral public key, nonce, and encrypted data
		var ephemeralPub [32]byte
		var nonce [24]byte
		copy(ephemeralPub[:], combined[:32])
		copy(nonce[:], combined[32:56])
		encryptedAESKey := combined[56:]

		// Try to decrypt
		aesKey, ok := box.Open(nil, encryptedAESKey, &nonce, &ephemeralPub, boxPrivKey)
		if !ok {
			continue
		}

		// Decrypt password
		password, err := AESDecrypt(entry.EncryptedPassword, aesKey)
		if err != nil {
			return "", fmt.Errorf("failed to decrypt password: %w", err)
		}

		_ = fingerprint // Use fingerprint if needed
		return string(password), nil
	}

	return "", fmt.Errorf("no box key could be decrypted for this device")
}
