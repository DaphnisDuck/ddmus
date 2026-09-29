// Package youtubesrc is the catalog sync source for a YouTube Music
// account: its music playlists and Liked Music.
package youtubesrc

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
	Liked     = catalog.CollectionLiked
	Playlists = catalog.CollectionPlaylists
)

// Client reads the account. Each method returns a whole collection or an
// error, never part of one.
type Client interface {
	// PlaylistRecords lists the music playlists, without tracks. A record
	// with a Snapshot is refetched only when it changes.
	PlaylistRecords(ctx context.Context) ([]catalog.PlaylistRecord, error)
	PlaylistTrackRecords(ctx context.Context, playlistID string) ([]catalog.TrackRecord, error)
	LikedTrackRecords(ctx context.Context) ([]catalog.TrackRecord, error)
}

// Source syncs one YouTube Music account.
type Source struct {
	client Client
}

var _ catalogsync.Source = (*Source)(nil)

// New returns a Source reading through client.
func New(client Client) *Source { return &Source{client: client} }

// Provider implements catalogsync.Source.
func (*Source) Provider() string { return catalog.YouTube }

// Collections implements catalogsync.Source. Liked Music first: it is one
// read, while playlists take one each.
func (*Source) Collections() []string { return []string{Liked, Playlists} }

// Fetch implements catalogsync.Source.
func (s *Source) Fetch(ctx context.Context, collection string, known catalogsync.Known) (catalog.Snapshot, error) {
	snap := catalog.Snapshot{Provider: catalog.YouTube, Collection: collection}
	var err error
	switch collection {
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

// playlists lists the music playlists and fetches the tracks of each whose
// change marker is missing or changed. A playlist YouTube will not show
// keeps its stored tracks; any other failure fails the whole collection.
func (s *Source) playlists(ctx context.Context, known map[string]string) ([]catalog.PlaylistRecord, error) {
	lists, err := s.client.PlaylistRecords(ctx)
	if err != nil {
		return nil, err
	}
	for i := range lists {
		p := &lists[i]
		if p.Snapshot != "" && known[p.Ref.ProviderID] == p.Snapshot {
			continue
		}
		tracks, err := s.client.PlaylistTrackRecords(ctx, p.Ref.ProviderID)
		if errors.Is(err, catalog.ErrForbidden) {
			applog.Info("catalog sync: skipping tracks of playlist %q: %v", p.Name, err)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("playlist %q: %w", p.Name, err)
		}
		p.Tracks, p.TracksFetched, p.TrackCount = tracks, true, len(tracks)
	}
	return lists, nil
}
