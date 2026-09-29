package main

// omatunes: wiring for the library navigation. Kept out of main.go so
// upstream merges there stay conflict-free.

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bjarneo/cliamp/catalog"
	"github.com/bjarneo/cliamp/external/local"
	"github.com/bjarneo/cliamp/library"
	"github.com/bjarneo/cliamp/ui/model"
)

// librarySources picks the providers the Music root shows. Other configured
// providers stay constructed (playback, resume and IPC still use them) but
// have no root entry until Milestone 4.
func librarySources(providers []model.ProviderEntry, initialDir string, rt *catalogRuntime) library.Sources {
	src := library.Sources{MusicDir: musicDir(initialDir), Catalog: rt.catalog()}
	for _, p := range providers {
		switch p.Key {
		case "spotify":
			src.Spotify = p.Provider
		case "local":
			src.Local = p.Provider
		case "radio":
			src.Radio = p.Provider
		case "cliamp":
			src.Channels = p.Provider
		}
	}
	// A synced provider's menu offers the lists its sync provides.
	if cols := rt.collections(catalog.Spotify); len(cols) > 0 && src.Spotify != nil {
		src.Synced = append(src.Synced, library.SyncedSource{
			Provider: catalog.Spotify, Title: "Spotify", Player: src.Spotify, Collections: cols})
	}
	return src
}

// musicDir is the directory Local's Albums/Artists/Genres are scanned from:
// the configured initial_directory, else $XDG_MUSIC_DIR, else ~/Music.
func musicDir(initialDir string) string {
	if dir := local.ExpandPath(strings.TrimSpace(initialDir)); dir != "" {
		return dir
	}
	if dir := local.ExpandPath(os.Getenv("XDG_MUSIC_DIR")); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "Music")
	}
	return ""
}
