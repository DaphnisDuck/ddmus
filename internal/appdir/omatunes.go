// ddmus: the fork's own on-disk identity, so ddmus and cliamp keep separate
// files and can run side by side. See docs/ddmus/files.md.

package appdir

import (
	"os"
	"path/filepath"
	"testing"
)

// Name is the short name of DaphnisDuck's Music Player: its directories,
// binary, and the name it gives servers, MPRIS and the IPC socket.
const Name = "ddmus"

// ConfigDirEnv overrides the config directory for ddmus alone.
const ConfigDirEnv = "DDMUS_CONFIG_DIR"

// configDirOverride returns DDMUS_CONFIG_DIR, or "" under go test.
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
// (~/.local/share/ddmus).
func LibraryDBPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "library.db"), nil
}

// DownloadsDir is the default directory for saved tracks (~/Music/ddmus).
func DownloadsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Music", Name), nil
}
