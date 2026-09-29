package appdir

import (
	"os"
	"path/filepath"
	"runtime"
)

// Dir returns the omatunes configuration directory.
//
// Resolution order:
//   - CLIAMP_CONFIG_DIR (explicit override)
//   - OMATUNES_CONFIG_DIR (explicit override; omatunes)
//   - XDG_CONFIG_HOME/omatunes
//   - HOME/.config/omatunes
//   - on Windows: APPDATA/omatunes
//   - fallback: os.UserHomeDir()/.config/omatunes
func Dir() (string, error) {
	if dir, ok := os.LookupEnv("CLIAMP_CONFIG_DIR"); ok && dir != "" {
		return dir, nil
	}
	// omatunes: see configDirOverride for why this comes second.
	if dir := configDirOverride(); dir != "" {
		return dir, nil
	}
	if xdg, ok := os.LookupEnv("XDG_CONFIG_HOME"); ok && xdg != "" {
		return filepath.Join(xdg, Name), nil
	}
	if home, ok := os.LookupEnv("HOME"); ok && home != "" {
		return filepath.Join(home, ".config", Name), nil
	}
	if runtime.GOOS == "windows" {
		if appData, ok := os.LookupEnv("APPDATA"); ok && appData != "" {
			return filepath.Join(appData, Name), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", Name), nil
}

// PluginDir returns the omatunes plugin directory.
func PluginDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "plugins"), nil
}

// DataDir returns the omatunes data directory (~/.local/share/omatunes), used for
// state that is not user-edited config: plugin stores, downloaded assets, etc.
func DataDir() (string, error) {
	// Honor HOME first, matching Dir(); on Windows os.UserHomeDir() reads
	// USERPROFILE and ignores HOME, so this keeps the two resolvers consistent.
	if home, ok := os.LookupEnv("HOME"); ok && home != "" {
		return filepath.Join(home, ".local", "share", Name), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", Name), nil
}
