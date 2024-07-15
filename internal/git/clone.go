package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/mattermost/migration-assist/internal/logger"
)

const (
	gitBinary               = "git"
	mattermostRepositoryURL = "https://github.com/mattermost/mattermost.git"
)

type CloneOptions struct {
	TempRepoPath string
	DriverType   string
	Output       string
	Version      semver.Version
}

func CloneMigrations(opts CloneOptions, baseLogger logger.LogInterface) error {
	// 1. first check if the git installedd
	_, err := exec.LookPath(gitBinary)
	if err != nil {
		return fmt.Errorf("git binary is not installed :%w", err)
	}

	// 2. clone the repository
	gitArgs := []string{"clone", "--no-checkout", "--depth=1", "--filter=tree:0", fmt.Sprintf("--branch=%s", fmt.Sprintf("v%s", opts.Version.String())), mattermostRepositoryURL, opts.TempRepoPath}

	cmd := exec.Command("git", gitArgs...)

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error during clone: %w", err)
	}

	// 3. download only migration files
	v8 := semver.MustParse("8.0.0")
	migrationsDir := filepath.Join("server", "channels", "db", "migrations", opts.DriverType)
	if opts.Version.LT(v8) {
		migrationsDir = strings.TrimPrefix(migrationsDir, filepath.Join("server", "channels"))
	}

	baseLogger.Printf("checking out...\n")
	cmd = exec.Command(gitBinary, "sparse-checkout", "set", "--no-cone", migrationsDir)
	cmd.Dir = opts.TempRepoPath

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error during sparse checkout: %w", err)
	}

	cmd = exec.Command(gitBinary, "checkout")
	cmd.Dir = opts.TempRepoPath

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error during checkout: %w", err)
	}

	if _, err2 := os.Stat(opts.Output); err2 == nil || os.IsExist(err2) {
		baseLogger.Println("removing existing migrations...")
		err2 = os.RemoveAll(opts.Output)
		if err2 != nil {
			return fmt.Errorf("error clearing output directory: %w", err)
		}
	}

	baseLogger.Printf("moving migration files into a better place..\n")
	// 4. move files to migrations directory and remove temp dir
	err = os.Rename(filepath.Join(opts.TempRepoPath, migrationsDir), opts.Output)
	if err != nil {
		return fmt.Errorf("error while renaming migrations directory: %w", err)
	}

	err = os.RemoveAll(opts.TempRepoPath)
	if err != nil {
		return fmt.Errorf("error while removing temporary directory: %w", err)
	}

	return nil
}
