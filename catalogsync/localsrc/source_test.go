package localsrc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/catalog"
	"github.com/bjarneo/cliamp/catalog/sqlite"
	"github.com/bjarneo/cliamp/catalogsync"
	"github.com/bjarneo/cliamp/playlist"
)

func TestSnapshotGroupsLikeTheFolderScan(t *testing.T) {
	f := func(path string, tr playlist.Track) file {
		tr.Path = path
		return file{stat: catalog.FileStat{Path: path}, track: tr}
	}
	snap := snapshot([]file{
		f("/m/Mahler/5/CD2/01.flac", playlist.Track{Title: "IV", Artist: "Ozawa", Album: "Mahler 5", Year: 1990, TrackNumber: 1}),
		f("/m/Mahler/5/CD1/02.flac", playlist.Track{Title: "II", Artist: "ozawa", Album: "Mahler 5", TrackNumber: 2}),
		f("/m/Mahler/5/CD1/01.flac", playlist.Track{Title: "I", Artist: "Ozawa", Album: "mahler 5", TrackNumber: 1}),
		f("/m/Mix/01.mp3", playlist.Track{Title: "A", Artist: "Ravel", Album: "Mix"}),
		f("/m/Mix/02.mp3", playlist.Track{Title: "B", Artist: "Respighi", Album: "Mix"}),
		f("/m/Untagged/x.mp3", playlist.Track{}),
	})

	type albumRow struct {
		title, credit string
		year, count   int
	}
	var albums []albumRow
	for _, a := range snap.Albums {
		albums = append(albums, albumRow{a.Title, a.Credit, a.Year, a.TrackCount})
	}
	want := []albumRow{
		{"Mahler 5", "Ozawa", 1990, 3},
		{"Mix", variousArtists, 0, 2},
		{"Untagged", unknownArtist, 0, 1},
	}
	if !slices.Equal(albums, want) {
		t.Errorf("albums = %+v, want %+v", albums, want)
	}
	if mix := snap.Albums[1]; len(mix.Artists) != 2 || mix.Artists[0].Name != "Ravel" || mix.Artists[1].Name != "Respighi" {
		t.Errorf("compilation credits = %+v, want each track artist", mix.Artists)
	}

	type trackRow struct {
		path  string
		disc  int
		album string
	}
	var tracks []trackRow
	for _, tr := range snap.Tracks {
		tracks = append(tracks, trackRow{tr.PlayableURI, tr.Disc, tr.Album.Title})
		if tr.Ref.ProviderID != tr.PlayableURI || tr.File == nil || tr.File.Path != tr.PlayableURI {
			t.Errorf("track %s: ref %v, file %+v; want the path everywhere", tr.PlayableURI, tr.Ref, tr.File)
		}
	}
	wantTracks := []trackRow{
		{"/m/Mahler/5/CD1/01.flac", 1, "Mahler 5"},
		{"/m/Mahler/5/CD1/02.flac", 1, "Mahler 5"},
		{"/m/Mahler/5/CD2/01.flac", 2, "Mahler 5"},
		{"/m/Mix/01.mp3", 1, "Mix"},
		{"/m/Mix/02.mp3", 1, "Mix"},
		{"/m/Untagged/x.mp3", 1, "Untagged"},
	}
	if !slices.Equal(tracks, wantTracks) {
		t.Errorf("tracks = %+v, want %+v", tracks, wantTracks)
	}
	if got := snap.Tracks[5].Title; got != "x" {
		t.Errorf("untitled track = %q, want the file name", got)
	}

	var artists []string
	for _, a := range snap.Artists {
		artists = append(artists, a.Name)
	}
	// "ozawa" is the same artist as "Ozawa"; the first spelling in album
	// order wins.
	if !slices.Equal(artists, []string{"Ozawa", "Ravel", "Respighi", unknownArtist}) {
		t.Errorf("artists = %v", artists)
	}
}

// library is a temp music folder indexed into a temp catalog, with tags
// served from a map so tests can edit them.
type library struct {
	t     *testing.T
	dir   string
	store *sqlite.Store
	src   *Source
	eng   *catalogsync.Engine

	mu    sync.Mutex
	tags  map[string]playlist.Track // by path relative to dir
	reads []string
}

func newLibrary(t *testing.T) *library {
	t.Helper()
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	l := &library{t: t, dir: t.TempDir(), store: store, tags: map[string]playlist.Track{}}
	l.src = New(l.dir, store)
	l.src.readTags = func(paths []string) []playlist.Track {
		l.mu.Lock()
		defer l.mu.Unlock()
		out := make([]playlist.Track, len(paths))
		for i, p := range paths {
			rel, _ := filepath.Rel(l.dir, p)
			l.reads = append(l.reads, rel)
			out[i] = l.tags[rel]
		}
		return out
	}
	l.eng = catalogsync.New(store, nil, l.src)
	return l
}

// write creates or rewrites a file with tags; content sets its size.
func (l *library) write(rel, content string, tags playlist.Track) {
	l.t.Helper()
	path := filepath.Join(l.dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		l.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		l.t.Fatal(err)
	}
	l.mu.Lock()
	l.tags[rel] = tags
	l.mu.Unlock()
}

func (l *library) sync() error {
	l.mu.Lock()
	l.reads = nil
	l.mu.Unlock()
	return l.eng.Sync(context.Background(), catalog.Local)
}

func (l *library) readFiles() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Sorted(slices.Values(l.reads))
}

// albums lists album title → track titles, as the library shows them.
func (l *library) albums() map[string][]string {
	l.t.Helper()
	ctx := context.Background()
	albums, err := l.store.Albums(ctx, catalog.Local)
	if err != nil {
		l.t.Fatal(err)
	}
	out := map[string][]string{}
	for _, a := range albums {
		tracks, _, err := l.store.AlbumTracks(ctx, a.ID)
		if err != nil {
			l.t.Fatal(err)
		}
		out[a.Title] = []string{}
		for _, tr := range tracks {
			out[a.Title] = append(out[a.Title], tr.Title)
		}
	}
	return out
}

func (l *library) indexedPaths() []string {
	l.t.Helper()
	files, err := l.store.IndexedFiles(context.Background())
	if err != nil {
		l.t.Fatal(err)
	}
	var out []string
	for _, f := range files {
		rel, _ := filepath.Rel(l.dir, f.Path)
		out = append(out, rel)
	}
	slices.Sort(out)
	return out
}

func equalAlbums(got, want map[string][]string) bool {
	if len(got) != len(want) {
		return false
	}
	for k, v := range want {
		if !slices.Equal(got[k], v) {
			return false
		}
	}
	return true
}

func TestIndexIsIncremental(t *testing.T) {
	l := newLibrary(t)
	l.write("Holst/Planets/01.flac", "mars", playlist.Track{Title: "Mars", Artist: "Holst", Album: "The Planets", TrackNumber: 1, Genre: "Classical"})
	l.write("Holst/Planets/02.flac", "venus", playlist.Track{Title: "Venus", Artist: "Holst", Album: "The Planets", TrackNumber: 2})
	l.write("Ravel/Bolero.mp3", "bolero", playlist.Track{Title: "Boléro", Artist: "Ravel", Album: "Boléro", Genre: "classical"})
	l.write("notes.txt", "not audio", playlist.Track{})

	if err := l.sync(); err != nil {
		t.Fatal(err)
	}
	if got := l.readFiles(); !slices.Equal(got, []string{"Holst/Planets/01.flac", "Holst/Planets/02.flac", "Ravel/Bolero.mp3"}) {
		t.Errorf("first index read %v, want every audio file", got)
	}
	want := map[string][]string{"The Planets": {"Mars", "Venus"}, "Boléro": {"Boléro"}}
	if got := l.albums(); !equalAlbums(got, want) {
		t.Fatalf("albums = %v, want %v", got, want)
	}

	// Nothing changed: no tags read, nothing written, and the sync succeeds.
	before, _ := l.store.SyncStatus(context.Background(), catalog.Local)
	time.Sleep(2 * time.Millisecond)
	if err := l.sync(); err != nil {
		t.Fatal(err)
	}
	if got := l.readFiles(); len(got) != 0 {
		t.Errorf("unchanged index read %v", got)
	}
	after, _ := l.store.SyncStatus(context.Background(), catalog.Local)
	if len(after) != 1 || !after[0].LastSuccess.After(before[0].LastSuccess) {
		t.Errorf("unchanged index status = %+v, want a newer success", after)
	}

	// An edited file (new size) and a touched one (new mtime) are reread,
	// and only they.
	l.write("Holst/Planets/02.flac", "venus, remastered", playlist.Track{Title: "Venus (2024)", Artist: "Holst", Album: "The Planets", TrackNumber: 2})
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(l.dir, "Ravel/Bolero.mp3"), later, later); err != nil {
		t.Fatal(err)
	}
	if err := l.sync(); err != nil {
		t.Fatal(err)
	}
	if got := l.readFiles(); !slices.Equal(got, []string{"Holst/Planets/02.flac", "Ravel/Bolero.mp3"}) {
		t.Errorf("changed index read %v, want the two changed files", got)
	}
	want["The Planets"] = []string{"Mars", "Venus (2024)"}
	if got := l.albums(); !equalAlbums(got, want) {
		t.Errorf("albums after edit = %v, want %v", got, want)
	}

	// A deleted file leaves its album, and an emptied album leaves the
	// library.
	if err := os.Remove(filepath.Join(l.dir, "Holst/Planets/01.flac")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(l.dir, "Ravel/Bolero.mp3")); err != nil {
		t.Fatal(err)
	}
	if err := l.sync(); err != nil {
		t.Fatal(err)
	}
	if got := l.readFiles(); len(got) != 0 {
		t.Errorf("deletions read %v, want nothing", got)
	}
	want = map[string][]string{"The Planets": {"Venus (2024)"}}
	if got := l.albums(); !equalAlbums(got, want) {
		t.Errorf("albums after delete = %v, want %v", got, want)
	}
	if got := l.indexedPaths(); !slices.Equal(got, []string{"Holst/Planets/02.flac"}) {
		t.Errorf("file index = %v", got)
	}
	artists, _ := l.store.Artists(context.Background(), catalog.Local)
	if len(artists) != 1 || artists[0].Name != "Holst" {
		t.Errorf("artists after delete = %+v, want only Holst", artists)
	}
}

func TestIndexGenres(t *testing.T) {
	l := newLibrary(t)
	l.write("a/1.mp3", "1", playlist.Track{Title: "1", Artist: "A", Album: "One", Genre: "Jazz"})
	l.write("a/2.mp3", "2", playlist.Track{Title: "2", Artist: "A", Album: "One", Genre: "Rock"})
	l.write("b/1.mp3", "3", playlist.Track{Title: "3", Artist: "B", Album: "Two", Genre: "jazz"})
	if err := l.sync(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	genres, err := l.store.Genres(ctx, catalog.Local)
	if err != nil {
		t.Fatal(err)
	}
	if len(genres) != 2 || genres[0].AlbumCount != 2 || genres[1] != (catalog.Genre{Name: "Rock", AlbumCount: 1}) {
		t.Fatalf("genres = %+v, want Jazz (2, either case) and Rock (1)", genres)
	}
	albums, err := l.store.GenreAlbums(ctx, catalog.Local, "JAZZ")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 2 || albums[0].Title != "One" || albums[1].Title != "Two" {
		t.Errorf("jazz albums = %+v", albums)
	}
}

// A folder that vanished or emptied (an unmounted drive) must not erase the
// indexed library.
func TestIndexKeepsLibraryWhenFolderIsGone(t *testing.T) {
	l := newLibrary(t)
	l.write("a/1.mp3", "1", playlist.Track{Title: "1", Artist: "A", Album: "One"})
	if err := l.sync(); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"One": {"1"}}

	if err := os.RemoveAll(filepath.Join(l.dir, "a")); err != nil {
		t.Fatal(err)
	}
	if err := l.sync(); err == nil {
		t.Error("indexing an empty folder over a full index succeeded")
	}
	if got := l.albums(); !equalAlbums(got, want) {
		t.Errorf("albums after an empty folder = %v, want %v", got, want)
	}

	if err := os.RemoveAll(l.dir); err != nil {
		t.Fatal(err)
	}
	if err := l.sync(); err == nil {
		t.Error("indexing a missing folder succeeded")
	}
	if got := l.albums(); !equalAlbums(got, want) {
		t.Errorf("albums after a missing folder = %v, want %v", got, want)
	}
	status, _ := l.store.SyncStatus(context.Background(), catalog.Local)
	if len(status) != 1 || status[0].LastError == "" {
		t.Errorf("status = %+v, want the failure recorded", status)
	}
}

func TestIndexKeepsFilesUnderUnreadableFolders(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every folder")
	}
	l := newLibrary(t)
	l.write("open/1.mp3", "1", playlist.Track{Title: "1", Artist: "A", Album: "Open"})
	l.write("locked/2.mp3", "2", playlist.Track{Title: "2", Artist: "B", Album: "Locked"})
	if err := l.sync(); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(l.dir, "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	l.write("open/3.mp3", "3", playlist.Track{Title: "3", Artist: "A", Album: "Open"})
	if err := l.sync(); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"Open": {"1", "3"}, "Locked": {"2"}}
	if got := l.albums(); !equalAlbums(got, want) {
		t.Errorf("albums = %v, want the locked folder's album kept", got)
	}
}

func TestIndexCancelled(t *testing.T) {
	l := newLibrary(t)
	l.write("a/1.mp3", "1", playlist.Track{Title: "1"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.eng.Sync(ctx, catalog.Local); !errors.Is(err, context.Canceled) {
		t.Errorf("Sync() = %v, want context.Canceled", err)
	}
	if got := l.albums(); len(got) != 0 {
		t.Errorf("a cancelled index wrote %v", got)
	}
}
