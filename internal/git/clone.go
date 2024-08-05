package git

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/mattermost/migration-assist/internal/logger"
)

const (
	gitBinary               = "git"
	minGitVersion           = "2.28.0"
	mattermostRepositoryURL = "https://github.com/mattermost/mattermost.git"
)

var (
	versionRegex = regexp.MustCompile(`v*[0-9]+\.[0-9]+`)
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

	out, err := exec.Command(gitBinary, "version").Output()
	if err != nil {
		return fmt.Errorf("error while checking git version: %w", err)
	}
	baseLogger.Printf("git version: %s\n", strings.TrimSpace(string(out)))
	gitVersion, err := semver.ParseTolerant(versionRegex.FindString(string(out)))
	if err != nil {
		return fmt.Errorf("error while parsing git version: %w", err)
	}
	if semver.MustParse(minGitVersion).GT(gitVersion) {
		return fmt.Errorf("git version should be at least %s, found %s", minGitVersion, gitVersion.String())
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
	err = CopyFS(opts.Output, os.DirFS(filepath.Join(opts.TempRepoPath, migrationsDir)))
	if err != nil {
		return fmt.Errorf("error while copying migrations directory: %w", err)
	}

	err = os.RemoveAll(opts.TempRepoPath)
	if err != nil {
		return fmt.Errorf("error while removing temporary directory: %w", err)
	}

	return nil
}

// TODO: this is a temporary solution, we'll replace this with fs.CopyFS when we move to go 1.23
// This is like a copy of fs.CopyFS from go 1.23 anyway
func CopyFS(dir string, fsys fs.FS) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		newPath := filepath.Join(dir, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(newPath, 0777)
		}

		if !d.Type().IsRegular() {
			return &os.PathError{Op: "CopyFS", Path: path, Err: os.ErrInvalid}
		}

		r, err := fsys.Open(path)
		if err != nil {
			return err
		}
		defer r.Close()

		info, err := r.Stat()
		if err != nil {
			return err
		}
		w, err := os.OpenFile(newPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666|info.Mode()&0777)
		if err != nil {
			return err
		}

		if _, err := io.Copy(w, r); err != nil {
			w.Close()
			return &os.PathError{Op: "Copy", Path: newPath, Err: err}
		}

		return w.Close()
	})
}
