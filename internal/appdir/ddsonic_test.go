package appdir

import (
	"path/filepath"
	"testing"
)

func TestCacheDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, tt := range []struct{ xdg, want string }{
		{"/xdg/cache", "/xdg/cache/ddmus"},
		{"", filepath.Join(home, ".cache", "ddmus")},
		{"relative", filepath.Join(home, ".cache", "ddmus")}, // XDG says to ignore relative paths
	} {
		t.Setenv("XDG_CACHE_HOME", tt.xdg)
		if got, err := CacheDir(); err != nil || got != tt.want {
			t.Errorf("XDG_CACHE_HOME=%q: CacheDir = %q, %v; want %q", tt.xdg, got, err, tt.want)
		}
	}
}
