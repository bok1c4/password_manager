package sync

import (
	"fmt"
	"os"
	"time"

	"github.com/bok1c4/pwman/internal/config"
	gogit "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type GitSync struct {
	repo      *gogit.Repository
	vaultName string
	cfg       *config.Config
	remote    string
}

func NewGitSync(cfg *config.Config) (*GitSync, error) {
	active, _ := config.GetActiveVault()
	gs := &GitSync{
		cfg:       cfg,
		vaultName: active,
	}

	if cfg.GitRemote != "" {
		remote, err := gs.openRemote()
		if err != nil {
			return nil, err
		}
		gs.remote = remote
	}

	return gs, nil
}

func (g *GitSync) openRemote() (string, error) {
	if g.cfg.GitRemote == "" {
		return "", fmt.Errorf("no remote configured")
	}
	return g.cfg.GitRemote, nil
}

func InitRepo(vaultPath string) (*gogit.Repository, error) {
	_, err := os.Stat(vaultPath)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(vaultPath, 0700); err != nil {
			return nil, fmt.Errorf("failed to create vault directory: %w", err)
		}
	}

	repo, err := gogit.PlainInit(vaultPath, false)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize repository: %w", err)
	}

	return repo, nil
}

func (g *GitSync) SetRemote(remoteURL string) error {
	if g.repo == nil {
		repo, err := InitRepo(config.VaultPath(g.vaultName))
		if err != nil {
			return err
		}
		g.repo = repo
	}

	_, err := g.repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteURL},
	})

	if err != nil {
		return fmt.Errorf("failed to set remote: %w", err)
	}

	g.remote = remoteURL
	g.cfg.GitRemote = remoteURL
	return g.cfg.SaveForVault(g.vaultName)
}

func (g *GitSync) Pull() error {
	if g.repo == nil {
		repo, err := gogit.PlainOpen(config.VaultPath(g.vaultName))
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}
		g.repo = repo
	}

	remoteURL, err := g.openRemote()
	if err != nil {
		return err
	}

	_, err = g.repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteURL},
	})
	if err != nil {
		return fmt.Errorf("failed to create remote: %w", err)
	}

	worktree, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	err = worktree.Pull(&gogit.PullOptions{
		RemoteName: "origin",
	})
	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to pull: %w", err)
	}

	return nil
}

func (g *GitSync) Push() error {
	if g.repo == nil {
		repo, err := gogit.PlainOpen(config.VaultPath(g.vaultName))
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}
		g.repo = repo
	}

	remoteURL, err := g.openRemote()
	if err != nil {
		return err
	}

	_, err = g.repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteURL},
	})
	if err != nil {
		return fmt.Errorf("failed to create remote: %w", err)
	}

	err = g.repo.Push(&gogit.PushOptions{
		RemoteName: "origin",
	})
	if err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}

	return nil
}

func (g *GitSync) CommitAndPush(message string) error {
	if g.repo == nil {
		repo, err := gogit.PlainOpen(config.VaultPath(g.vaultName))
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}
		g.repo = repo
	}

	worktree, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	status, err := worktree.Status()
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	if status.IsClean() {
		return fmt.Errorf("nothing to commit")
	}

	dbPath := config.DatabasePath()
	_, err = worktree.Add(dbPath)
	if err != nil {
		return fmt.Errorf("failed to add files: %w", err)
	}

	_, err = worktree.Commit(message, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  g.cfg.DeviceName,
			Email: "device@local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	remoteURL, err := g.openRemote()
	if err != nil {
		return err
	}

	_, err = g.repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteURL},
	})
	if err != nil {
		return fmt.Errorf("failed to create remote: %w", err)
	}

	err = g.repo.Push(&gogit.PushOptions{
		RemoteName: "origin",
	})
	if err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}

	return nil
}

func (g *GitSync) HasRemote() bool {
	return g.cfg.GitRemote != ""
}

func (g *GitSync) GetRemote() string {
	return g.cfg.GitRemote
}

type Auth struct {
	Username string
	Password string
}

func (g *GitSync) Clone(remoteURL, branch string) error {
	branchRef := plumbing.NewBranchReferenceName(branch)
	repo, err := gogit.PlainClone(config.VaultPath(g.vaultName), false, &gogit.CloneOptions{
		URL:           remoteURL,
		ReferenceName: branchRef,
		SingleBranch:  true,
	})
	if err != nil {
		return fmt.Errorf("failed to clone: %w", err)
	}

	g.repo = repo
	g.cfg.GitRemote = remoteURL
	return g.cfg.SaveForVault(g.vaultName)
}
