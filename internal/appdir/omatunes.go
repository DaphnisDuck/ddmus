// omatunes: the fork's own on-disk identity, so omatunes and cliamp keep
// separate files and can run side by side. See docs/omatunes/files.md.

package appdir

import (
	"os"
	"path/filepath"
	"testing"
)

// Name is the directory name used for omatunes config, data and downloads.
const Name = "omatunes"

// ConfigDirEnv overrides the config directory for omatunes alone.
const ConfigDirEnv = "OMATUNES_CONFIG_DIR"

// configDirOverride returns OMATUNES_CONFIG_DIR, or "" under go test.
// Upstream's tests isolate themselves with CLIAMP_CONFIG_DIR or a temporary
// HOME and know nothing of this variable, so a developer who exports it must
// not send those tests to the real config. Dir checks CLIAMP_CONFIG_DIR first
// for the same reason.
func configDirOverride() string {
	if testing.Testing() {
		return ""
	}
	return os.Getenv(ConfigDirEnv)
}

// LibraryDBPath is the catalog database, library.db in DataDir
// (~/.local/share/omatunes).
func LibraryDBPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "library.db"), nil
}

// DownloadsDir is the default directory for saved tracks (~/Music/omatunes).
func DownloadsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Music", Name), nil
}
