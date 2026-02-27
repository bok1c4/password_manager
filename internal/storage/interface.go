package storage

import "github.com/bok1c4/pwman/pkg/models"

type Storage interface {
	Close() error

	UpsertDevice(device *models.Device) error
	GetDevice(id string) (*models.Device, error)
	ListDevices() ([]models.Device, error)
	DeleteDevice(id string) error

	CreateEntry(entry *models.PasswordEntry) error
	GetEntry(id string) (*models.PasswordEntry, error)
	GetEntryBySite(site string) (*models.PasswordEntry, error)
	ListEntries() ([]models.PasswordEntry, error)
	UpdateEntry(entry *models.PasswordEntry) error
	DeleteEntry(id string) error

	UpsertMeta(key, value string) error
	GetMeta(key string) (string, error)
}
