package appmeta

import "testing"

func TestDefaults(t *testing.T) {
	if got := ClientName(); got != "ddsonic" {
		t.Fatalf("ClientName() = %q, want %q", got, "ddsonic")
	}
	if got := DeviceName(); got != "ddsonic" {
		t.Fatalf("DeviceName() = %q, want %q", got, "ddsonic")
	}
}

func TestSetVersion(t *testing.T) {
	original := Version()
	defer SetVersion(original)

	SetVersion("1.2.3")
	if got := Version(); got != "1.2.3" {
		t.Fatalf("Version() = %q, want %q", got, "1.2.3")
	}
}

func TestSetVersionEmpty(t *testing.T) {
	original := Version()
	defer SetVersion(original)

	SetVersion("test")
	SetVersion("") // should be a no-op
	if got := Version(); got != "test" {
		t.Fatalf("empty SetVersion should be no-op, got %q", got)
	}
}

// ddsonic: the one User-Agent every request uses names ddsonic, its version
// without the tag's "v", and the project, never cliamp.
func TestUserAgent(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })
	for _, tt := range []struct{ version, want string }{
		{"dev", "ddsonic/dev (https://github.com/DaphnisDuck/ddsonic)"},
		{"v1.0.0", "ddsonic/1.0.0 (https://github.com/DaphnisDuck/ddsonic)"},
		{"v1.0.0-rc.1", "ddsonic/1.0.0-rc.1 (https://github.com/DaphnisDuck/ddsonic)"},
	} {
		version = tt.version
		if got := UserAgent(); got != tt.want {
			t.Errorf("UserAgent() with version %q = %q, want %q", tt.version, got, tt.want)
		}
	}
}
