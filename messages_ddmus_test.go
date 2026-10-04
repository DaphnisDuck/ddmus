package main

// ddmus: the messages a user sees name ddmus, not cliamp.

import (
	"strings"
	"testing"

	"github.com/bjarneo/cliamp/ipc"
)

func TestNotRunningNamesDdmus(t *testing.T) {
	msg := userIPCError(ipc.ErrNotRunning).Error()
	if !strings.HasPrefix(msg, "ddmus is not running") || strings.Contains(msg, "cliamp") {
		t.Fatalf("userIPCError = %q, want it to name ddmus", msg)
	}
}
