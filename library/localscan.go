package library

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/resolve"
)

// Album is a local album grouped from file tags.
type Album struct {
	Title  string
	Artist string // the shared track artist, or "Various Artists"
	Year   int
	Tracks []playlist.Track
}

// Group is a named set of albums: an artist or a genre.
type Group struct {
	Name   string
	Albums []*Album
}

// Index is the in-memory local library. Milestone 2 replaces it with the
// catalog; keep its shape close to catalog rows.
type Index struct {
	Albums  []*Album
	Artists []*Group
	Genres  []*Group
}

// Scanner builds the Index from a music directory once per session.
type Scanner struct {
	dir  string
	scan func(dir string) ([]playlist.Track, error)

	mu      sync.Mutex
	idx     *Index
	pending *scanRun // the scan in flight, if any
}

// scanRun is one background scan. done closes once idx or err is set.
type scanRun struct {
	done chan struct{}
	idx  *Index
	err  error
}

// NewScanner returns a Scanner for dir that reads tags with the existing
// resolve/playlist tag reader.
func NewScanner(dir string) *Scanner {
	return &Scanner{dir: dir, scan: scanDir}
}

func scanDir(dir string) ([]playlist.Track, error) {
	files, err := resolve.AudioFiles(dir, true)
	if err != nil {
		return nil, fmt.Errorf("scan music directory: %w", err)
	}
	return resolve.TracksFromPaths(files), nil
}

// Dir is the scanned directory.
func (s *Scanner) Dir() string { return s.dir }

// Index returns the library index, scanning on first use. The scan runs once
// in the background: a caller that gives up (Back, timeout) returns at once
// while the scan finishes and is cached, so the next visit is instant. A
// failed scan is not cached, so opening the level again retries.
func (s *Scanner) Index(ctx context.Context) (*Index, error) {
	s.mu.Lock()
	if s.idx != nil {
		s.mu.Unlock()
		return s.idx, nil
	}
	run := s.pending
	if run == nil {
		run = &scanRun{done: make(chan struct{})}
		s.pending = run
		go s.run(run)
	}
	s.mu.Unlock()

	select {
	case <-run.done:
		return run.idx, run.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Scanner) run(run *scanRun) {
	tracks, err := s.scan(s.dir)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		run.err = err
	} else {
		run.idx = BuildIndex(tracks)
		s.idx = run.idx
	}
	s.pending = nil
	close(run.done)
}

const (
	unknownArtist  = "Unknown Artist"
	variousArtists = "Various Artists"
)

// discDir matches per-disc subfolders ("CD1", "Disc 2") so a multi-disc
// album stays one album.
var discDir = regexp.MustCompile(`(?i)^(cd|disc|disk)\s*\d+$`)

// albumDir is the folder that identifies an album on disk.
func albumDir(path string) string {
	dir := filepath.Dir(path)
	if discDir.MatchString(filepath.Base(dir)) {
		return filepath.Dir(dir)
	}
	return dir
}

// BuildIndex groups tracks into albums (by album tag within an album folder),
// artists and genres. Tracks without an album tag are grouped by folder.
func BuildIndex(tracks []playlist.Track) *Index {
	type albumKey struct{ title, dir string }
	albums := map[albumKey]*Album{}
	var order []*Album
	for _, t := range tracks {
		dir := albumDir(t.Path)
		title := t.Album
		if title == "" {
			title = filepath.Base(dir)
		}
		key := albumKey{strings.ToLower(title), dir}
		a, ok := albums[key]
		if !ok {
			a = &Album{Title: title}
			albums[key] = a
			order = append(order, a)
		}
		a.Tracks = append(a.Tracks, t)
	}

	idx := &Index{Albums: order}
	artists := map[string]*Group{}
	genres := map[string]*Group{}
	for _, a := range order {
		finishAlbum(a)
		seenArtist := map[string]bool{}
		seenGenre := map[string]bool{}
		for _, t := range a.Tracks {
			artist := cmp.Or(t.Artist, unknownArtist)
			if !seenArtist[strings.ToLower(artist)] {
				seenArtist[strings.ToLower(artist)] = true
				addToGroup(artists, &idx.Artists, artist, a)
			}
			if t.Genre != "" && !seenGenre[strings.ToLower(t.Genre)] {
				seenGenre[strings.ToLower(t.Genre)] = true
				addToGroup(genres, &idx.Genres, t.Genre, a)
			}
		}
	}

	slices.SortStableFunc(idx.Albums, compareAlbums)
	for _, groups := range [][]*Group{idx.Artists, idx.Genres} {
		slices.SortStableFunc(groups, func(x, y *Group) int {
			return cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
		})
		for _, g := range groups {
			slices.SortStableFunc(g.Albums, compareAlbums)
		}
	}
	return idx
}

// finishAlbum orders an album's tracks and derives its artist and year.
func finishAlbum(a *Album) {
	slices.SortStableFunc(a.Tracks, func(x, y playlist.Track) int {
		return cmp.Or(
			cmp.Compare(filepath.Dir(x.Path), filepath.Dir(y.Path)), // disc folders
			cmp.Compare(x.TrackNumber, y.TrackNumber),
			cmp.Compare(x.Path, y.Path),
		)
	})
	a.Artist = cmp.Or(a.Tracks[0].Artist, unknownArtist)
	for _, t := range a.Tracks {
		if !strings.EqualFold(cmp.Or(t.Artist, unknownArtist), a.Artist) {
			a.Artist = variousArtists
		}
		a.Year = max(a.Year, t.Year)
	}
}

func addToGroup(index map[string]*Group, list *[]*Group, name string, a *Album) {
	key := strings.ToLower(name)
	g, ok := index[key]
	if !ok {
		g = &Group{Name: name}
		index[key] = g
		*list = append(*list, g)
	}
	g.Albums = append(g.Albums, a)
}

func compareAlbums(x, y *Album) int {
	return cmp.Or(
		cmp.Compare(strings.ToLower(x.Title), strings.ToLower(y.Title)),
		cmp.Compare(strings.ToLower(x.Artist), strings.ToLower(y.Artist)),
	)
}
