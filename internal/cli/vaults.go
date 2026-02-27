package cli

import (
	"fmt"
	"os"

	"github.com/bok1c4/pwman/internal/config"
	"github.com/spf13/cobra"
)

var vaultName string

var vaultsCmd = &cobra.Command{
	Use:   "vaults",
	Short: "Manage multiple vaults",
	Long:  `Manage multiple password vaults on this device.`,
}

var vaultsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all vaults",
	Long:  `List all vaults and show which is active.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadGlobalConfig()
		if err != nil {
			fmt.Printf("[ERROR] Failed to load config: %v\n", err)
			os.Exit(1)
		}

		if len(cfg.Vaults) == 0 {
			fmt.Println("[INFO] No vaults found. Create one with 'pwman init --vault <name>'")
			return
		}

		fmt.Println("Vaults:")
		for _, v := range cfg.Vaults {
			marker := "  "
			if v == cfg.ActiveVault {
				marker = "* "
			}
			fmt.Printf("%s%s\n", marker, v)
		}
		fmt.Printf("\nActive vault: %s\n", cfg.ActiveVault)
	},
}

var vaultsCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new vault",
	Long:  `Create a new vault with the given name.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		cfg, err := config.LoadGlobalConfig()
		if err != nil {
			fmt.Printf("[ERROR] Failed to load config: %v\n", err)
			os.Exit(1)
		}

		for _, v := range cfg.Vaults {
			if v == name {
				fmt.Printf("[ERROR] Vault '%s' already exists\n", name)
				os.Exit(1)
			}
		}

		if err := config.EnsureVaultDirForVault(name); err != nil {
			fmt.Printf("[ERROR] Failed to create vault directory: %v\n", err)
			os.Exit(1)
		}

		cfg.AddVault(name)
		cfg.ActiveVault = name
		if err := cfg.Save(); err != nil {
			fmt.Printf("[ERROR] Failed to save config: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("[INFO] Created vault '%s'\n", name)
		fmt.Printf("[INFO] Use 'pwman init --vault %s --name <device>' to initialize it\n", name)
	},
}

var vaultsDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a vault",
	Long:  `Delete a vault and all its data. This cannot be undone!`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		cfg, err := config.LoadGlobalConfig()
		if err != nil {
			fmt.Printf("[ERROR] Failed to load config: %v\n", err)
			os.Exit(1)
		}

		found := false
		for _, v := range cfg.Vaults {
			if v == name {
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("[ERROR] Vault '%s' does not exist\n", name)
			os.Exit(1)
		}

		fmt.Printf("[WARN] This will DELETE all data in vault '%s'!\n", name)
		fmt.Print("Type the vault name to confirm: ")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != name {
			fmt.Println("[INFO] Cancelled")
			os.Exit(0)
		}

		cfg.RemoveVault(name)
		if err := cfg.Save(); err != nil {
			fmt.Printf("[ERROR] Failed to save config: %v\n", err)
			os.Exit(1)
		}

		os.RemoveAll(config.VaultPath(name))
		fmt.Printf("[INFO] Vault '%s' deleted\n", name)
	},
}

var useCmd = &cobra.Command{
	Use:   "use <vault>",
	Short: "Switch to a different vault",
	Long:  `Switch the active vault.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		if err := config.SetActiveVault(name); err != nil {
			fmt.Printf("[ERROR] %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("[INFO] Switched to vault '%s'\n", name)
	},
}

func init() {
	vaultsCmd.AddCommand(vaultsListCmd)
	vaultsCmd.AddCommand(vaultsCreateCmd)
	vaultsCmd.AddCommand(vaultsDeleteCmd)
	AddCommand(vaultsCmd)
	AddCommand(useCmd)
}
