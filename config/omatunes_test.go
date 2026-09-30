// omatunes: tests for the [omatunes] config section.

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadOmatunesSection(t *testing.T) {
	tests := []struct {
		name string
		toml string
		want time.Duration
	}{
		{"default without the section", "volume = -3\n", DefaultSpotifyRefresh},
		{"quoted duration", "[omatunes]\nspotify_refresh = \"2h\"\n", 2 * time.Hour},
		{"bare duration", "[omatunes]\nspotify_refresh = 90s\n", 90 * time.Second},
		{"zero syncs every startup", "[omatunes]\nspotify_refresh = \"0s\"\n", 0},
		{"invalid keeps the default", "[omatunes]\nspotify_refresh = \"soon\"\n", DefaultSpotifyRefresh},
		{"negative keeps the default", "[omatunes]\nspotify_refresh = \"-5m\"\n", DefaultSpotifyRefresh},
		{"top-level key is not the section's", "spotify_refresh = \"2h\"\n", DefaultSpotifyRefresh},
	}
	for _, tt := range []struct {
		name string
		toml string
		want time.Duration
	}{
		{"youtube default", "", DefaultYouTubeRefresh},
		{"youtube set", "[omatunes]\nyoutube_refresh = \"6h\"\n", 6 * time.Hour},
		{"youtube invalid", "[omatunes]\nyoutube_refresh = \"often\"\n", DefaultYouTubeRefresh},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			path := filepath.Join(os.Getenv("HOME"), ".config", "ddmus", "config.toml")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tt.toml), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Omatunes.YouTubeRefresh != tt.want {
				t.Errorf("YouTubeRefresh = %v, want %v", cfg.Omatunes.YouTubeRefresh, tt.want)
			}
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			path := filepath.Join(os.Getenv("HOME"), ".config", "ddmus", "config.toml")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tt.toml), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Omatunes.SpotifyRefresh != tt.want {
				t.Errorf("SpotifyRefresh = %v, want %v", cfg.Omatunes.SpotifyRefresh, tt.want)
			}
		})
	}
}

// A section that follows [omatunes] gets its own keys back.
func TestOmatunesSectionDoesNotLeak(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(os.Getenv("HOME"), ".config", "ddmus", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	toml := "[omatunes]\nspotify_refresh = \"1h\"\n\n[spotify]\nbitrate = 160\n"
	if err := os.WriteFile(path, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Omatunes.SpotifyRefresh != time.Hour || !cfg.Spotify.Enabled || cfg.Spotify.Bitrate != 160 {
		t.Errorf("omatunes %+v, spotify %+v", cfg.Omatunes, cfg.Spotify)
	}
}

func TestLoadYouTubePlaylists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(os.Getenv("HOME"), ".config", "ddmus", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	toml := "[omatunes]\nyoutube_playlists = [\"https://music.youtube.com/playlist?list=PLa&si=x\", \"PLb\"]\n"
	if err := os.WriteFile(path, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://music.youtube.com/playlist?list=PLa&si=x", "PLb"}
	if len(cfg.Omatunes.YouTubePlaylists) != 2 || cfg.Omatunes.YouTubePlaylists[0] != want[0] || cfg.Omatunes.YouTubePlaylists[1] != want[1] {
		t.Errorf("YouTubePlaylists = %q, want %q", cfg.Omatunes.YouTubePlaylists, want)
	}
}
