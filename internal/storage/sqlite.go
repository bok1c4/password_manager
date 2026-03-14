package storage

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/bok1c4/pwman/pkg/models"
)

type SQLite struct {
	db *sql.DB
}

var schema = `
CREATE TABLE IF NOT EXISTS devices (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    public_key TEXT NOT NULL,
    fingerprint TEXT UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    trusted BOOLEAN DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS entries (
    id TEXT PRIMARY KEY,
    version INTEGER NOT NULL DEFAULT 1,
    site TEXT NOT NULL,
    username TEXT,
    encrypted_password TEXT NOT NULL,
    notes TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_by TEXT REFERENCES devices(id),
    deleted_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS encrypted_keys (
    entry_id TEXT REFERENCES entries(id) ON DELETE CASCADE,
    device_fingerprint TEXT NOT NULL,
    encrypted_aes_key TEXT NOT NULL,
    PRIMARY KEY (entry_id, device_fingerprint)
);

CREATE TABLE IF NOT EXISTS vault_meta (
    key TEXT PRIMARY KEY,
    value TEXT
);

CREATE INDEX IF NOT EXISTS idx_entries_site ON entries(site);
CREATE INDEX IF NOT EXISTS idx_entries_updated ON entries(updated_at);
`

func NewSQLite(path string) (*SQLite, error) {
	if err := os.MkdirAll(path[:len(path)-len("/vault.db")], 0700); err != nil {
		return nil, fmt.Errorf("failed to create vault directory: %w", err)
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &SQLite{db: db}, nil
}

func (s *SQLite) Close() error {
	return s.db.Close()
}

// Begin starts a transaction
func (s *SQLite) Begin() (*sql.Tx, error) {
	return s.db.Begin()
}

// GetDB returns the underlying database connection
func (s *SQLite) GetDB() *sql.DB {
	return s.db
}

// UpdateEntryTx updates an entry within a transaction
func (s *SQLite) UpdateEntryTx(tx *sql.Tx, entry *models.PasswordEntry) error {
	_, err := tx.Exec(
		"UPDATE entries SET version = ?, site = ?, username = ?, encrypted_password = ?, notes = ?, updated_at = ?, updated_by = ? WHERE id = ?",
		entry.Version, entry.Site, entry.Username, entry.EncryptedPassword, entry.Notes, entry.UpdatedAt, entry.UpdatedBy, entry.ID,
	)
	if err != nil {
		return err
	}

	// Update encrypted keys
	_, err = tx.Exec("DELETE FROM encrypted_keys WHERE entry_id = ?", entry.ID)
	if err != nil {
		return err
	}

	for fp, key := range entry.EncryptedAESKeys {
		_, err = tx.Exec(
			"INSERT INTO encrypted_keys (entry_id, device_fingerprint, encrypted_aes_key) VALUES (?, ?, ?)",
			entry.ID, fp, key,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *SQLite) UpsertDevice(device *models.Device) error {
	_, err := s.db.Exec(`
		INSERT INTO devices (id, name, public_key, fingerprint, created_at, trusted)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			public_key = excluded.public_key,
			fingerprint = excluded.fingerprint,
			trusted = excluded.trusted
	`, device.ID, device.Name, device.PublicKey, device.Fingerprint, device.CreatedAt, device.Trusted)
	return err
}

func (s *SQLite) GetDevice(id string) (*models.Device, error) {
	var device models.Device
	var createdAt sql.NullTime
	err := s.db.QueryRow(`
		SELECT id, name, public_key, fingerprint, created_at, trusted
		FROM devices WHERE id = ?
	`, id).Scan(&device.ID, &device.Name, &device.PublicKey, &device.Fingerprint, &createdAt, &device.Trusted)
	if err != nil {
		return nil, err
	}
	if createdAt.Valid {
		device.CreatedAt = createdAt.Time
	}
	return &device, nil
}

func (s *SQLite) ListDevices() ([]models.Device, error) {
	rows, err := s.db.Query(`
		SELECT id, name, public_key, fingerprint, created_at, trusted
		FROM devices ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []models.Device
	for rows.Next() {
		var device models.Device
		var createdAt sql.NullTime
		if err := rows.Scan(&device.ID, &device.Name, &device.PublicKey, &device.Fingerprint, &createdAt, &device.Trusted); err != nil {
			return nil, err
		}
		if createdAt.Valid {
			device.CreatedAt = createdAt.Time
		}
		devices = append(devices, device)
	}
	return devices, nil
}

func (s *SQLite) DeleteDevice(id string) error {
	_, err := s.db.Exec(`DELETE FROM devices WHERE id = ?`, id)
	return err
}

func (s *SQLite) CreateEntry(entry *models.PasswordEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO entries (id, version, site, username, encrypted_password, notes, created_at, updated_at, updated_by, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.ID, entry.Version, entry.Site, entry.Username, entry.EncryptedPassword, entry.Notes, entry.CreatedAt, entry.UpdatedAt, entry.UpdatedBy, entry.DeletedAt)
	if err != nil {
		return err
	}

	for fingerprint, encryptedKey := range entry.EncryptedAESKeys {
		_, err = tx.Exec(`
			INSERT INTO encrypted_keys (entry_id, device_fingerprint, encrypted_aes_key)
			VALUES (?, ?, ?)
		`, entry.ID, fingerprint, encryptedKey)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLite) GetEntry(id string) (*models.PasswordEntry, error) {
	var entry models.PasswordEntry
	var deletedAt sql.NullTime
	err := s.db.QueryRow(`
		SELECT id, version, site, username, encrypted_password, notes, created_at, updated_at, updated_by, deleted_at
		FROM entries WHERE id = ?
	`, id).Scan(&entry.ID, &entry.Version, &entry.Site, &entry.Username, &entry.EncryptedPassword, &entry.Notes, &entry.CreatedAt, &entry.UpdatedAt, &entry.UpdatedBy, &deletedAt)
	if err != nil {
		return nil, err
	}
	if deletedAt.Valid {
		entry.DeletedAt = &deletedAt.Time
	}

	keys, err := s.getEncryptedKeys(id)
	if err != nil {
		return nil, err
	}
	entry.EncryptedAESKeys = keys

	return &entry, nil
}

func (s *SQLite) GetEntryBySite(site string) (*models.PasswordEntry, error) {
	var entry models.PasswordEntry
	var deletedAt sql.NullTime
	err := s.db.QueryRow(`
		SELECT id, version, site, username, encrypted_password, notes, created_at, updated_at, updated_by, deleted_at
		FROM entries WHERE site = ? AND deleted_at IS NULL
	`, site).Scan(&entry.ID, &entry.Version, &entry.Site, &entry.Username, &entry.EncryptedPassword, &entry.Notes, &entry.CreatedAt, &entry.UpdatedAt, &entry.UpdatedBy, &deletedAt)
	if err != nil {
		return nil, err
	}
	if deletedAt.Valid {
		entry.DeletedAt = &deletedAt.Time
	}

	keys, err := s.getEncryptedKeys(entry.ID)
	if err != nil {
		return nil, err
	}
	entry.EncryptedAESKeys = keys

	return &entry, nil
}

func (s *SQLite) ListEntries() ([]models.PasswordEntry, error) {
	rows, err := s.db.Query(`
		SELECT id, version, site, username, encrypted_password, notes, created_at, updated_at, updated_by, deleted_at
		FROM entries WHERE deleted_at IS NULL ORDER BY site
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.PasswordEntry
	for rows.Next() {
		var entry models.PasswordEntry
		var deletedAt sql.NullTime
		if err := rows.Scan(&entry.ID, &entry.Version, &entry.Site, &entry.Username, &entry.EncryptedPassword, &entry.Notes, &entry.CreatedAt, &entry.UpdatedAt, &entry.UpdatedBy, &deletedAt); err != nil {
			return nil, err
		}
		if deletedAt.Valid {
			entry.DeletedAt = &deletedAt.Time
		}

		keys, err := s.getEncryptedKeys(entry.ID)
		if err != nil {
			return nil, err
		}
		entry.EncryptedAESKeys = keys

		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *SQLite) getEncryptedKeys(entryID string) (map[string]string, error) {
	rows, err := s.db.Query(`
		SELECT device_fingerprint, encrypted_aes_key FROM encrypted_keys WHERE entry_id = ?
	`, entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := make(map[string]string)
	for rows.Next() {
		var fingerprint, encryptedKey string
		if err := rows.Scan(&fingerprint, &encryptedKey); err != nil {
			return nil, err
		}
		keys[fingerprint] = encryptedKey
	}
	return keys, nil
}

func (s *SQLite) UpdateEntry(entry *models.PasswordEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		UPDATE entries SET
			version = ?,
			site = ?,
			username = ?,
			encrypted_password = ?,
			notes = ?,
			updated_at = ?,
			updated_by = ?,
			deleted_at = ?
		WHERE id = ?
	`, entry.Version, entry.Site, entry.Username, entry.EncryptedPassword, entry.Notes, entry.UpdatedAt, entry.UpdatedBy, entry.DeletedAt, entry.ID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`DELETE FROM encrypted_keys WHERE entry_id = ?`, entry.ID)
	if err != nil {
		return err
	}

	for fingerprint, encryptedKey := range entry.EncryptedAESKeys {
		_, err = tx.Exec(`
			INSERT INTO encrypted_keys (entry_id, device_fingerprint, encrypted_aes_key)
			VALUES (?, ?, ?)
		`, entry.ID, fingerprint, encryptedKey)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func generateSecureRandom(length int) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	rand.Read(result)
	for i := range result {
		result[i] = chars[result[i]%byte(len(chars))]
	}
	return string(result)
}

func (s *SQLite) DeleteEntry(id string) error {
	// 1. Get the entry first
	entry, err := s.GetEntry(id)
	if err != nil {
		return err
	}

	// 2. Securely overwrite sensitive fields with random data
	entry.EncryptedPassword = generateSecureRandom(len(entry.EncryptedPassword))
	entry.Notes = generateSecureRandom(len(entry.Notes))

	// 3. Update with garbage data first
	_, err = s.db.Exec(
		"UPDATE entries SET encrypted_password = ?, notes = ? WHERE id = ?",
		entry.EncryptedPassword, entry.Notes, id,
	)
	if err != nil {
		return err
	}

	// 4. Now mark as deleted
	_, err = s.db.Exec(
		"UPDATE entries SET deleted_at = ? WHERE id = ?",
		time.Now().UTC(), id,
	)
	return err
}

func (s *SQLite) UpsertMeta(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO vault_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

func (s *SQLite) GetMeta(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM vault_meta WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}
