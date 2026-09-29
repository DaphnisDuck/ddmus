package appmeta

import "testing"

func TestDefaults(t *testing.T) {
	if got := ClientName(); got != "omatunes" {
		t.Fatalf("ClientName() = %q, want %q", got, "omatunes")
	}
	if got := DeviceName(); got != "omatunes" {
		t.Fatalf("DeviceName() = %q, want %q", got, "omatunes")
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
