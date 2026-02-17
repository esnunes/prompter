package repo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/esnunes/prompter/internal/paths"
)

var repoURLPattern = regexp.MustCompile(`^github\.com/[\w.\-]+/[\w.\-]+$`)

func ValidateURL(url string) error {
	if !repoURLPattern.MatchString(url) {
		return fmt.Errorf("invalid repository URL %q: expected format github.com/owner/repo", url)
	}
	return nil
}

func LocalPath(repoURL string) (string, error) {
	cacheDir, err := paths.CacheDir()
	if err != nil {
		return "", fmt.Errorf("getting cache directory: %w", err)
	}
	return filepath.Join(cacheDir, "repos", repoURL), nil
}

func EnsureCloned(ctx context.Context, repoURL string) (string, error) {
	localPath, err := LocalPath(repoURL)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(filepath.Join(localPath, ".git")); err == nil {
		return localPath, pull(ctx, localPath)
	}

	return localPath, clone(ctx, repoURL, localPath)
}

func clone(ctx context.Context, repoURL, localPath string) error {
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	cloneURL := "https://" + repoURL + ".git"
	cmd := exec.CommandContext(ctx, "git", "clone", cloneURL, localPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Cloning %s...\n", repoURL)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cloning repository: %w", err)
	}
	return nil
}

func pull(ctx context.Context, localPath string) error {
	cmd := exec.CommandContext(ctx, "git", "pull", "--ff-only")
	cmd.Dir = localPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("Updating repository...")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pulling repository (try deleting %s and restarting): %w", localPath, err)
	}
	return nil
}
