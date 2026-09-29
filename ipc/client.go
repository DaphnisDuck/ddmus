package ipc

import (
	"os"
	"path/filepath"

	"github.com/bjarneo/cliamp/internal/appdir"
)

// DefaultSocketPath returns the default IPC socket path (cliamp.sock in the app config directory).
func DefaultSocketPath() string {
	dir, err := appdir.Dir()
	if err != nil {
		return filepath.Join(os.TempDir(), appdir.Name+".sock") // omatunes
	}
	return filepath.Join(dir, "cliamp.sock")
}
