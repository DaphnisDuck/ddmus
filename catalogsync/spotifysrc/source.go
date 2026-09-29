// Package spotifysrc is the catalog sync source for a Spotify library:
// saved albums, followed artists, liked tracks and library playlists.
package spotifysrc

import (
	"context"
	"errors"
	"fmt"

	"github.com/bjarneo/cliamp/applog"
	"github.com/bjarneo/cliamp/catalog"
	"github.com/bjarneo/cliamp/catalogsync"
)

// Collection names, as recorded in the catalog's sync state.
const (
	Albums    = "albums"
	Artists   = "artists"
	Liked     = "liked"
	Playlists = "playlists"
)

// Client is the part of the Spotify provider the source uses. Each method
// returns a whole collection or an error, never part of one.
type Client interface {
	SavedAlbumRecords(ctx context.Context) ([]catalog.AlbumRecord, error)
	FollowedArtistRecords(ctx context.Context) ([]catalog.ArtistRecord, error)
	LikedTrackRecords(ctx context.Context) ([]catalog.TrackRecord, error)
	PlaylistRecords(ctx context.Context) ([]catalog.PlaylistRecord, error)
	PlaylistTrackRecords(ctx context.Context, playlistID string) ([]catalog.TrackRecord, error)
	// AlbumTrackRecords retries rate limits; AlbumTrackRecordsOnce returns
	// a *catalog.RateLimitError instead.
	AlbumTrackRecords(ctx context.Context, albumID string) ([]catalog.TrackRecord, error)
	AlbumTrackRecordsOnce(ctx context.Context, albumID string) ([]catalog.TrackRecord, error)
}

// Source syncs one Spotify account.
type Source struct {
	client Client
}

var (
	_ catalogsync.Source      = (*Source)(nil)
	_ catalogsync.AlbumSource = (*Source)(nil)
)

// New returns a Source reading through client.
func New(client Client) *Source { return &Source{client: client} }

// Provider implements catalogsync.Source.
func (*Source) Provider() string { return catalog.Spotify }

// Collections implements catalogsync.Source. Playlists come last: they are
// the slowest and the least likely to change.
func (*Source) Collections() []string { return []string{Albums, Artists, Liked, Playlists} }

// Fetch implements catalogsync.Source.
func (s *Source) Fetch(ctx context.Context, collection string, known catalogsync.Known) (catalog.Snapshot, error) {
	snap := catalog.Snapshot{Provider: catalog.Spotify, Collection: collection}
	var err error
	switch collection {
	case Albums:
		snap.Albums, err = s.client.SavedAlbumRecords(ctx)
	case Artists:
		snap.Artists, err = s.client.FollowedArtistRecords(ctx)
	case Liked:
		snap.Tracks, err = s.client.LikedTrackRecords(ctx)
	case Playlists:
		snap.Playlists, err = s.playlists(ctx, known.PlaylistSnapshots)
	default:
		err = fmt.Errorf("unknown collection %q", collection)
	}
	if err != nil {
		return catalog.Snapshot{}, err
	}
	return snap, nil
}

// playlists lists the library playlists and fetches tracks only for those
// whose Spotify snapshot changed since the catalog stored them. A playlist
// Spotify will not let this app read keeps its stored tracks; any other
// failure fails the whole collection.
func (s *Source) playlists(ctx context.Context, known map[string]string) ([]catalog.PlaylistRecord, error) {
	lists, err := s.client.PlaylistRecords(ctx)
	if err != nil {
		return nil, err
	}
	for i := range lists {
		p := &lists[i]
		if p.Snapshot != "" && known[p.Ref.ProviderID] == p.Snapshot {
			continue // unchanged: keep the stored tracks
		}
		tracks, err := s.client.PlaylistTrackRecords(ctx, p.Ref.ProviderID)
		if errors.Is(err, catalog.ErrForbidden) {
			applog.Info("catalog sync: skipping tracks of playlist %q: %v", p.Name, err)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("playlist %q: %w", p.Name, err)
		}
		p.Tracks, p.TracksFetched = tracks, true
	}
	return lists, nil
}

// AlbumTracks implements catalogsync.AlbumSource.
func (s *Source) AlbumTracks(ctx context.Context, album catalog.Ref) ([]catalog.TrackRecord, error) {
	return s.client.AlbumTrackRecords(ctx, album.ProviderID)
}

// AlbumTracksOnce implements catalogsync.AlbumSource.
func (s *Source) AlbumTracksOnce(ctx context.Context, album catalog.Ref) ([]catalog.TrackRecord, error) {
	return s.client.AlbumTrackRecordsOnce(ctx, album.ProviderID)
}
