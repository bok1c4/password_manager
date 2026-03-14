// Package crypto provides Argon2id key derivation for modern password hashing.
package crypto

import (
	"golang.org/x/crypto/argon2"
)

// DefaultArgon2idParams provides recommended parameters for interactive use
// These parameters take ~100ms on modern hardware with 4 threads
var DefaultArgon2idParams = Argon2idParams{
	Time:    3,         // 3 iterations
	Memory:  64 * 1024, // 64 MB
	Threads: 4,         // 4 threads
	KeyLen:  32,        // 32 bytes (256 bits)
}

// StrongArgon2idParams provides stronger parameters for sensitive use
// These parameters take ~500ms on modern hardware
var StrongArgon2idParams = Argon2idParams{
	Time:    4,          // 4 iterations
	Memory:  128 * 1024, // 128 MB
	Threads: 4,          // 4 threads
	KeyLen:  32,         // 32 bytes (256 bits)
}

// Argon2idParams holds the parameters for Argon2id key derivation
type Argon2idParams struct {
	Time    uint32 // Number of iterations
	Memory  uint32 // Memory usage in KB
	Threads uint8  // Number of parallel threads
	KeyLen  uint32 // Length of derived key in bytes
}

// DeriveKeyArgon2id derives a key from password and salt using Argon2id
// Argon2id is the winner of the Password Hashing Competition (PHC)
// It provides resistance to both GPU and ASIC attacks through memory hardness
func DeriveKeyArgon2id(password string, salt []byte, params Argon2idParams) []byte {
	return argon2.IDKey(
		[]byte(password),
		salt,
		params.Time,
		params.Memory,
		params.Threads,
		params.KeyLen,
	)
}

// DeriveKeyArgon2idDefault derives a key using default parameters
func DeriveKeyArgon2idDefault(password string, salt []byte) []byte {
	return DeriveKeyArgon2id(password, salt, DefaultArgon2idParams)
}
