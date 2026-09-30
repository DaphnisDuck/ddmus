package catalogsync

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/catalog"
	"github.com/bjarneo/cliamp/catalog/sqlite"
)

// fakeMeta answers metadata reads; errs scripts each video's errors in
// order before it answers.
type fakeMeta struct {
	mu    sync.Mutex
	errs  map[string][]error
	reads []string
}

func (*fakeMeta) Provider() string { return catalog.YouTube }

func (f *fakeMeta) TrackMetadata(_ context.Context, t catalog.Ref) (catalog.TrackMetadata, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, t.ProviderID)
	if errs := f.errs[t.ProviderID]; len(errs) > 0 {
		err := errs[0]
		f.errs[t.ProviderID] = errs[1:]
		return catalog.TrackMetadata{}, err
	}
	artist := []catalog.ArtistRecord{{Ref: catalog.Ref{Provider: catalog.YouTube, ProviderID: "artist:a"}, Name: "Artist A"}}
	return catalog.TrackMetadata{Title: "Song " + t.ProviderID, Artists: artist,
		Album: &catalog.AlbumRecord{Ref: catalog.Ref{Provider: catalog.YouTube, ProviderID: "album:a"}, Title: "Album A", Artists: artist}}, nil
}

func enricherSetup(t *testing.T, videos ...string) (*sqlite.Store, *fakeMeta, *Enricher, *[]time.Duration, *int) {
	t.Helper()
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	snap := catalog.Snapshot{Provider: catalog.YouTube, Collection: "liked"}
	for _, v := range videos {
		snap.Tracks = append(snap.Tracks, catalog.TrackRecord{Ref: catalog.Ref{Provider: catalog.YouTube, ProviderID: v},
			Title: "video " + v, PlayableURI: "https://music.youtube.com/watch?v=" + v})
	}
	if err := store.ApplySnapshot(context.Background(), snap); err != nil {
		t.Fatal(err)
	}
	src := &fakeMeta{errs: map[string][]error{}}
	changes := 0
	e := NewEnricher(store, src, testPacing, func() { changes++ })
	waits := &[]time.Duration{}
	e.sleep = func(ctx context.Context, d time.Duration) error { *waits = append(*waits, d); return ctx.Err() }
	return store, src, e, waits, &changes
}

func TestEnricherFillsTheLibrary(t *testing.T) {
	store, src, e, waits, changes := enricherSetup(t, "v1", "v2", "v3")
	src.errs["v2"] = []error{&catalog.RateLimitError{}, catalog.ErrForbidden}
	if err := e.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	liked, _ := store.LikedTracks(ctx, catalog.YouTube)
	got := map[string]string{}
	for _, tr := range liked {
		got[tr.Ref.ProviderID] = tr.Title + "|" + tr.AlbumTitle
	}
	if got["v1"] != "Song v1|Album A" || got["v3"] != "Song v3|Album A" || got["v2"] != "video v2|" {
		t.Errorf("tracks = %v; want v1 and v3 enriched, unreadable v2 as it was", got)
	}
	if left, _ := store.UnenrichedTracks(ctx, catalog.YouTube, 10); len(left) != 0 {
		t.Errorf("unenriched = %d, want every track read once", len(left))
	}
	albums, _ := store.Albums(ctx, catalog.YouTube, catalog.ByTitle)
	if len(albums) != 1 || albums[0].Title != "Album A" || *changes < 2 {
		t.Errorf("albums %+v after %d refreshes", albums, *changes)
	}
	// The rate limit waited the first backoff, then v2 was read again.
	if (*waits)[0] != testPacing.Delay || (*waits)[1] != testPacing.MinBackoff {
		t.Errorf("waits = %v", *waits)
	}
	// Nothing left: a second run reads nothing.
	before := len(src.reads)
	if err := e.Run(ctx); err != nil || len(src.reads) != before {
		t.Errorf("second run: %v, %d new reads", err, len(src.reads)-before)
	}
}

// A batch that finds nothing refreshes nothing beyond the run's first
// refresh.
func TestEnricherSkipsEmptyRefresh(t *testing.T) {
	_, src, e, _, changes := enricherSetup(t, "v1", "v2")
	src.errs["v1"] = []error{catalog.ErrForbidden}
	src.errs["v2"] = []error{catalog.ErrForbidden}
	if err := e.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if *changes != 1 {
		t.Errorf("refreshed %d times, want only at the start", *changes)
	}
}

func TestEnricherGivesUp(t *testing.T) {
	_, src, e, _, _ := enricherSetup(t, "v1")
	down := errors.New("dial tcp: no route to host")
	src.errs["v1"] = []error{down, down, down}
	if err := e.Run(context.Background()); !errors.Is(err, down) {
		t.Errorf("Run() = %v, want the network error after %d tries", err, testPacing.MaxFailures)
	}
}
