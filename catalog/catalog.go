// Package catalog is omatunes' music catalog: the albums, artists, tracks
// and playlists known across providers, and which of them are in the user's
// library. It defines the types and the interface the UI reads through; it
// holds no SQL and no Bubbletea. catalog/sqlite implements it, and
// catalogsync keeps it current from the providers.
//
// Every object has an internal ID owned by the catalog. The provider's own ID
// (Provider, ProviderID) is kept alongside it and is unique, but is never the
// catalog's key.
package catalog

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound means a requested object is not in the catalog.
var ErrNotFound = errors.New("not in catalog")

// ErrForbidden means a provider refuses this client access to one item, for
// example a Spotify playlist it may list but not read. A sync skips the item
// and keeps what the catalog already holds for it.
var ErrForbidden = errors.New("provider refused access")

// Provider names used in the catalog.
const (
	Spotify = "spotify"
	Local   = "local"
)

// Kind is what a library membership row refers to.
type Kind string

// Library membership kinds. For Spotify: saved albums, followed artists,
// liked tracks and library playlists. For Local: indexed albums and artists.
const (
	KindAlbum    Kind = "album"
	KindArtist   Kind = "artist"
	KindTrack    Kind = "track"
	KindPlaylist Kind = "playlist"
)

// Ref is a provider's identity for an object.
type Ref struct {
	Provider   string
	ProviderID string
}

// Artist is a catalog artist.
type Artist struct {
	ID       int64
	Ref      Ref
	Name     string
	ImageURL string
}

// Album is a catalog album. Artist is the display credit, joined from the
// album's artists in order.
type Album struct {
	ID         int64
	Ref        Ref
	Title      string
	Artist     string
	Year       int
	TrackCount int
	ArtworkURL string
	// TracksCached reports whether the album's track list is in the catalog.
	TracksCached bool
}

// Track is a catalog track. PlayableURI is what the player is given
// (spotify:track:…, a file path, …); Artist is the joined display credit.
type Track struct {
	ID          int64
	Ref         Ref
	Title       string
	Artist      string
	AlbumID     int64 // 0 when the track has no catalog album
	AlbumTitle  string
	Disc        int
	TrackNo     int
	Duration    time.Duration
	PlayableURI string
	Genre       string
	Year        int
}

// Playlist is a catalog playlist.
type Playlist struct {
	ID         int64
	Ref        Ref
	Name       string
	Own        bool // owned by the user rather than followed
	TrackCount int
}

// CollectionStatus is the last sync outcome for one provider collection.
type CollectionStatus struct {
	Collection  string
	LastAttempt time.Time // zero if never attempted
	LastSuccess time.Time // zero if never succeeded
	LastError   string    // from the last attempt; empty when it succeeded
}

// Catalog is the read side the UI uses. Every method is a local query: none
// reaches the network.
type Catalog interface {
	// Albums returns provider's library albums, by artist then title.
	Albums(ctx context.Context, provider string) ([]Album, error)
	// AlbumTracks returns an album's tracks in disc and track order, and
	// whether its track list has been cached at all.
	AlbumTracks(ctx context.Context, albumID int64) (tracks []Track, cached bool, err error)
	// Artists returns provider's library artists, by name.
	Artists(ctx context.Context, provider string) ([]Artist, error)
	// ArtistAlbums returns every catalog album credited to the artist,
	// newest first.
	ArtistAlbums(ctx context.Context, artistID int64) ([]Album, error)
	// Playlists returns provider's library playlists: the user's own first,
	// then followed, each by name.
	Playlists(ctx context.Context, provider string) ([]Playlist, error)
	// PlaylistTracks returns a playlist's tracks in playlist order.
	PlaylistTracks(ctx context.Context, playlistID int64) ([]Track, error)
	// LikedTracks returns provider's liked tracks, most recently liked first.
	LikedTracks(ctx context.Context, provider string) ([]Track, error)
	// SyncStatus returns the last sync outcome of each of provider's
	// collections.
	SyncStatus(ctx context.Context, provider string) ([]CollectionStatus, error)
}
