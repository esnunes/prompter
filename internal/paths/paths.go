package paths

import (
	"os"
	"path/filepath"
)

// CacheDir returns the prompter cache directory, following XDG conventions:
// $XDG_CACHE_HOME/prompter or ~/.cache/prompter as fallback.
func CacheDir() (string, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "prompter"), nil
}
