package library

import (
	"context"
	"fmt"
	"strconv"

	"github.com/bjarneo/cliamp/catalog"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/provider"
)

// catalogLevel is a top-level list read from the catalog, so it loads
// instantly and offline. The UI reloads it when a sync of its provider
// lands. Levels beneath it (album tracks, discographies) are ordinary levels:
// some read the network, and a sync must not re-run those.
type catalogLevel struct {
	funcLevel
	provider string
}

func (l *catalogLevel) CatalogProvider() string { return l.provider }

// SpotifyCatalog returns the Spotify menu backed by the synced catalog.
// An album whose tracks are not cached yet is fetched through the catalog
// when it is a catalog.AlbumTrackFetcher, and cached on the way. prov plays
// tracks and fills the other gaps: albums a plain catalog cannot fetch, a
// playlist whose items could not be synced, and an artist's full
// discography, which only the live API has.
func SpotifyCatalog(cat catalog.Catalog, prov playlist.Provider) Level {
	b := &catalogBrowser{cat: cat, prov: prov, provider: catalog.Spotify,
		pending: "Syncing your library… (press r to retry if this persists)"}
	return Menu("Spotify",
		Entry{Title: "Albums", Open: b.list("Albums", b.albums)},
		Entry{Title: "Artists", Open: b.list("Artists", b.artists)},
		Entry{Title: "Playlists", Open: b.list("Playlists", b.playlists)},
		Entry{Title: "Liked Songs", Open: b.list("Liked Songs", b.liked)},
	)
}

type catalogBrowser struct {
	cat      catalog.Catalog
	prov     playlist.Provider
	provider string
	pending  string // shown in an empty list before the first sync
	none     string // shown in an empty list after it; "" shows nothing
}

// list is a top-level catalog list, refreshed after syncs.
func (b *catalogBrowser) list(title string, load func(ctx context.Context) ([]Entry, error)) Level {
	return &catalogLevel{funcLevel{title, load}, b.provider}
}

// level is a level beneath the lists. It may fall back to the provider, so
// it offers sign-in and is not refreshed by syncs.
func (b *catalogBrowser) level(title string, load func(ctx context.Context) ([]Entry, error)) Level {
	return providerLevel(title, b.prov, load)
}

// syncingPlaceholder stands in for an empty list: before the first sync has
// finished, so an empty cache reads as "coming" rather than "you have none",
// and after it when the browser names what is missing.
func (b *catalogBrowser) syncingPlaceholder(ctx context.Context, entries []Entry) ([]Entry, error) {
	if len(entries) > 0 {
		return entries, nil
	}
	status, err := b.cat.SyncStatus(ctx, b.provider)
	if err != nil {
		return nil, err
	}
	for _, st := range status {
		if !st.LastSuccess.IsZero() {
			if b.none != "" {
				return []Entry{{Title: b.none}}, nil
			}
			return entries, nil
		}
	}
	return []Entry{{Title: b.pending}}, nil
}

func (b *catalogBrowser) albums(ctx context.Context) ([]Entry, error) {
	albums, err := b.cat.Albums(ctx, b.provider)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(albums))
	for i, a := range albums {
		entries[i] = b.albumEntry(a, a.Artist)
	}
	return b.syncingPlaceholder(ctx, entries)
}

func (b *catalogBrowser) albumEntry(a catalog.Album, detail string) Entry {
	return Entry{ID: catalogID(a.ID), Title: a.Title, Detail: detail, Open: b.level(a.Title, func(ctx context.Context) ([]Entry, error) {
		tracks, cached, err := b.cat.AlbumTracks(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		if cached {
			return catalogTrackEntries(tracks), nil
		}
		// Not cached yet: fetch and cache it, or at least show it live.
		if f, ok := b.cat.(catalog.AlbumTrackFetcher); ok {
			fetched, err := f.FetchAlbumTracks(ctx, a)
			if err != nil {
				return nil, err
			}
			return catalogTrackEntries(fetched), nil
		}
		loader, ok := b.prov.(provider.AlbumTrackLoader)
		if !ok {
			return catalogTrackEntries(tracks), nil
		}
		live, err := loader.AlbumTracks(a.Ref.ProviderID)
		if err != nil {
			return nil, err
		}
		return trackEntries(live), nil
	})}
}

func (b *catalogBrowser) artists(ctx context.Context) ([]Entry, error) {
	artists, err := b.cat.Artists(ctx, b.provider)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(artists))
	for i, a := range artists {
		entries[i] = Entry{ID: catalogID(a.ID), Title: a.Name, Open: b.level(a.Name, func(ctx context.Context) ([]Entry, error) {
			return b.artistAlbums(ctx, a)
		})}
	}
	return b.syncingPlaceholder(ctx, entries)
}

// artistAlbums shows the artist's full discography from the provider, and
// falls back to the albums the catalog knows when the provider is out of
// reach (offline, rate-limited).
func (b *catalogBrowser) artistAlbums(ctx context.Context, a catalog.Artist) ([]Entry, error) {
	if ab, ok := b.prov.(provider.ArtistBrowser); ok {
		live, liveErr := ab.ArtistAlbums(a.Ref.ProviderID)
		if liveErr == nil {
			return artistAlbumEntries(b.prov, live), nil
		}
		cached, err := b.cat.ArtistAlbums(ctx, a.ID)
		if err != nil || len(cached) == 0 {
			return nil, liveErr
		}
		return b.catalogArtistAlbums(cached), nil
	}
	cached, err := b.cat.ArtistAlbums(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	return b.catalogArtistAlbums(cached), nil
}

func (b *catalogBrowser) catalogArtistAlbums(albums []catalog.Album) []Entry {
	entries := make([]Entry, len(albums))
	for i, al := range albums {
		entries[i] = b.albumEntry(al, yearDetail(al.Year))
	}
	return entries
}

func (b *catalogBrowser) playlists(ctx context.Context) ([]Entry, error) {
	pls, err := b.cat.Playlists(ctx, b.provider)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(pls))
	for i, p := range pls {
		section := SpotifyFollowedPlaylistsSection
		if p.Own {
			section = SpotifyOwnPlaylistsSection
		}
		e := Entry{ID: catalogID(p.ID), Title: p.Name, Section: section, Open: b.level(p.Name, func(ctx context.Context) ([]Entry, error) {
			tracks, err := b.cat.PlaylistTracks(ctx, p.ID)
			if err != nil {
				return nil, err
			}
			// Items the sync could not read (Spotify refuses some followed
			// playlists) are fetched live instead.
			if len(tracks) == 0 && p.TrackCount > 0 {
				live, err := b.prov.Tracks(p.Ref.ProviderID)
				if err != nil {
					return nil, err
				}
				return trackEntries(live), nil
			}
			return catalogTrackEntries(tracks), nil
		})}
		if p.TrackCount > 0 {
			e.Detail = fmt.Sprintf("%d tracks", p.TrackCount)
		}
		entries[i] = e
	}
	return b.syncingPlaceholder(ctx, entries)
}

func (b *catalogBrowser) liked(ctx context.Context) ([]Entry, error) {
	tracks, err := b.cat.LikedTracks(ctx, b.provider)
	if err != nil {
		return nil, err
	}
	return b.syncingPlaceholder(ctx, catalogTrackEntries(tracks))
}

func catalogID(id int64) string { return strconv.FormatInt(id, 10) }

// PlayableTrack converts a catalog track into what the player plays. The
// path is the provider's playable URI, so playback routes through the
// provider exactly as it does for live browsing.
func PlayableTrack(t catalog.Track) playlist.Track {
	return playlist.Track{
		Path:         t.PlayableURI,
		Title:        t.Title,
		Artist:       t.Artist,
		Album:        t.AlbumTitle,
		AlbumArtURL:  t.ArtworkURL,
		Year:         t.Year,
		Genre:        t.Genre,
		TrackNumber:  t.TrackNo,
		DurationSecs: int(t.Duration.Seconds()),
	}
}

func catalogTrackEntries(tracks []catalog.Track) []Entry {
	out := make([]playlist.Track, len(tracks))
	for i, t := range tracks {
		out[i] = PlayableTrack(t)
	}
	entries := trackEntries(out)
	for i, t := range tracks {
		entries[i].ID = catalogID(t.ID)
	}
	return entries
}
