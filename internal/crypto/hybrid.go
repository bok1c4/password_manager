package crypto

import (
	"crypto/rsa"
	"encoding/base64"
	"fmt"

	"github.com/bok1c4/pwman/pkg/models"
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
