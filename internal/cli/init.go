package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"syscall"

	"github.com/bok1c4/pwman/internal/config"
	"github.com/bok1c4/pwman/internal/device"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var initName string
var initVault string

func getPassword(prompt string) string {
	fmt.Print(prompt)
	password, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		fmt.Println("Error reading password:", err)
		os.Exit(1)
	}
	return string(password)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new vault",
	Long:  `Initialize a new password vault and generate device keys`,
	Run: func(cmd *cobra.Command, args []string) {
		if initName == "" {
			fmt.Println("Error: --name is required")
			cmd.Usage()
			os.Exit(1)
		}

		vaultName := initVault
		if vaultName == "" {
			active, err := config.GetActiveVault()
			if err != nil || active == "" {
				globalCfg, _ := config.LoadGlobalConfig()
				if len(globalCfg.Vaults) > 0 {
					vaultName = globalCfg.Vaults[0]
				} else {
					vaultName = "default"
				}
			} else {
				vaultName = active
			}
		}

		cfg, _ := config.LoadVaultConfig(vaultName)
		if cfg != nil && cfg.DeviceID != "" {
			fmt.Printf("[ERROR] Vault '%s' already initialized\n", vaultName)
			os.Exit(1)
		}

		globalCfg, err := config.LoadGlobalConfig()
		if err != nil {
			fmt.Printf("[ERROR] Failed to load global config: %v\n", err)
			os.Exit(1)
		}

		found := false
		for _, v := range globalCfg.Vaults {
			if v == vaultName {
				found = true
				break
			}
		}
		if !found {
			globalCfg.AddVault(vaultName)
			globalCfg.ActiveVault = vaultName
			if err := globalCfg.Save(); err != nil {
				fmt.Printf("[ERROR] Failed to save config: %v\n", err)
				os.Exit(1)
			}
		}

		if err := config.SetActiveVault(vaultName); err != nil {
			fmt.Printf("[ERROR] Failed to set active vault: %v\n", err)
			os.Exit(1)
		}

		password := getPassword("Enter vault password: ")
		if len(password) < 8 {
			fmt.Println("[ERROR] Password must be at least 8 characters")
			os.Exit(1)
		}

		confirmPassword := getPassword("Confirm vault password: ")
		if password != confirmPassword {
			fmt.Println("[ERROR] Passwords do not match")
			os.Exit(1)
		}

		cfg = &config.Config{
			DeviceName: initName,
		}

		if err := device.InitVault(cfg, password); err != nil {
			fmt.Printf("[ERROR] Failed to initialize vault: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("[INFO] Vault initialized successfully")
		fmt.Printf("[INFO] Device ID: %s\n", cfg.DeviceID)
		fmt.Printf("[INFO] Vault location: %s\n", config.VaultPath(vaultName))
		fmt.Println("[INFO] IMPORTANT: Remember your password! It cannot be recovered.")
	},
}

var unlockVault string
var unlockPassword string

var unlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Unlock the vault",
	Long:  `Unlock the vault with your password`,
	Run: func(cmd *cobra.Command, args []string) {
		if !device.IsVaultInitialized() {
			fmt.Println("[ERROR] Vault not initialized. Run 'pwman init' first")
			os.Exit(1)
		}

		vaultName := unlockVault
		if vaultName == "" {
			cfg, _ := config.LoadGlobalConfig()
			if cfg != nil && cfg.ActiveVault != "" {
				vaultName = cfg.ActiveVault
			}
		}

		if vaultName == "" {
			fmt.Println("[ERROR] No vault found. Specify one with --vault flag")
			os.Exit(1)
		}

		// Switch to the vault first
		useReq := map[string]string{"vault": vaultName}
		useBody, _ := json.Marshal(useReq)
		http.Post(apiBase+"/vaults/use", "application/json", bytes.NewBuffer(useBody))

		password := unlockPassword
		if password == "" {
			password = getPassword("Enter vault password: ")
		}

		unlockReq := map[string]string{"password": password}
		body, _ := json.Marshal(unlockReq)
		resp, err := http.Post(apiBase+"/unlock", "application/json", bytes.NewBuffer(body))
		if err != nil {
			fmt.Printf("[ERROR] Could not connect to server: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		if result["success"] != true {
			errMsg := "unknown error"
			if e, ok := result["error"].(string); ok {
				errMsg = e
			}
			fmt.Printf("[ERROR] %s\n", errMsg)
			os.Exit(1)
		}

		fmt.Printf("[INFO] Vault '%s' unlocked successfully\n", vaultName)
	},
}

var lockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Lock the vault",
	Long:  `Lock the vault (clears cached keys from memory)`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("[INFO] Vault locked (no cached keys to clear in CLI mode)")
	},
}

func init() {
	initCmd.Flags().StringVar(&initName, "name", "", "Device name (e.g., 'Arch Desktop', 'MacBook Pro')")
	initCmd.Flags().StringVar(&initVault, "vault", "", "Vault name (e.g., 'work', 'personal')")
	initCmd.MarkFlagRequired("name")
	AddCommand(initCmd)

	unlockCmd.Flags().StringVar(&unlockVault, "vault", "", "Vault name to unlock")
	unlockCmd.Flags().StringVarP(&unlockPassword, "password", "p", "", "Vault password (optional, will prompt if not provided)")
	AddCommand(unlockCmd)
	AddCommand(lockCmd)
}
