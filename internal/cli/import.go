package cli

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/bok1c4/pwman/internal/config"
	"github.com/bok1c4/pwman/internal/crypto"
	"github.com/bok1c4/pwman/internal/device"
	"github.com/bok1c4/pwman/internal/imported"
	"github.com/bok1c4/pwman/pkg/models"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var importDB string
var importPrivateKey string
var importPassphrase string

var importCmd = &cobra.Command{
	Use:   "import cpp",
	Short: "Import from C++ PostgreSQL database",
	Long: `Import passwords from the C++ implementation's PostgreSQL database.
	
Example:
  pwman import cpp \
    --db "postgres://user:pass@localhost:5432/passwords" \
    --private-key ./private.key \
    --passphrase "your-passphrase"`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil || cfg == nil {
			fmt.Println("[ERROR] Vault not initialized. Run 'pwman init' first")
			os.Exit(1)
		}

		if importDB == "" {
			fmt.Println("[ERROR] --db is required")
			os.Exit(1)
		}

		if importPrivateKey == "" {
			fmt.Println("[ERROR] --private-key is required")
			os.Exit(1)
		}

		privateKey, err := loadPrivateKeyWithPassphrase(importPrivateKey, importPassphrase)
		if err != nil {
			fmt.Printf("[ERROR] Failed to load private key: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("[INFO] Connecting to PostgreSQL database...")
		cppDB, err := imported.NewCPPDatabase(importDB)
		if err != nil {
			fmt.Printf("[ERROR] Failed to connect to database: %v\n", err)
			os.Exit(1)
		}
		defer cppDB.Close()

		entryCount, err := cppDB.GetEntriesCount()
		if err != nil {
			fmt.Printf("[ERROR] Failed to get entries count: %v\n", err)
			os.Exit(1)
		}

		keyCount, err := cppDB.GetPublicKeysCount()
		if err != nil {
			fmt.Printf("[ERROR] Failed to get public keys count: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("[INFO] Found %d entries and %d public keys\n", entryCount, keyCount)

		entries, err := cppDB.GetEntries()
		if err != nil {
			fmt.Printf("[ERROR] Failed to get entries: %v\n", err)
			os.Exit(1)
		}

		fmt.Print("Enter vault password: ")
		vaultPassword, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			fmt.Println("[ERROR] Failed to read password")
			os.Exit(1)
		}

		mgr, _, err := device.LoadVault(string(vaultPassword))
		if err != nil {
			fmt.Printf("[ERROR] Failed to unlock vault: %v\n", err)
			os.Exit(1)
		}

		trustedDevices, err := mgr.GetTrustedDevices()
		if err != nil {
			fmt.Printf("[ERROR] Failed to get trusted devices: %v\n", err)
			os.Exit(1)
		}

		importedCount := 0
		for _, entry := range entries {
			decryptedPassword, err := decryptCPPEntry(entry, privateKey)
			if err != nil {
				fmt.Printf("[WARN] Failed to decrypt entry %d: %v\n", entry.ID, err)
				continue
			}

			encrypted, err := crypto.HybridEncrypt(decryptedPassword, trustedDevices, func(fingerprint string) (*rsa.PublicKey, error) {
				for _, d := range trustedDevices {
					if d.Fingerprint == fingerprint {
						return crypto.LoadPublicKey(d.PublicKey)
					}
				}
				return nil, fmt.Errorf("device not found")
			})
			if err != nil {
				fmt.Printf("[WARN] Failed to encrypt entry %d: %v\n", entry.ID, err)
				continue
			}

			newEntry := &models.PasswordEntry{
				ID:                uuid.New().String(),
				Version:           1,
				Site:              fmt.Sprintf("imported-%d", entry.ID),
				Username:          "",
				EncryptedPassword: encrypted.EncryptedPassword,
				EncryptedAESKeys:  encrypted.EncryptedAESKeys,
				Notes:             entry.Note,
				CreatedAt:         time.Now(),
				UpdatedAt:         time.Now(),
				UpdatedBy:         mgr.GetDeviceID(),
			}

			if err := mgr.GetStorage().CreateEntry(newEntry); err != nil {
				fmt.Printf("[WARN] Failed to save entry %d: %v\n", entry.ID, err)
				continue
			}

			importedCount++
		}

		fmt.Printf("[INFO] Successfully imported %d/%d entries\n", importedCount, len(entries))
	},
}

func loadPrivateKeyWithPassphrase(keyPath, passphrase string) (*rsa.PrivateKey, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	var decryptedBytes []byte

	if x509.IsEncryptedPEMBlock(block) {
		if passphrase == "" {
			return nil, fmt.Errorf("passphrase required for encrypted key")
		}
		decryptedBytes, err = x509.DecryptPEMBlock(block, []byte(passphrase))
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt PEM block: %w", err)
		}
	} else {
		decryptedBytes = block.Bytes
	}

	return x509.ParsePKCS1PrivateKey(decryptedBytes)
}

func decryptCPPEntry(entry imported.CPPEntry, privateKey *rsa.PrivateKey) (string, error) {
	encryptedKeyBytes, err := base64.StdEncoding.DecodeString(entry.AESKey)
	if err != nil {
		return "", fmt.Errorf("failed to decode AES key: %w", err)
	}

	aesKey, err := crypto.RSADecrypt(encryptedKeyBytes, privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt AES key: %w", err)
	}

	passwordBytes, err := base64.StdEncoding.DecodeString(entry.Password)
	if err != nil {
		return "", fmt.Errorf("failed to decode password: %w", err)
	}

	decryptedPassword, err := crypto.AESDecrypt(string(passwordBytes), aesKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt password: %w", err)
	}

	return string(decryptedPassword), nil
}

func init() {
	importCmd.Flags().StringVar(&importDB, "db", "", "PostgreSQL connection string (required)")
	importCmd.Flags().StringVar(&importPrivateKey, "private-key", "", "Path to private key file (required)")
	importCmd.Flags().StringVar(&importPassphrase, "passphrase", "", "Passphrase for encrypted private key")
	AddCommand(importCmd)
}
