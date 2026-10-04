//go:build linux

package player

// ddmus: the messages a user sees name ddmus, not cliamp.

import (
	"strings"
	"testing"
)

func TestAudioOutputHintNamesDdmus(t *testing.T) {
	hint := audioOutputHint()
	if !strings.Contains(hint, "ddmus outputs through ALSA") || strings.Contains(hint, "cliamp") {
		t.Fatalf("audioOutputHint() = %q, want it to name ddmus", hint)
	}
}
