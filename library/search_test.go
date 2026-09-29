package library

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/bjarneo/cliamp/catalog"
	"github.com/bjarneo/cliamp/playlist"
)

// searchable is a Spotify provider that can also search live, and a radio
// provider with a directory.
type searchable struct{ liveSpotify }

func (*searchable) SearchTracks(context.Context, string, int) ([]playlist.Track, error) {
	return nil, nil
}

type directoryRadio struct{ fakeProvider }

func (*directoryRadio) SearchStationTracks(q string) ([]playlist.Track, error) {
	return []playlist.Track{{Path: "https://dir/" + q, Title: "Directory " + q}}, nil
}

func lref3(p, id string) catalog.Ref { return catalog.Ref{Provider: p, ProviderID: id} }

func newSearchFixture(t *testing.T) (*fakeCatalog, *searchable, Level) {
	t.Helper()
	cat, _, _ := newCatalogFixture()
	spotifyAlbum := cat.albums[0] // cached, ID 1
	localAlbum := catalog.Album{ID: 40, Ref: lref3(catalog.Local, "/m/holst"), Title: "The Planets", Artist: "Holst", Year: 1990}
	cat.albums = append(cat.albums, localAlbum)
	cat.albumTracks[40] = []catalog.Track{
		{ID: 41, Ref: lref3(catalog.Local, "/m/holst/1.flac"), Title: "Mars", AlbumID: 40, PlayableURI: "/m/holst/1.flac"},
		{ID: 42, Ref: lref3(catalog.Local, "/m/holst/2.flac"), Title: "Venus", AlbumID: 40, PlayableURI: "/m/holst/2.flac"},
	}
	cat.found = catalog.SearchResults{
		catalog.SearchArtist: {
			{Kind: catalog.SearchArtist, Provider: catalog.Spotify, Artist: &catalog.Artist{ID: 5, Ref: sref2("ar-ozawa"), Name: "Ozawa"}},
			{Kind: catalog.SearchArtist, Provider: catalog.Local, Artist: &catalog.Artist{ID: 6, Ref: lref3(catalog.Local, "holst"), Name: "Holst"}},
		},
		catalog.SearchAlbum: {
			{Kind: catalog.SearchAlbum, Provider: catalog.Spotify, Album: &spotifyAlbum},
			{Kind: catalog.SearchAlbum, Provider: catalog.Local, Album: &localAlbum},
		},
		catalog.SearchTrack: {
			{Kind: catalog.SearchTrack, Provider: catalog.Local, Track: &cat.albumTracks[40][1]},
		},
		catalog.SearchPlaylist: {
			{Kind: catalog.SearchPlaylist, Provider: catalog.Spotify, Playlist: &cat.playlists[0]},
		},
		catalog.SearchStation: {
			{Kind: catalog.SearchStation, Provider: catalog.Radio, Track: &catalog.Track{ID: 50, Ref: lref3(catalog.Radio, "https://wbgo"), Title: "WBGO", PlayableURI: "https://wbgo"}},
		},
	}
	sp := &searchable{liveSpotify{fakeProvider: fakeProvider{name: "Spotify"}}}
	root := Root(Sources{Spotify: sp, Local: &fakeProvider{name: "Local"}, Radio: &directoryRadio{fakeProvider{name: "Radio"}},
		MusicDir: "/m", Catalog: cat})
	return cat, sp, child(t, root, "Search")
}

func searchFor(t *testing.T, root Level, query string) []Entry {
	t.Helper()
	sl, ok := root.(SearchLevel)
	if !ok {
		t.Fatalf("Search is %T, not a SearchLevel", root)
	}
	return load(t, sl.WithQuery(query))
}

func TestSearchResultsBySection(t *testing.T) {
	_, _, search := newSearchFixture(t)
	if got := load(t, search); len(got) != 0 {
		t.Errorf("empty query = %+v, want nothing", got)
	}
	rows := searchFor(t, search, "holst")
	type row struct{ section, title, detail string }
	var got []row
	for _, e := range rows {
		got = append(got, row{e.Section, e.Title, e.Detail})
	}
	want := []row{
		{"Artists", "Ozawa", "Spotify"},
		{"Artists", "Holst", "Local"},
		{"Albums", "Cached", "Spotify · Ozawa"},
		{"Albums", "The Planets", "Local · Holst · 1990"},
		{"Tracks", "Venus", ""},
		{"Playlists", "Mine", "1 tracks"},
		{"Stations", "WBGO", ""},
		{beyondSection, "Search Spotify for “holst”", ""},
		{beyondSection, "Search the radio directory for “holst”", ""},
	}
	if !slices.Equal(got, want) {
		t.Errorf("rows =\n%v\nwant\n%v", got, want)
	}
	ids := map[string]bool{}
	for _, e := range rows[:7] {
		if e.ID == "" || ids[e.ID] {
			t.Errorf("row %q has ID %q, want a unique one", e.Title, e.ID)
		}
		ids[e.ID] = true
	}
	// The explicit rows carry the query without its operators.
	rows = searchFor(t, search, "artist:holst mars")
	if spotify := rows[len(rows)-2]; spotify.Intent != IntentSearch || spotify.Query != "holst mars" {
		t.Errorf("Spotify row = %+v", spotify)
	}
	if got := titles(load(t, rows[len(rows)-1].Open)); !slices.Equal(got, []string{"Directory holst mars"}) {
		t.Errorf("radio directory = %v", got)
	}
}

func TestSearchNoMatchesAndMore(t *testing.T) {
	cat, _, search := newSearchFixture(t)
	cat.found = nil
	if got := titles(searchFor(t, search, "zzz")); !slices.Equal(got, []string{noMatches,
		"Search Spotify for “zzz”", "Search the radio directory for “zzz”"}) {
		t.Errorf("no matches = %v", got)
	}

	var many []catalog.SearchResult
	for i := range 30 {
		many = append(many, catalog.SearchResult{Kind: catalog.SearchArtist, Provider: catalog.Local,
			Artist: &catalog.Artist{ID: int64(100 + i), Ref: lref3(catalog.Local, fmt.Sprint(i)), Name: fmt.Sprintf("Artist %d", i)}})
	}
	cat.found = catalog.SearchResults{catalog.SearchArtist: many}
	rows := searchFor(t, search, "artist")
	var artists int
	var more *Entry
	for i, e := range rows {
		if e.Section == "Artists" {
			if strings.HasPrefix(e.Title, "More") {
				more = &rows[i]
			} else {
				artists++
			}
		}
	}
	if artists != 5 || more == nil || more.Title != "More artists…" {
		t.Fatalf("artists shown = %d, more = %+v", artists, more)
	}
	if got := load(t, more.Open); len(got) != 30 {
		t.Errorf("More artists = %d rows, want all 30", len(got))
	}
	if last := cat.queries[len(cat.queries)-1]; !slices.Equal(last.Kinds, []catalog.SearchKind{catalog.SearchArtist}) {
		t.Errorf("More searched kinds %v", last.Kinds)
	}
}

func playFrom(t *testing.T, e Entry) ([]string, int) {
	t.Helper()
	if e.Track == nil || e.PlayFrom == nil {
		t.Fatalf("row %q is not a playable search track: %+v", e.Title, e)
	}
	tracks, i, err := e.PlayFrom(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, tr := range tracks {
		titles = append(titles, tr.Title)
	}
	return titles, i
}

// A searched track plays its album from that track; a station plays alone,
// as a stream.
func TestSearchTrackPlaysItsAlbum(t *testing.T) {
	_, _, search := newSearchFixture(t)
	rows := searchFor(t, search, "venus")
	var venus, wbgo Entry
	for _, e := range rows {
		switch e.Title {
		case "Venus":
			venus = e
		case "WBGO":
			wbgo = e
		}
	}
	if got, i := playFrom(t, venus); !slices.Equal(got, []string{"Mars", "Venus"}) || i != 1 {
		t.Errorf("Venus plays %v from %d, want its album from Venus", got, i)
	}
	if got, i := playFrom(t, wbgo); !slices.Equal(got, []string{"WBGO"}) || i != 0 || !wbgo.Track.Stream || !wbgo.Track.Realtime {
		t.Errorf("station plays %v from %d, stream %v", got, i, wbgo.Track.Stream)
	}
}

// fetchCatalog fetches uncached albums, or fails like an offline one.
type fetchCatalog struct {
	*fakeCatalog
	offline bool
}

func (f *fetchCatalog) FetchAlbumTracks(_ context.Context, a catalog.Album) ([]catalog.Track, error) {
	if f.offline {
		return nil, errors.New("offline")
	}
	return []catalog.Track{{ID: 60, Title: "One"}, {ID: 61, Title: "Two", AlbumID: a.ID}}, nil
}

func TestSearchTrackFetchesUncachedAlbum(t *testing.T) {
	cat, _, _ := newCatalogFixture()
	fc := &fetchCatalog{fakeCatalog: cat}
	s := &searcher{cat: fc}
	two := catalog.Track{ID: 61, Ref: sref2("t61"), Title: "Two", AlbumID: 2, PlayableURI: "spotify:track:t61"}
	e, ok := s.entry(catalog.SearchResult{Kind: catalog.SearchTrack, Provider: catalog.Spotify, Track: &two})
	if !ok {
		t.Fatal("track result dropped")
	}
	if got, i := playFrom(t, e); !slices.Equal(got, []string{"One", "Two"}) || i != 1 {
		t.Errorf("uncached album plays %v from %d, want it fetched", got, i)
	}
	fc.offline = true
	if got, i := playFrom(t, e); !slices.Equal(got, []string{"Two"}) || i != 0 {
		t.Errorf("offline plays %v from %d, want just the track", got, i)
	}
	lone := catalog.Track{ID: 70, Title: "Single", PlayableURI: "spotify:track:x"}
	e, _ = s.entry(catalog.SearchResult{Kind: catalog.SearchTrack, Provider: catalog.Spotify, Track: &lone})
	if got, _ := playFrom(t, e); !slices.Equal(got, []string{"Single"}) {
		t.Errorf("album-less track plays %v", got)
	}
}

func TestRootSearchWithoutCatalog(t *testing.T) {
	root := Root(Sources{Spotify: &fakeProvider{name: "Spotify"}})
	for _, e := range load(t, root) {
		if e.Title == "Search" && (e.Intent != IntentSearch || e.Open != nil) {
			t.Errorf("Search without a catalog = %+v, want the provider search intent", e)
		}
	}
}
