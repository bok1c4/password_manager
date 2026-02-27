package imported

import (
	"database/sql"
	"encoding/base64"
	"fmt"

	_ "github.com/lib/pq"
)

type CPPDatabase struct {
	db *sql.DB
}

type CPPEntry struct {
	ID        int
	Password  string
	AESKey    string
	Note      string
	CreatedAt string
}

type CPPPublicKey struct {
	ID          int
	PublicKey   string
	Fingerprint string
	Username    string
}

func NewCPPDatabase(connStr string) (*CPPDatabase, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &CPPDatabase{db: db}, nil
}

func (c *CPPDatabase) Close() error {
	return c.db.Close()
}

func (c *CPPDatabase) GetEntries() ([]CPPEntry, error) {
	rows, err := c.db.Query(`
		SELECT id, password, aes_key, note, created_at 
		FROM passwords 
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query entries: %w", err)
	}
	defer rows.Close()

	var entries []CPPEntry
	for rows.Next() {
		var e CPPEntry
		if err := rows.Scan(&e.ID, &e.Password, &e.AESKey, &e.Note, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func (c *CPPDatabase) GetPublicKeys() ([]CPPPublicKey, error) {
	rows, err := c.db.Query(`
		SELECT id, public_key, fingerprint, username 
		FROM user_public_keys
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query public keys: %w", err)
	}
	defer rows.Close()

	var keys []CPPPublicKey
	for rows.Next() {
		var k CPPPublicKey
		if err := rows.Scan(&k.ID, &k.PublicKey, &k.Fingerprint, &k.Username); err != nil {
			return nil, fmt.Errorf("failed to scan public key: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (c *CPPDatabase) GetEntriesCount() (int, error) {
	var count int
	err := c.db.QueryRow("SELECT COUNT(*) FROM passwords").Scan(&count)
	return count, err
}

func (c *CPPDatabase) GetPublicKeysCount() (int, error) {
	var count int
	err := c.db.QueryRow("SELECT COUNT(*) FROM user_public_keys").Scan(&count)
	return count, err
}

func DecodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

func EncodeBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
