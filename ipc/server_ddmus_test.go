package ipc

// ddmus: the messages a user sees name ddmus, not cliamp.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestAlreadyRunningNamesDdmus(t *testing.T) {
	dir := shortTempDir(t)
	sock := filepath.Join(dir, "ddmus.sock")
	if err := os.WriteFile(sock+".pid", []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewServer(sock)
	if err == nil || !strings.Contains(err.Error(), "ddmus is already running") || strings.Contains(err.Error(), "cliamp") {
		t.Fatalf("NewServer() error = %v, want it to say ddmus is already running", err)
	}
}
