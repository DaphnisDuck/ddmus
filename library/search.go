package library

import (
	"context"
	"strings"
	"sync/atomic"

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

// searchFetch is one more than the largest section, so a full section
// knows it has more.
var searchFetch = func() int {
	n := 0
	for _, sec := range searchSections {
		n = max(n, sec.limit)
	}
	return n + 1
}()

const (
	// moreLimit caps a "More…" list.
	moreLimit = 200

	beyondSection = "Beyond your library"
	noMatches     = "No matches in your library"
)

// stationSearcher is the radio provider's directory search.
type stationSearcher interface {
	SearchStationTracks(query string) ([]playlist.Track, error)
}

// catalogView reads the catalog across sources, for search and the Library
// menu, and builds rows with each source's own builders, so a row opens
// exactly as it does while browsing that source.
type catalogView struct {
	cat    catalog.Catalog
	synced map[string]*catalogBrowser // by catalog provider name
	local  *catalogBrowser
	// Beyond the catalog: live Spotify search and the radio directory.
	spotifyProv playlist.Provider
	radioProv   playlist.Provider
	// albumOrder is the Library Albums list's order, a catalog.AlbumOrder.
	albumOrder atomic.Int32
}

func newCatalogView(cat catalog.Catalog, src Sources) *catalogView {
	s := &catalogView{cat: cat, synced: map[string]*catalogBrowser{},
		local: localBrowser(cat, src.Local, src.MusicDir), spotifyProv: src.Spotify, radioProv: src.Radio}
	for _, ss := range src.Synced {
		s.synced[ss.Provider] = syncedBrowser(cat, ss)
	}
	return s
}

func (s *catalogView) level(query string) SearchLevel { return &searchLevel{s: s, query: query} }

type searchLevel struct {
	s     *catalogView
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
func (s *catalogView) results(ctx context.Context, query string) ([]Entry, error) {
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
		for _, e := range s.rows(rs[:min(len(rs), sec.limit)]) {
			e.Section = sec.heading
			entries = append(entries, e)
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
func (s *catalogView) more(q catalog.Query, kind catalog.SearchKind, heading string) Level {
	q.Kinds = []catalog.SearchKind{kind}
	return NewLevel(heading, func(ctx context.Context) ([]Entry, error) {
		found, err := s.cat.Search(ctx, q, moreLimit)
		if err != nil {
			return nil, err
		}
		return s.rows(found[kind]), nil
	})
}

// rows builds the results' rows, skipping those of unconfigured sources.
func (s *catalogView) rows(results []catalog.SearchResult) []Entry {
	var entries []Entry
	for _, r := range results {
		if e, ok := s.entry(r); ok {
			entries = append(entries, e)
		}
	}
	return entries
}

// beyond is the rows that search outside the catalog: live Spotify search
// and the radio directory, run only when chosen.
func (s *catalogView) beyond(q catalog.Query) []Entry {
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
func (s *catalogView) entry(r catalog.SearchResult) (e Entry, ok bool) {
	switch {
	case r.Artist != nil:
		e, ok = s.artistRow(*r.Artist)
	case r.Album != nil:
		e, ok = s.albumRow(*r.Album)
	case r.Playlist != nil && s.synced[r.Playlist.Ref.Provider] != nil:
		e, ok = s.synced[r.Playlist.Ref.Provider].playlistEntry(*r.Playlist), true
		e.Detail = joinDetail(SourceLabel(r.Playlist.Ref.Provider), e.Detail)
	case r.Track != nil:
		e, ok = s.trackRow(*r.Track, r.Kind == catalog.SearchStation), true
	}
	// Kinds share catalog IDs; the kind keeps a row's ID unique.
	e.ID = string(r.Kind) + ":" + e.ID
	return e, ok
}

// trackRow is a searched track's row: Enter plays its album from it, or
// plays a station alone as a live stream.
func (s *catalogView) trackRow(t catalog.Track, station bool) Entry {
	track := PlayableTrack(t)
	e := Entry{ID: catalogID(t.ID), Title: t.Title, Track: &track, PlayFrom: s.albumFrom(t)}
	if station {
		track.Stream, track.Realtime = true, true
		e.PlayFrom = func(context.Context) ([]playlist.Track, int, error) {
			return []playlist.Track{track}, 0, nil
		}
	}
	return e
}

// artistRow is an artist's row, built by its source and labelled with it.
// ok is false when the source is not configured.
func (s *catalogView) artistRow(a catalog.Artist) (Entry, bool) {
	var e Entry
	switch b := s.synced[a.Ref.Provider]; {
	case b != nil:
		e = b.artistEntry(a)
	case a.Ref.Provider == catalog.Local:
		e = s.local.localArtistEntry(a)
	default:
		return Entry{}, false
	}
	e.Detail = SourceLabel(a.Ref.Provider)
	return e, true
}

// albumRow is an album's row, built by its source and labelled with it.
// ok is false when the source is not configured.
func (s *catalogView) albumRow(a catalog.Album) (Entry, bool) {
	var e Entry
	switch b := s.synced[a.Ref.Provider]; {
	case b != nil:
		e = b.albumEntry(a, a.Artist)
	case a.Ref.Provider == catalog.Local:
		e = s.local.localAlbumEntry(a)
	default:
		return Entry{}, false
	}
	// The source comes first so a long credit cannot truncate it away.
	e.Detail = joinDetail(SourceLabel(a.Ref.Provider), e.Detail)
	return e, true
}

// albumFrom plays a searched track's album from that track: the catalog's
// track list, fetched and cached first for an uncached Spotify album. When
// the album cannot be had (offline, no album) it plays just the track.
func (s *catalogView) albumFrom(t catalog.Track) func(ctx context.Context) ([]playlist.Track, int, error) {
	return func(ctx context.Context) ([]playlist.Track, int, error) {
		single := []playlist.Track{PlayableTrack(t)}
		if t.AlbumID == 0 {
			return single, 0, nil
		}
		tracks, cached, err := s.cat.AlbumTracks(ctx, t.AlbumID)
		if err != nil {
			return single, 0, nil
		}
		// An uncached Spotify album holds only the tracks the catalog met
		// elsewhere (liked, in a playlist); playing those as the album would
		// silently skip the rest. A local album is always whole.
		if !cached && t.Ref.Provider != catalog.Local {
			tracks = s.fetchAlbum(ctx, t.AlbumID)
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

// fetchAlbum fetches and caches an album's tracks, or returns none when it
// cannot (offline, or a catalog without a fetcher).
func (s *catalogView) fetchAlbum(ctx context.Context, albumID int64) []catalog.Track {
	f, ok := s.cat.(catalog.AlbumTrackFetcher)
	if !ok {
		return nil
	}
	album, err := s.cat.Album(ctx, albumID)
	if err != nil {
		return nil
	}
	tracks, err := f.FetchAlbumTracks(ctx, album)
	if err != nil {
		return nil
	}
	return tracks
}

// sourceLabels spells the providers whose names do not simply capitalize.
var sourceLabels = map[string]string{"youtube": "YouTube"}

// SourceLabel names a catalog provider for display: "spotify" → "Spotify",
// "youtube" → "YouTube".
func SourceLabel(provider string) string {
	if label, ok := sourceLabels[provider]; ok {
		return label
	}
	if provider == "" {
		return ""
	}
	return strings.ToUpper(provider[:1]) + provider[1:]
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
