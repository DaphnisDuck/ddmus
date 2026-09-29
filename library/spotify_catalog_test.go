package library

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/catalog"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/provider"
)

// fakeCatalog is an in-memory catalog.Catalog.
type fakeCatalog struct {
	albums      []catalog.Album
	albumTracks map[int64][]catalog.Track
	cached      map[int64]bool
	artists     []catalog.Artist
	artistAlbs  map[int64][]catalog.Album
	playlists   []catalog.Playlist
	plTracks    map[int64][]catalog.Track
	liked       []catalog.Track
	synced      bool
}

func (f *fakeCatalog) Albums(context.Context, string) ([]catalog.Album, error) { return f.albums, nil }
func (f *fakeCatalog) AlbumTracks(_ context.Context, id int64) ([]catalog.Track, bool, error) {
	return f.albumTracks[id], f.cached[id], nil
}
func (f *fakeCatalog) Artists(context.Context, string) ([]catalog.Artist, error) {
	return f.artists, nil
}
func (f *fakeCatalog) ArtistAlbums(_ context.Context, id int64) ([]catalog.Album, error) {
	return f.artistAlbs[id], nil
}
func (f *fakeCatalog) Playlists(context.Context, string) ([]catalog.Playlist, error) {
	return f.playlists, nil
}
func (f *fakeCatalog) PlaylistTracks(_ context.Context, id int64) ([]catalog.Track, error) {
	return f.plTracks[id], nil
}
func (f *fakeCatalog) LikedTracks(context.Context, string) ([]catalog.Track, error) {
	return f.liked, nil
}
func (f *fakeCatalog) SyncStatus(context.Context, string) ([]catalog.CollectionStatus, error) {
	if !f.synced {
		return nil, nil
	}
	return []catalog.CollectionStatus{{Collection: "albums", LastSuccess: time.UnixMilli(1)}}, nil
}

// liveSpotify is the provider side: live album tracks, discography, and
// playlist tracks, with a switch to simulate being offline.
type liveSpotify struct {
	fakeProvider
	offline bool
	calls   []string
}

func (l *liveSpotify) AlbumTracks(id string) ([]playlist.Track, error) {
	l.calls = append(l.calls, "album:"+id)
	return []playlist.Track{{Path: "spotify:track:live-" + id, Title: "live"}}, nil
}
func (l *liveSpotify) Artists() ([]provider.ArtistInfo, error) { return nil, nil }
func (l *liveSpotify) ArtistAlbums(id string) ([]provider.AlbumInfo, error) {
	l.calls = append(l.calls, "artist:"+id)
	if l.offline {
		return nil, errors.New("dial tcp: no route to host")
	}
	return []provider.AlbumInfo{{ID: "disco", Name: "Discography Album", Year: 2001}}, nil
}
func (l *liveSpotify) Tracks(id string) ([]playlist.Track, error) {
	l.calls = append(l.calls, "playlist:"+id)
	return []playlist.Track{{Path: "spotify:track:pl-" + id, Title: "live"}}, nil
}

func sref2(id string) catalog.Ref { return catalog.Ref{Provider: catalog.Spotify, ProviderID: id} }

func newCatalogFixture() (*fakeCatalog, *liveSpotify, Level) {
	cat := &fakeCatalog{
		synced: true,
		albums: []catalog.Album{
			{ID: 1, Ref: sref2("al-cached"), Title: "Cached", Artist: "Ozawa"},
			{ID: 2, Ref: sref2("al-new"), Title: "Not Cached", Artist: "Muti"},
		},
		albumTracks: map[int64][]catalog.Track{1: {{ID: 10, Title: "I.", PlayableURI: "spotify:track:t10", Duration: time.Minute, ArtworkURL: "https://img"}}},
		cached:      map[int64]bool{1: true},
		artists:     []catalog.Artist{{ID: 5, Ref: sref2("ar-ozawa"), Name: "Ozawa"}},
		artistAlbs:  map[int64][]catalog.Album{5: {{ID: 1, Ref: sref2("al-cached"), Title: "Cached", Year: 1990}}},
		playlists: []catalog.Playlist{
			{ID: 20, Ref: sref2("pl-mine"), Name: "Mine", Own: true, TrackCount: 1},
			{ID: 21, Ref: sref2("pl-locked"), Name: "Locked", TrackCount: 3},
		},
		plTracks: map[int64][]catalog.Track{20: {{ID: 11, Title: "Song", PlayableURI: "spotify:track:t11"}}},
		liked:    []catalog.Track{{ID: 12, Title: "Liked", PlayableURI: "spotify:track:t12"}},
	}
	live := &liveSpotify{fakeProvider: fakeProvider{name: "Spotify"}}
	return cat, live, SpotifyCatalog(cat, live)
}

func TestSpotifyCatalogReadsTheCatalog(t *testing.T) {
	_, live, root := newCatalogFixture()
	albums := load(t, child(t, root, "Albums"))
	if got := titles(albums); !slices.Equal(got, []string{"Cached", "Not Cached"}) || albums[0].ID != "1" {
		t.Fatalf("albums = %+v", albums)
	}
	if cl, ok := child(t, root, "Albums").(CatalogLevel); !ok || cl.CatalogProvider() != catalog.Spotify {
		t.Error("Albums level is not a catalog level")
	}

	cached := load(t, albums[0].Open)
	if len(cached) != 1 || cached[0].Track.Path != "spotify:track:t10" || cached[0].Track.DurationSecs != 60 ||
		cached[0].Track.AlbumArtURL != "https://img" || cached[0].ID != "10" {
		t.Errorf("cached album tracks = %+v", cached)
	}
	if len(live.calls) != 0 {
		t.Errorf("a cached album called the provider: %v", live.calls)
	}
	// An uncached album is fetched live by its Spotify ID.
	if uncached := load(t, albums[1].Open); len(uncached) != 1 || !slices.Equal(live.calls, []string{"album:al-new"}) {
		t.Errorf("uncached album = %+v, calls %v", uncached, live.calls)
	}

	liked := load(t, child(t, root, "Liked Songs"))
	if len(liked) != 1 || liked[0].Track.Path != "spotify:track:t12" {
		t.Errorf("liked = %+v", liked)
	}
}

func TestSpotifyCatalogPlaylists(t *testing.T) {
	_, live, root := newCatalogFixture()
	pls := load(t, child(t, root, "Playlists"))
	if len(pls) != 2 || pls[0].Section != SpotifyOwnPlaylistsSection || pls[1].Section != SpotifyFollowedPlaylistsSection {
		t.Fatalf("playlists = %+v", pls)
	}
	if got := load(t, pls[0].Open); len(got) != 1 || len(live.calls) != 0 {
		t.Errorf("synced playlist = %+v, calls %v", got, live.calls)
	}
	// Tracks the sync could not read come live.
	if got := load(t, pls[1].Open); len(got) != 1 || !slices.Equal(live.calls, []string{"playlist:pl-locked"}) {
		t.Errorf("unsynced playlist = %+v, calls %v", got, live.calls)
	}
}

func TestSpotifyCatalogArtistDiscography(t *testing.T) {
	_, live, root := newCatalogFixture()
	artist := child(t, child(t, root, "Artists"), "Ozawa")
	if got := titles(load(t, artist)); !slices.Equal(got, []string{"Discography Album"}) {
		t.Errorf("online discography = %v", got)
	}
	live.offline = true
	if got := titles(load(t, artist)); !slices.Equal(got, []string{"Cached"}) {
		t.Errorf("offline discography = %v, want the catalog's albums", got)
	}
}

func TestSpotifyCatalogBeforeFirstSync(t *testing.T) {
	cat, live, _ := newCatalogFixture()
	cat.synced, cat.albums = false, nil
	root := SpotifyCatalog(cat, live)
	got := load(t, child(t, root, "Albums"))
	if len(got) != 1 || got[0].Open != nil || got[0].Track != nil {
		t.Errorf("empty unsynced albums = %+v, want one placeholder row", got)
	}
	// After a sync, an empty list is simply empty.
	cat.synced = true
	if got := load(t, child(t, root, "Albums")); len(got) != 0 {
		t.Errorf("empty synced albums = %+v, want none", got)
	}
}

func TestRootUsesCatalogWhenSet(t *testing.T) {
	cat, live, _ := newCatalogFixture()
	spotify := child(t, Root(Sources{Spotify: live, Catalog: cat}), "Spotify")
	if _, ok := child(t, spotify, "Albums").(CatalogLevel); !ok {
		t.Error("Root with a catalog did not use catalog levels")
	}
	spotify = child(t, Root(Sources{Spotify: live}), "Spotify")
	if _, ok := child(t, spotify, "Playlists").(CatalogLevel); ok {
		t.Error("Root without a catalog used catalog levels")
	}
}

// fetchingCatalog is a catalog that can fetch and cache uncached albums.
type fetchingCatalog struct {
	*fakeCatalog
	fetched []int64
}

func (f *fetchingCatalog) FetchAlbumTracks(_ context.Context, a catalog.Album) ([]catalog.Track, error) {
	f.fetched = append(f.fetched, a.ID)
	f.cached[a.ID] = true
	f.albumTracks[a.ID] = []catalog.Track{{ID: 30, Title: "Fetched", PlayableURI: "spotify:track:t30"}}
	return f.albumTracks[a.ID], nil
}

func TestSpotifyCatalogCachesUncachedAlbums(t *testing.T) {
	cat, live, _ := newCatalogFixture()
	fc := &fetchingCatalog{fakeCatalog: cat}
	albums := load(t, child(t, SpotifyCatalog(fc, live), "Albums"))
	for range 2 {
		got := load(t, albums[1].Open)
		if len(got) != 1 || got[0].ID != "30" || got[0].Track.Path != "spotify:track:t30" {
			t.Errorf("uncached album = %+v", got)
		}
	}
	// Fetched once through the catalog, then read from the cache; never live.
	if !slices.Equal(fc.fetched, []int64{2}) || len(live.calls) != 0 {
		t.Errorf("fetched %v, live calls %v", fc.fetched, live.calls)
	}
}
