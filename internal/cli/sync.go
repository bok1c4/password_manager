package cli

import (
	"fmt"
	"os"

	"github.com/bok1c4/pwman/internal/config"
	"github.com/bok1c4/pwman/internal/sync"
	"github.com/spf13/cobra"
)

var syncInitRemote string
var syncMessage string

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Git sync commands",
	Long:  `Synchronize your vault using Git.`,
}

var syncInitCmd = &cobra.Command{
	Use:   "init <remote-url>",
	Short: "Initialize git sync with a remote repository",
	Long:  `Initialize git sync by setting up a remote repository URL.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil || cfg == nil {
			fmt.Println("[ERROR] Vault not initialized. Run 'pwman init' first")
			os.Exit(1)
		}

		remoteURL := args[0]
		gs, err := sync.NewGitSync(cfg)
		if err != nil {
			fmt.Printf("[ERROR] Failed to create git sync: %v\n", err)
			os.Exit(1)
		}

		if err := gs.SetRemote(remoteURL); err != nil {
			fmt.Printf("[ERROR] Failed to set remote: %v\n", err)
			os.Exit(1)
		}

		active, _ := config.GetActiveVault()
		_, err = sync.InitRepo(config.VaultPath(active))
		if err != nil {
			fmt.Printf("[ERROR] Failed to init repo: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("[INFO] Git sync initialized with remote: %s\n", remoteURL)
	},
}

var syncPushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push changes to remote",
	Long:  `Commit and push local changes to the remote repository.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil || cfg == nil {
			fmt.Println("[ERROR] Vault not initialized. Run 'pwman init' first")
			os.Exit(1)
		}

		gs, err := sync.NewGitSync(cfg)
		if err != nil {
			fmt.Printf("[ERROR] Failed to create git sync: %v\n", err)
			os.Exit(1)
		}

		if !gs.HasRemote() {
			fmt.Println("[ERROR] No remote configured. Run 'pwman sync init <remote>' first")
			os.Exit(1)
		}

		message := syncMessage
		if message == "" {
			message = "Update vault"
		}

		err = gs.CommitAndPush(message)
		if err != nil {
			fmt.Printf("[ERROR] Failed to push: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("[INFO] Changes pushed to remote")
	},
}

var syncPullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull changes from remote",
	Long:  `Pull and merge changes from the remote repository.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil || cfg == nil {
			fmt.Println("[ERROR] Vault not initialized. Run 'pwman init' first")
			os.Exit(1)
		}

		gs, err := sync.NewGitSync(cfg)
		if err != nil {
			fmt.Printf("[ERROR] Failed to create git sync: %v\n", err)
			os.Exit(1)
		}

		if !gs.HasRemote() {
			fmt.Println("[ERROR] No remote configured. Run 'pwman sync init <remote>' first")
			os.Exit(1)
		}

		err = gs.Pull()
		if err != nil {
			fmt.Printf("[ERROR] Failed to pull: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("[INFO] Changes pulled from remote")
	},
}

var syncStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync status",
	Long:  `Show current sync status and remote information.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil || cfg == nil {
			fmt.Println("[ERROR] Vault not initialized. Run 'pwman init' first")
			os.Exit(1)
		}

		gs, err := sync.NewGitSync(cfg)
		if err != nil {
			fmt.Printf("[ERROR] Failed to create git sync: %v\n", err)
			os.Exit(1)
		}

		if gs.HasRemote() {
			fmt.Printf("[INFO] Remote: %s\n", gs.GetRemote())
		} else {
			fmt.Println("[INFO] No remote configured")
		}
	},
}

var syncCmdFull = &cobra.Command{
	Use:   "sync",
	Short: "Full sync (pull + push)",
	Long:  `Pull changes from remote, then push local changes.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil || cfg == nil {
			fmt.Println("[ERROR] Vault not initialized. Run 'pwman init' first")
			os.Exit(1)
		}

		gs, err := sync.NewGitSync(cfg)
		if err != nil {
			fmt.Printf("[ERROR] Failed to create git sync: %v\n", err)
			os.Exit(1)
		}

		if !gs.HasRemote() {
			fmt.Println("[ERROR] No remote configured. Run 'pwman sync init <remote>' first")
			os.Exit(1)
		}

		fmt.Println("[INFO] Pulling changes...")
		if err := gs.Pull(); err != nil {
			fmt.Printf("[WARN] Pull failed: %v\n", err)
		}

		message := syncMessage
		if message == "" {
			message = "Update vault"
		}

		fmt.Println("[INFO] Pushing changes...")
		if err := gs.CommitAndPush(message); err != nil {
			fmt.Printf("[ERROR] Push failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("[INFO] Sync complete")
	},
}

func init() {
	syncPushCmd.Flags().StringVarP(&syncMessage, "message", "m", "", "Commit message")
	syncCmdFull.Flags().StringVarP(&syncMessage, "message", "m", "", "Commit message")

	syncCmd.AddCommand(syncInitCmd)
	syncCmd.AddCommand(syncPushCmd)
	syncCmd.AddCommand(syncPullCmd)
	syncCmd.AddCommand(syncStatusCmd)
	syncCmd.AddCommand(syncCmdFull)
	AddCommand(syncCmd)
}
