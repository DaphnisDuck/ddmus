package library

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"sync/atomic"

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

// catalogAlbumsLevel is a catalog Albums list, sorted by title or by artist. The
// order is the browser's, so it holds while the list is reopened.
type catalogAlbumsLevel struct {
	catalogLevel
	order *atomic.Int32 // a catalog.AlbumOrder
}

var albumOrderNames = map[catalog.AlbumOrder]string{catalog.ByTitle: "by title", catalog.ByArtist: "by artist"}

func (l *catalogAlbumsLevel) OrderName() string {
	return albumOrderNames[catalog.AlbumOrder(l.order.Load())]
}

func (l *catalogAlbumsLevel) NextOrder() string {
	next := catalog.ByArtist
	if catalog.AlbumOrder(l.order.Load()) == catalog.ByArtist {
		next = catalog.ByTitle
	}
	l.order.Store(int32(next))
	return albumOrderNames[next]
}

// SyncedSource is a provider the catalog syncs, as its menu needs it.
// Collections are what its sync provides (catalog.Collection*): the menu
// offers a list for each, so a source that has no albums shows none.
type SyncedSource struct {
	Provider    string // catalog provider name, e.g. catalog.Spotify
	Title       string // menu title, e.g. "Spotify"
	Collections []string
	// Player plays the source's tracks and fills gaps the catalog cannot:
	// an album whose tracks are not cached (when the catalog is no
	// catalog.AlbumTrackFetcher), a playlist whose items could not be
	// synced, and an artist's full discography, which only its live API has.
	Player playlist.Provider
	// LikedTitle names the liked list; "Liked Songs" when empty.
	LikedTitle string
	// PartialAlbums marks a source whose albums hold only the tracks the
	// catalog has (YouTube's, derived from enriched tracks): an album opens
	// and plays as it is, never fetched whole.
	PartialAlbums bool
}

// SyncedMenu returns a synced source's menu, backed by the catalog. It
// offers, in this order, Albums, Artists, Playlists and the liked list, each
// only if the source syncs that collection.
func SyncedMenu(cat catalog.Catalog, src SyncedSource) Level {
	b := syncedBrowser(cat, src)
	liked := cmp.Or(src.LikedTitle, "Liked Songs")
	lists := []struct {
		collection string
		entry      Entry
	}{
		{catalog.CollectionAlbums, Entry{Title: "Albums", Open: b.albumsList(b.albums)}},
		{catalog.CollectionArtists, Entry{Title: "Artists", Open: b.list("Artists", b.artists)}},
		{catalog.CollectionPlaylists, Entry{Title: "Playlists", Open: b.list("Playlists", b.playlists)}},
		{catalog.CollectionLiked, Entry{Title: liked, Open: b.list(liked, b.liked)}},
	}
	var entries []Entry
	for _, l := range lists {
		if slices.Contains(src.Collections, l.collection) {
			entries = append(entries, l.entry)
		}
	}
	return Menu(src.Title, entries...)
}

func syncedBrowser(cat catalog.Catalog, src SyncedSource) *catalogBrowser {
	return &catalogBrowser{cat: cat, prov: src.Player, provider: src.Provider, partialAlbums: src.PartialAlbums,
		pending: "Syncing your library… (press r to retry if this persists)"}
}

// catalogBrowser builds one provider's catalog-backed levels and entries,
// for its browse menu and for search results.
type catalogBrowser struct {
	cat      catalog.Catalog
	prov     playlist.Provider
	provider string
	pending  string // shown in an empty list before the first sync
	none     string // shown in an empty list after it; "" shows nothing
	// albumOrder is the Albums list's order, a catalog.AlbumOrder.
	albumOrder atomic.Int32
	// partialAlbums: albums are only the catalog's tracks (SyncedSource).
	partialAlbums bool
}

// albumsList is the Albums list, which the UI can reorder.
func (b *catalogBrowser) albumsList(load func(ctx context.Context) ([]Entry, error)) Level {
	return &catalogAlbumsLevel{catalogLevel{funcLevel{"Albums", load}, b.provider}, &b.albumOrder}
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
	albums, err := b.cat.Albums(ctx, b.provider, catalog.AlbumOrder(b.albumOrder.Load()))
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
		if cached || b.partialAlbums {
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
		entries[i] = b.artistEntry(a)
	}
	return b.syncingPlaceholder(ctx, entries)
}

func (b *catalogBrowser) artistEntry(a catalog.Artist) Entry {
	return Entry{ID: catalogID(a.ID), Title: a.Name, Open: b.level(a.Name, func(ctx context.Context) ([]Entry, error) {
		return b.artistAlbums(ctx, a)
	})}
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
		entries[i] = b.playlistEntry(p)
	}
	return b.syncingPlaceholder(ctx, entries)
}

func (b *catalogBrowser) playlistEntry(p catalog.Playlist) Entry {
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
	return e
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
		Title:        CleanText(t.Title),
		Artist:       CleanText(t.Artist),
		Album:        CleanText(t.AlbumTitle),
		AlbumArtURL:  t.ArtworkURL,
		Year:         t.Year,
		Genre:        CleanText(t.Genre),
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
