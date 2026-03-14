package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeriveKeyArgon2id(t *testing.T) {
	password := "test-password"
	salt := []byte("test-salt-16byte")

	// Derive key with default params
	key1 := DeriveKeyArgon2idDefault(password, salt)

	// Should be 32 bytes (256 bits)
	assert.Len(t, key1, 32)

	// Same password + salt should produce same key
	key2 := DeriveKeyArgon2idDefault(password, salt)
	assert.Equal(t, key1, key2)

	// Different password should produce different key
	key3 := DeriveKeyArgon2idDefault("different-password", salt)
	assert.NotEqual(t, key1, key3)

	// Different salt should produce different key
	key4 := DeriveKeyArgon2idDefault(password, []byte("different-salt-16"))
	assert.NotEqual(t, key1, key4)
}

func TestDeriveKeyArgon2id_CustomParams(t *testing.T) {
	password := "test-password"
	salt := []byte("test-salt-16byte")

	params := Argon2idParams{
		Time:    1,
		Memory:  32 * 1024,
		Threads: 2,
		KeyLen:  16,
	}

	key := DeriveKeyArgon2id(password, salt, params)

	// Should be 16 bytes as specified
	assert.Len(t, key, 16)
}

func TestDefaultArgon2idParams(t *testing.T) {
	params := DefaultArgon2idParams

	assert.Equal(t, uint32(3), params.Time)
	assert.Equal(t, uint32(64*1024), params.Memory)
	assert.Equal(t, uint8(4), params.Threads)
	assert.Equal(t, uint32(32), params.KeyLen)
}

func TestStrongArgon2idParams(t *testing.T) {
	params := StrongArgon2idParams

	assert.Equal(t, uint32(4), params.Time)
	assert.Equal(t, uint32(128*1024), params.Memory)
	assert.Equal(t, uint8(4), params.Threads)
	assert.Equal(t, uint32(32), params.KeyLen)
}
