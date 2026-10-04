package appmeta

import "testing"

func TestDefaults(t *testing.T) {
	if got := ClientName(); got != "ddmus" {
		t.Fatalf("ClientName() = %q, want %q", got, "ddmus")
	}
	if got := DeviceName(); got != "ddmus" {
		t.Fatalf("DeviceName() = %q, want %q", got, "ddmus")
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

// ddmus: the one User-Agent every request uses names ddmus, its version
// without the tag's "v", and the project, never cliamp.
func TestUserAgent(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })
	for _, tt := range []struct{ version, want string }{
		{"dev", "ddmus/dev (https://github.com/DaphnisDuck/ddmus)"},
		{"v1.0.0", "ddmus/1.0.0 (https://github.com/DaphnisDuck/ddmus)"},
		{"v1.0.0-rc.1", "ddmus/1.0.0-rc.1 (https://github.com/DaphnisDuck/ddmus)"},
	} {
		version = tt.version
		if got := UserAgent(); got != tt.want {
			t.Errorf("UserAgent() with version %q = %q, want %q", tt.version, got, tt.want)
		}
	}
}
