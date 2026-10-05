package main

// ddmus: wiring for the library navigation. Kept out of main.go so
// upstream merges there stay conflict-free.

import (
	"context"
	"image"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/bjarneo/cliamp/artwork"
	"github.com/bjarneo/cliamp/catalog"
	"github.com/bjarneo/cliamp/external/local"
	"github.com/bjarneo/cliamp/internal/appdir"
	"github.com/bjarneo/cliamp/internal/httpclient"
	"github.com/bjarneo/cliamp/library"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/ui/kittyimg"
	"github.com/bjarneo/cliamp/ui/model"
)

// librarySources picks the providers the Music root shows. Other configured
// providers stay constructed (playback, resume and IPC still use them) but
// have no root entry until each gets its own release.
func librarySources(providers []model.ProviderEntry, initialDir string, rt *catalogRuntime) library.Sources {
	src := library.Sources{MusicDir: local.MusicDir(initialDir), Catalog: rt.catalog()}
	var youtube playlist.Provider
	for _, p := range providers {
		switch p.Key {
		case "ytmusic":
			youtube = p.Provider
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
	// A synced provider's menu offers the lists its sync provides; it
	// needs the provider that plays its tracks.
	if cols := rt.collections(catalog.Spotify); len(cols) > 0 && src.Spotify != nil {
		src.Synced = append(src.Synced, library.SyncedSource{
			Provider: catalog.Spotify, Title: "Spotify", Player: src.Spotify, Collections: cols})
	}
	if cols := rt.collections(catalog.YouTube); len(cols) > 0 && youtube != nil {
		src.Synced = append(src.Synced, library.SyncedSource{
			Provider: catalog.YouTube, Title: "YouTube Music", Player: youtube, Collections: cols,
			LikedTitle: "Liked Music", PartialAlbums: true})
	}
	return src
}

// artworkLoader loads album artwork for the track info view, or is nil when
// artwork is off or the terminal cannot draw images (ui/kittyimg).
func artworkLoader(enabled bool) func(context.Context, artwork.Ref) (image.Image, error) {
	if !enabled || !kittyimg.Supported(os.Getenv) {
		return nil
	}
	dir, err := appdir.CacheDir()
	if err != nil {
		return nil
	}
	l := &artwork.Loader{
		Dir:    filepath.Join(dir, "artwork"),
		Client: &http.Client{Timeout: artworkFetchTimeout, Transport: httpclient.Streaming.Transport, CheckRedirect: artwork.NoDowngrade},
	}
	return l.Load
}

// artworkFetchTimeout bounds one artwork download.
const artworkFetchTimeout = 20 * time.Second
