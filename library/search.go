package library

import (
	"context"
	"strings"

	"github.com/bjarneo/cliamp/catalog"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/provider"
)

// SearchLevel is the search screen: its rows are the results for Query. The
// UI swaps in WithQuery's level as the user types, so a level never changes
// while it loads.
type SearchLevel interface {
	Level
	Query() string
	WithQuery(query string) SearchLevel
}

// Search sections, in the order they are shown, and how many results each
// shows before a "More…" row.
var searchSections = []struct {
	kind    catalog.SearchKind
	heading string
	limit   int
}{
	{catalog.SearchArtist, "Artists", 5},
	{catalog.SearchAlbum, "Albums", 8},
	{catalog.SearchTrack, "Tracks", 20},
	{catalog.SearchPlaylist, "Playlists", 5},
	{catalog.SearchStation, "Stations", 5},
}

const (
	// searchFetch is one more than the largest section, so a full section
	// knows it has more.
	searchFetch = 21
	// moreLimit caps a "More…" list.
	moreLimit = 200

	beyondSection = "Beyond your library"
	noMatches     = "No matches in your library"
)

// stationSearcher is the radio provider's directory search.
type stationSearcher interface {
	SearchStationTracks(query string) ([]playlist.Track, error)
}

// searcher searches the catalog and builds result rows with each source's
// own builders, so a result opens exactly as it does while browsing.
type searcher struct {
	cat     catalog.Catalog
	spotify *catalogBrowser // nil without Spotify
	local   *catalogBrowser
	// Beyond the catalog: live Spotify search and the radio directory.
	spotifyProv playlist.Provider
	radioProv   playlist.Provider
}

func newSearcher(cat catalog.Catalog, src Sources) *searcher {
	s := &searcher{cat: cat, local: localBrowser(cat, src.Local, src.MusicDir), radioProv: src.Radio}
	if src.Spotify != nil {
		s.spotify, s.spotifyProv = spotifyBrowser(cat, src.Spotify), src.Spotify
	}
	return s
}

func (s *searcher) level(query string) SearchLevel { return &searchLevel{s: s, query: query} }

type searchLevel struct {
	s     *searcher
	query string
}

func (l *searchLevel) Title() string                      { return "Search" }
func (l *searchLevel) Query() string                      { return l.query }
func (l *searchLevel) WithQuery(query string) SearchLevel { return l.s.level(query) }
func (l *searchLevel) Load(ctx context.Context) ([]Entry, error) {
	return l.s.results(ctx, l.query)
}

// results is the search screen for query: each kind's best results under
// its heading, then rows that search beyond the catalog.
func (s *searcher) results(ctx context.Context, query string) ([]Entry, error) {
	q := catalog.ParseQuery(query)
	if q.Empty() {
		return nil, nil
	}
	found, err := s.cat.Search(ctx, q, searchFetch)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for _, sec := range searchSections {
		rs := found[sec.kind]
		for _, r := range rs[:min(len(rs), sec.limit)] {
			if e, ok := s.entry(r); ok {
				e.Section = sec.heading
				entries = append(entries, e)
			}
		}
		if len(rs) > sec.limit {
			entries = append(entries, Entry{
				ID: "more:" + string(sec.kind), Section: sec.heading,
				Title: "More " + strings.ToLower(sec.heading) + "…", Open: s.more(q, sec.kind, sec.heading),
			})
		}
	}
	if len(entries) == 0 {
		entries = append(entries, Entry{Title: noMatches})
	}
	return append(entries, s.beyond(q)...), nil
}

// more lists up to moreLimit results of one kind.
func (s *searcher) more(q catalog.Query, kind catalog.SearchKind, heading string) Level {
	q.Kinds = []catalog.SearchKind{kind}
	return NewLevel(heading, func(ctx context.Context) ([]Entry, error) {
		found, err := s.cat.Search(ctx, q, moreLimit)
		if err != nil {
			return nil, err
		}
		var entries []Entry
		for _, r := range found[kind] {
			if e, ok := s.entry(r); ok {
				entries = append(entries, e)
			}
		}
		return entries, nil
	})
}

// beyond is the rows that search outside the catalog: live Spotify search
// and the radio directory, run only when chosen.
func (s *searcher) beyond(q catalog.Query) []Entry {
	text := q.Text()
	if text == "" {
		return nil
	}
	var entries []Entry
	if _, ok := s.spotifyProv.(provider.Searcher); ok {
		entries = append(entries, Entry{Section: beyondSection, Title: "Search Spotify for “" + text + "”",
			Intent: IntentSearch, Provider: s.spotifyProv, Query: text})
	}
	if st, ok := s.radioProv.(stationSearcher); ok {
		entries = append(entries, Entry{Section: beyondSection, Title: "Search the radio directory for “" + text + "”",
			Open: TrackLevel("Radio directory", s.radioProv, func(context.Context) ([]playlist.Track, error) {
				return st.SearchStationTracks(text)
			})})
	}
	return entries
}

// entry builds a result's row with its source's builder, labelled with the
// source. ok is false for a result whose source is not configured.
func (s *searcher) entry(r catalog.SearchResult) (Entry, bool) {
	var e Entry
	switch {
	case r.Artist != nil:
		switch {
		case r.Provider == catalog.Spotify && s.spotify != nil:
			e = s.spotify.artistEntry(*r.Artist)
		case r.Provider == catalog.Local:
			e = s.local.localArtistEntry(*r.Artist)
		default:
			return Entry{}, false
		}
		e.Detail = sourceLabel(r.Provider)
	case r.Album != nil:
		switch {
		case r.Provider == catalog.Spotify && s.spotify != nil:
			e = s.spotify.albumEntry(*r.Album, r.Album.Artist)
		case r.Provider == catalog.Local:
			e = s.local.localAlbumEntry(*r.Album)
		default:
			return Entry{}, false
		}
		e.Detail = joinDetail(e.Detail, sourceLabel(r.Provider))
	case r.Playlist != nil:
		if s.spotify == nil || r.Provider != catalog.Spotify {
			return Entry{}, false
		}
		e = s.spotify.playlistEntry(*r.Playlist)
	case r.Track != nil:
		t := *r.Track
		track := PlayableTrack(t)
		e = Entry{Title: t.Title, Track: &track}
		if r.Kind == catalog.SearchStation {
			track.Stream, track.Realtime = true, true
			e.PlayFrom = func(context.Context) ([]playlist.Track, int, error) {
				return []playlist.Track{track}, 0, nil
			}
		} else {
			e.PlayFrom = s.albumFrom(t)
		}
	default:
		return Entry{}, false
	}
	// Kinds share catalog IDs; the kind keeps a row's ID unique.
	if r.Track != nil {
		e.ID = catalogID(r.Track.ID)
	}
	e.ID = string(r.Kind) + ":" + e.ID
	return e, true
}

// albumFrom plays a searched track's album from that track: the catalog's
// track list, fetched and cached first for an uncached Spotify album. When
// the album cannot be had (offline, no album) it plays just the track.
func (s *searcher) albumFrom(t catalog.Track) func(ctx context.Context) ([]playlist.Track, int, error) {
	return func(ctx context.Context) ([]playlist.Track, int, error) {
		single := []playlist.Track{PlayableTrack(t)}
		if t.AlbumID == 0 {
			return single, 0, nil
		}
		tracks, cached, err := s.cat.AlbumTracks(ctx, t.AlbumID)
		if err != nil {
			return single, 0, nil
		}
		if !cached && t.Ref.Provider == catalog.Spotify {
			if f, ok := s.cat.(catalog.AlbumTrackFetcher); ok {
				if album, err := s.cat.Album(ctx, t.AlbumID); err == nil {
					if fetched, err := f.FetchAlbumTracks(ctx, album); err == nil {
						tracks = fetched
					}
				}
			}
		}
		for i, at := range tracks {
			if at.ID == t.ID {
				out := make([]playlist.Track, len(tracks))
				for j, tr := range tracks {
					out[j] = PlayableTrack(tr)
				}
				return out, i, nil
			}
		}
		return single, 0, nil
	}
}

// sourceLabel names a catalog provider for display.
func sourceLabel(provider string) string {
	switch provider {
	case catalog.Spotify:
		return "Spotify"
	case catalog.Local:
		return "Local"
	case catalog.Radio:
		return "Radio"
	}
	return provider
}

func joinDetail(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}
