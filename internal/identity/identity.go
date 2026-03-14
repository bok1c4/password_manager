// Package identity provides Ed25519/X25519 key generation and management
// for device identity and encryption.
package identity

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/curve25519"
)

// DeviceIdentity holds Ed25519 for signing/identity and X25519 for encryption
type DeviceIdentity struct {
	// Ed25519 - for device identity, TLS certificates, signing
	SignPublicKey  ed25519.PublicKey
	SignPrivateKey ed25519.PrivateKey

	// X25519 - for NaCl box encryption of vault AES keys
	BoxPublicKey  [32]byte
	BoxPrivateKey [32]byte

	// Fingerprint: hex(sha256(SignPublicKey)[:8]) - 16 hex chars, readable
	Fingerprint string
}

// GenerateIdentity creates a new device identity with Ed25519 and X25519 keys
func GenerateIdentity() (*DeviceIdentity, error) {
	// Generate Ed25519 keypair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Ed25519 key: %w", err)
	}

	// Derive X25519 key from Ed25519 private key per RFC 7748 §5
	// 1. Hash the Ed25519 seed with SHA-512
	h := sha512.Sum512(priv.Seed())

	// 2. Apply clamping per RFC 7748:
	//    - Clear bit 0, 1, 2 of first byte
	//    - Set bit 6 of last byte
	//    - Clear bit 7 of last byte
	h[0] &= 248  // Clear bits 0, 1, 2
	h[31] &= 127 // Clear bit 7
	h[31] |= 64  // Set bit 6

	// 3. Copy to X25519 private key
	var boxPriv, boxPub [32]byte
	copy(boxPriv[:], h[:32])

	// 4. Derive public key
	curve25519.ScalarBaseMult(&boxPub, &boxPriv)

	// Generate fingerprint: first 8 bytes of SHA-256 hash = 16 hex chars
	fpHash := sha256.Sum256(pub)
	fp := hex.EncodeToString(fpHash[:8])

	return &DeviceIdentity{
		SignPublicKey:  pub,
		SignPrivateKey: priv,
		BoxPublicKey:   boxPub,
		BoxPrivateKey:  boxPriv,
		Fingerprint:    fp,
	}, nil
}

// Save writes the identity to files (private key encrypted, public key plaintext)
func (id *DeviceIdentity) Save(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Save Ed25519 private key
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "ED25519 PRIVATE KEY",
		Bytes: id.SignPrivateKey.Seed(),
	})

	if err := os.WriteFile(path, privPEM, 0600); err != nil {
		return fmt.Errorf("failed to save private key: %w", err)
	}

	// Save Ed25519 public key
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "ED25519 PUBLIC KEY",
		Bytes: id.SignPublicKey,
	})

	pubPath := path + ".pub"
	if err := os.WriteFile(pubPath, pubPEM, 0644); err != nil {
		return fmt.Errorf("failed to save public key: %w", err)
	}

	return nil
}

// LoadIdentity loads an identity from files
func LoadIdentity(path string) (*DeviceIdentity, error) {
	// Load private key
	privData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	block, _ := pem.Decode(privData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode private key PEM")
	}

	priv := ed25519.NewKeyFromSeed(block.Bytes)
	pub := priv.Public().(ed25519.PublicKey)

	// Derive X25519 keys per RFC 7748
	h := sha512.Sum512(priv.Seed())
	h[0] &= 248
	h[31] &= 127
	h[31] |= 64

	var boxPriv, boxPub [32]byte
	copy(boxPriv[:], h[:32])
	curve25519.ScalarBaseMult(&boxPub, &boxPriv)

	// Generate fingerprint
	fpHash := sha256.Sum256(pub)
	fp := hex.EncodeToString(fpHash[:8])

	return &DeviceIdentity{
		SignPublicKey:  pub,
		SignPrivateKey: priv,
		BoxPublicKey:   boxPub,
		BoxPrivateKey:  boxPriv,
		Fingerprint:    fp,
	}, nil
}

// GetBoxFingerprint returns the fingerprint for the X25519 public key
func (id *DeviceIdentity) GetBoxFingerprint() string {
	h := sha256.Sum256(id.BoxPublicKey[:])
	return hex.EncodeToString(h[:8])
}

// Sign signs a message with the Ed25519 private key
func (id *DeviceIdentity) Sign(message []byte) []byte {
	return ed25519.Sign(id.SignPrivateKey, message)
}

// Verify verifies a signature with the Ed25519 public key
func (id *DeviceIdentity) Verify(message, sig []byte) bool {
	return ed25519.Verify(id.SignPublicKey, message, sig)
}

// GetBoxPublicKeyBytes returns the X25519 public key as bytes
func (id *DeviceIdentity) GetBoxPublicKeyBytes() []byte {
	return id.BoxPublicKey[:]
}

// GetBoxPrivateKey returns a pointer to the X25519 private key
func (id *DeviceIdentity) GetBoxPrivateKey() *[32]byte {
	return &id.BoxPrivateKey
}

// SaveEncrypted saves the identity with password-based encryption
func (id *DeviceIdentity) SaveEncrypted(path string, masterPassword string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate random salt
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive key using Argon2id
	key := argon2.IDKey([]byte(masterPassword), salt, 3, 64*1024, 4, 32)

	// Encrypt the private key seed
	ciphertext, nonce, err := encryptAESGCM(id.SignPrivateKey.Seed(), key)
	if err != nil {
		return fmt.Errorf("failed to encrypt private key: %w", err)
	}

	// Combine salt + nonce + ciphertext
	encryptedData := make([]byte, 0, len(salt)+len(nonce)+len(ciphertext))
	encryptedData = append(encryptedData, salt...)
	encryptedData = append(encryptedData, nonce...)
	encryptedData = append(encryptedData, ciphertext...)

	// Save encrypted private key as PEM
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "ENCRYPTED PRIVATE KEY",
		Bytes: encryptedData,
	})

	if err := os.WriteFile(path, privPEM, 0600); err != nil {
		return fmt.Errorf("failed to save encrypted private key: %w", err)
	}

	// Save Ed25519 public key as plaintext
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "ED25519 PUBLIC KEY",
		Bytes: id.SignPublicKey,
	})

	pubPath := path + ".pub"
	if err := os.WriteFile(pubPath, pubPEM, 0644); err != nil {
		return fmt.Errorf("failed to save public key: %w", err)
	}

	return nil
}

// LoadIdentityEncrypted loads an identity from encrypted files
func LoadIdentityEncrypted(path string, masterPassword string) (*DeviceIdentity, error) {
	// Load encrypted private key
	privData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read encrypted private key: %w", err)
	}

	block, _ := pem.Decode(privData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode private key PEM")
	}

	if block.Type != "ENCRYPTED PRIVATE KEY" {
		return nil, fmt.Errorf("expected ENCRYPTED PRIVATE KEY PEM block, got %s", block.Type)
	}

	// Extract salt, nonce, and ciphertext
	if len(block.Bytes) < 28 {
		return nil, fmt.Errorf("encrypted data too short")
	}

	salt := block.Bytes[:16]
	nonce := block.Bytes[16:28]
	ciphertext := block.Bytes[28:]

	// Derive key using Argon2id
	key := argon2.IDKey([]byte(masterPassword), salt, 3, 64*1024, 4, 32)

	// Decrypt the private key seed
	seed, err := decryptAESGCM(ciphertext, nonce, key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt private key: %w", err)
	}

	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)

	// Derive X25519 keys per RFC 7748
	h := sha512.Sum512(priv.Seed())
	h[0] &= 248
	h[31] &= 127
	h[31] |= 64

	var boxPriv, boxPub [32]byte
	copy(boxPriv[:], h[:32])
	curve25519.ScalarBaseMult(&boxPub, &boxPriv)

	// Generate fingerprint
	fpHash := sha256.Sum256(pub)
	fp := hex.EncodeToString(fpHash[:8])

	return &DeviceIdentity{
		SignPublicKey:  pub,
		SignPrivateKey: priv,
		BoxPublicKey:   boxPub,
		BoxPrivateKey:  boxPriv,
		Fingerprint:    fp,
	}, nil
}

// encryptAESGCM encrypts plaintext using AES-256-GCM
func encryptAESGCM(plaintext, key []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// decryptAESGCM decrypts ciphertext using AES-256-GCM
func decryptAESGCM(ciphertext, nonce, key []byte) (plaintext []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid nonce size")
	}

	plaintext, err = gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}
