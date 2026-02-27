package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/bok1c4/pwman/pkg/models"
	"github.com/google/uuid"
)

func TestNewSQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("Failed to create SQLite: %v", err)
	}
	defer db.Close()

	if db.db == nil {
		t.Fatal("Database is nil")
	}
}

func TestDeviceCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("Failed to create SQLite: %v", err)
	}
	defer db.Close()

	device := &models.Device{
		ID:          uuid.New().String(),
		Name:        "Test Device",
		PublicKey:   "test-public-key",
		Fingerprint: "test-fingerprint",
		CreatedAt:   time.Now(),
		Trusted:     true,
	}

	if err := db.UpsertDevice(device); err != nil {
		t.Fatalf("Failed to upsert device: %v", err)
	}

	retrieved, err := db.GetDevice(device.ID)
	if err != nil {
		t.Fatalf("Failed to get device: %v", err)
	}

	if retrieved.ID != device.ID {
		t.Errorf("Device ID mismatch: got %s, want %s", retrieved.ID, device.ID)
	}

	if retrieved.Name != device.Name {
		t.Errorf("Device name mismatch: got %s, want %s", retrieved.Name, device.Name)
	}

	devices, err := db.ListDevices()
	if err != nil {
		t.Fatalf("Failed to list devices: %v", err)
	}

	if len(devices) != 1 {
		t.Errorf("Expected 1 device, got %d", len(devices))
	}

	if err := db.DeleteDevice(device.ID); err != nil {
		t.Fatalf("Failed to delete device: %v", err)
	}

	_, err = db.GetDevice(device.ID)
	if err == nil {
		t.Error("Device should have been deleted")
	}
}

func TestEntryCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("Failed to create SQLite: %v", err)
	}
	defer db.Close()

	entry := &models.PasswordEntry{
		ID:                uuid.New().String(),
		Version:           1,
		Site:              "github.com",
		Username:          "testuser",
		EncryptedPassword: "encrypted-password",
		EncryptedAESKeys: map[string]string{
			"device-1": "encrypted-key",
		},
		Notes:     "Test note",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		UpdatedBy: "device-1",
	}

	if err := db.CreateEntry(entry); err != nil {
		t.Fatalf("Failed to create entry: %v", err)
	}

	retrieved, err := db.GetEntry(entry.ID)
	if err != nil {
		t.Fatalf("Failed to get entry: %v", err)
	}

	if retrieved.Site != entry.Site {
		t.Errorf("Site mismatch: got %s, want %s", retrieved.Site, entry.Site)
	}

	if retrieved.Username != entry.Username {
		t.Errorf("Username mismatch: got %s, want %s", retrieved.Username, entry.Username)
	}

	bySite, err := db.GetEntryBySite("github.com")
	if err != nil {
		t.Fatalf("Failed to get entry by site: %v", err)
	}

	if bySite.ID != entry.ID {
		t.Error("Entry by site mismatch")
	}

	entries, err := db.ListEntries()
	if err != nil {
		t.Fatalf("Failed to list entries: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(entries))
	}

	retrieved.EncryptedPassword = "new-encrypted-password"
	if err := db.UpdateEntry(retrieved); err != nil {
		t.Fatalf("Failed to update entry: %v", err)
	}

	updated, _ := db.GetEntry(entry.ID)
	if updated.EncryptedPassword != "new-encrypted-password" {
		t.Error("Entry was not updated")
	}

	if err := db.DeleteEntry(entry.ID); err != nil {
		t.Fatalf("Failed to delete entry: %v", err)
	}

	entries, _ = db.ListEntries()
	if len(entries) != 0 {
		t.Errorf("Expected 0 entries after delete, got %d", len(entries))
	}
}

func TestMetaCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("Failed to create SQLite: %v", err)
	}
	defer db.Close()

	if err := db.UpsertMeta("test-key", "test-value"); err != nil {
		t.Fatalf("Failed to upsert meta: %v", err)
	}

	value, err := db.GetMeta("test-key")
	if err != nil {
		t.Fatalf("Failed to get meta: %v", err)
	}

	if value != "test-value" {
		t.Errorf("Meta value mismatch: got %s, want %s", value, "test-value")
	}

	if err := db.UpsertMeta("test-key", "new-value"); err != nil {
		t.Fatalf("Failed to update meta: %v", err)
	}

	value, _ = db.GetMeta("test-key")
	if value != "new-value" {
		t.Error("Meta was not updated")
	}
}

func TestSoftDelete(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("Failed to create SQLite: %v", err)
	}
	defer db.Close()

	entry := &models.PasswordEntry{
		ID:                uuid.New().String(),
		Version:           1,
		Site:              "delete-test.com",
		Username:          "testuser",
		EncryptedPassword: "encrypted-password",
		EncryptedAESKeys:  map[string]string{},
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	db.CreateEntry(entry)

	entries, _ := db.ListEntries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry before delete, got %d", len(entries))
	}

	db.DeleteEntry(entry.ID)

	entries, _ = db.ListEntries()
	if len(entries) != 0 {
		t.Errorf("Expected 0 entries after soft delete, got %d", len(entries))
	}
}
