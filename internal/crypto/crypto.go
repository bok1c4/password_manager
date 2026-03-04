package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/scrypt"
)

type KeyPair struct {
	PrivateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey
}

func GenerateRSAKeyPair(bits int) (*KeyPair, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	return &KeyPair{
		PrivateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
	}, nil
}

func SavePrivateKey(key *rsa.PrivateKey, path string) error {
	data := x509.MarshalPKCS1PrivateKey(key)
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: data}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
		return fmt.Errorf("failed to save private key: %w", err)
	}
	return nil
}

func SavePublicKey(key *rsa.PublicKey, path string) error {
	data := x509.MarshalPKCS1PublicKey(key)
	block := &pem.Block{Type: "RSA PUBLIC KEY", Bytes: data}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0644); err != nil {
		return fmt.Errorf("failed to save public key: %w", err)
	}
	return nil
}

func LoadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func LoadPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key: %w", err)
	}
	return ParsePublicKey(string(data))
}

func ParsePublicKey(pemContent string) (*rsa.PublicKey, error) {
	if pemContent == "" {
		return nil, fmt.Errorf("empty PEM content")
	}
	block, _ := pem.Decode([]byte(pemContent))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block: content starts with %s", pemContent[:min(50, len(pemContent))])
	}
	return x509.ParsePKCS1PublicKey(block.Bytes)
}

func GetFingerprint(key *rsa.PublicKey) string {
	return base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(key))
}

func RSAEncrypt(plaintext []byte, publicKey *rsa.PublicKey) ([]byte, error) {
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, publicKey, plaintext)
	if err != nil {
		return nil, fmt.Errorf("encryption failed: %w", err)
	}
	return ciphertext, nil
}

func RSADecrypt(ciphertext []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	plaintext, err := rsa.DecryptPKCS1v15(rand.Reader, privateKey, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}
	return plaintext, nil
}

const AESKeySize = 32

func GenerateAESKey() ([]byte, error) {
	key := make([]byte, AESKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate AES key: %w", err)
	}
	return key, nil
}

func AESEncrypt(plaintext, key []byte) (string, error) {
	if len(key) != AESKeySize {
		return "", fmt.Errorf("invalid key size: expected %d, got %d", AESKeySize, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func AESDecrypt(encoded string, key []byte) ([]byte, error) {
	if len(key) != AESKeySize {
		return nil, fmt.Errorf("invalid key size: expected %d, got %d", AESKeySize, len(key))
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

func AESEncryptRaw(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func AESDecryptRaw(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

const (
	SaltSize     = 32
	KeySize      = 32
	N            = 16384
	R            = 8
	P            = 1
	SCRYPTKeyLen = 32
)

func GenerateSalt() ([]byte, error) {
	salt := make([]byte, SaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	return salt, nil
}

func DeriveKey(password string, salt []byte) ([]byte, error) {
	key, err := scrypt.Key([]byte(password), salt, N, R, P, SCRYPTKeyLen)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}
	return key, nil
}

func EncryptPrivateKey(key *rsa.PrivateKey, password string) (encrypted []byte, salt []byte, err error) {
	salt, err = GenerateSalt()
	if err != nil {
		return nil, nil, err
	}

	derivedKey, err := DeriveKey(password, salt)
	if err != nil {
		return nil, nil, err
	}

	privateKeyData := x509.MarshalPKCS1PrivateKey(key)
	encrypted, err = AESEncryptRaw(privateKeyData, derivedKey)
	if err != nil {
		return nil, nil, err
	}

	return encrypted, salt, nil
}

func DecryptPrivateKey(encrypted []byte, password string, salt []byte) (*rsa.PrivateKey, error) {
	derivedKey, err := DeriveKey(password, salt)
	if err != nil {
		return nil, err
	}

	plaintext, err := AESDecryptRaw(encrypted, derivedKey)
	if err != nil {
		return nil, fmt.Errorf("wrong password or corrupted key")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return privateKey, nil
}

func EncryptPrivateKeyAndSave(key *rsa.PrivateKey, password, path string) (salt []byte, err error) {
	encrypted, salt, err := EncryptPrivateKey(key, password)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(path, encrypted, 0600); err != nil {
		return nil, fmt.Errorf("failed to write encrypted private key: %w", err)
	}

	saltPath := path + ".salt"
	if err := os.WriteFile(saltPath, salt, 0600); err != nil {
		return nil, fmt.Errorf("failed to write salt: %w", err)
	}

	return salt, nil
}

func LoadAndDecryptPrivateKey(password, path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read encrypted private key: %w", err)
	}

	saltPath := path + ".salt"
	saltData, err := os.ReadFile(saltPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read salt: %w", err)
	}

	return DecryptPrivateKey(data, password, saltData)
}

func HashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return base64.StdEncoding.EncodeToString(hash[:])
}

func VerifyPasswordHash(password, hash string) bool {
	return HashPassword(password) == hash
}
