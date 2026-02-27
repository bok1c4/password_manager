package models

import "time"

type Device struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	PublicKey    string    `json:"public_key"`
	Fingerprint  string    `json:"fingerprint"`
	CreatedAt    time.Time `json:"created_at"`
	Trusted      bool      `json:"trusted"`
	ApprovalCode string    `json:"approval_code,omitempty"`
}

type PasswordEntry struct {
	ID                string            `json:"id"`
	Version           int64             `json:"version"`
	Site              string            `json:"site"`
	Username          string            `json:"username"`
	EncryptedPassword string            `json:"encrypted_password"`
	EncryptedAESKeys  map[string]string `json:"encrypted_aes_keys"`
	Notes             string            `json:"notes"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	UpdatedBy         string            `json:"updated_by"`
	DeletedAt         *time.Time        `json:"deleted_at,omitempty"`
}

type VaultMetadata struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

const (
	MetaVaultID   = "vault_id"
	MetaVersion   = "version"
	MetaCreatedAt = "created_at"
	MetaLastSync  = "last_sync"
	MetaGitRemote = "git_remote"
)
