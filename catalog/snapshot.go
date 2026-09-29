package catalog

import (
	"context"
	"time"
)

// Snapshot is the complete contents of one provider collection as fetched by
// a sync, for example Spotify's saved albums. A snapshot is all or nothing:
// a source that could not fetch every page returns an error instead of a
// partial snapshot, because applying one reconciles away everything missing.
type Snapshot struct {
	Provider   string
	Collection string
	// The collection's members. A collection usually holds one kind, but may
	// hold several.
	Albums    []AlbumRecord
	Artists   []ArtistRecord
	Tracks    []TrackRecord
	Playlists []PlaylistRecord
}

// ArtistRecord is an artist as a provider describes it.
type ArtistRecord struct {
	Ref      Ref
	Name     string
	ImageURL string
}

// AlbumRecord is an album as a provider describes it. Zero fields mean
// "unknown" and never overwrite what the catalog already has, so an album
// seen only through a liked track does not erase a saved album's details.
type AlbumRecord struct {
	Ref        Ref
	Title      string
	Artists    []ArtistRecord // credit order; empty keeps the stored credit
	Year       int
	TrackCount int
	ArtworkURL string
	AddedAt    time.Time // when it joined the library; zero keeps the stored time
}

// TrackRecord is a track as a provider describes it.
type TrackRecord struct {
	Ref         Ref
	Title       string
	Artists     []ArtistRecord // credit order
	Album       *AlbumRecord   // nil when the provider gives none
	AlbumTitle  string         // display fallback when Album is nil
	Disc        int
	TrackNo     int
	Duration    time.Duration
	PlayableURI string
	Genre       string
	Year        int
	AddedAt     time.Time
}

// PlaylistRecord is a playlist as a provider describes it.
type PlaylistRecord struct {
	Ref        Ref
	Name       string
	Own        bool
	Snapshot   string // provider change marker, e.g. Spotify's snapshot_id
	TrackCount int
	// Tracks replaces the playlist's stored track list when TracksFetched is
	// set. A source skips fetching tracks whose Snapshot is unchanged and
	// leaves TracksFetched false, keeping the stored list.
	Tracks        []TrackRecord
	TracksFetched bool
	AddedAt       time.Time
}

// Writer is the write side of the catalog, used only by catalogsync.
type Writer interface {
	// ApplySnapshot makes the collection match snap in one transaction: it
	// upserts every record, marks the members seen with a new generation,
	// removes the collection's members that were not seen, and records the
	// sync as successful. Entities left unreferenced stay until Sweep.
	ApplySnapshot(ctx context.Context, snap Snapshot) error
	// Sweep deletes provider's entities that nothing keeps alive: no library
	// membership, playlist, local file or cached album references them.
	Sweep(ctx context.Context, provider string) error
	// RecordSyncFailure records a failed attempt without touching the
	// collection's contents or its last success.
	RecordSyncFailure(ctx context.Context, provider, collection string, cause error) error
	// PlaylistSnapshots returns the stored change marker of each of
	// provider's playlists, keyed by provider ID.
	PlaylistSnapshots(ctx context.Context, provider string) (map[string]string, error)
}
