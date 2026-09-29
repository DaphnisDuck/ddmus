package main

// omatunes: tests for the catalog runtime's sync retries.

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/catalog"
	"github.com/bjarneo/cliamp/catalog/sqlite"
	"github.com/bjarneo/cliamp/catalogsync"
)

// flakySource fails its first fails syncs, then returns an empty library.
type flakySource struct {
	mu    sync.Mutex
	fails int
	syncs int
	done  chan struct{} // closed on the first successful sync
}

func (*flakySource) Provider() string      { return catalog.Spotify }
func (*flakySource) Collections() []string { return []string{"albums"} }

func (f *flakySource) Fetch(_ context.Context, collection string, _ catalogsync.Known) (catalog.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.syncs++
	if f.syncs <= f.fails {
		return catalog.Snapshot{}, errors.New("login5: 503 no healthy upstream")
	}
	if f.syncs == f.fails+1 {
		close(f.done)
	}
	return catalog.Snapshot{Provider: catalog.Spotify, Collection: collection}, nil
}

func (*flakySource) AlbumTracks(context.Context, catalog.Ref) ([]catalog.TrackRecord, error) {
	return nil, nil
}
func (*flakySource) AlbumTracksOnce(context.Context, catalog.Ref) ([]catalog.TrackRecord, error) {
	return nil, nil
}

func testRuntime(t *testing.T, src *flakySource) *catalogRuntime {
	t.Helper()
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	rt := &catalogRuntime{store: store, providers: map[string]*providerSync{},
		retryMin: 5 * time.Millisecond, retryMax: 10 * time.Millisecond}
	rt.ctx, rt.cancel = context.WithCancel(context.Background())
	rt.filler = catalogsync.NewFiller(store, src, catalogsync.DefaultPacing)
	rt.setSources(source{Source: src, refresh: time.Hour, fill: true})
	t.Cleanup(rt.close)
	return rt
}

func TestFailedSyncRetriesUntilItSucceeds(t *testing.T) {
	src := &flakySource{fails: 2, done: make(chan struct{})}
	rt := testRuntime(t, src)
	rt.refresh(catalog.Spotify)
	select {
	case <-src.done:
	case <-time.After(5 * time.Second):
		t.Fatal("sync never succeeded after failures")
	}
	// Once the success lands, no further retry is pending and the delay resets.
	deadline := time.Now().Add(5 * time.Second)
	for {
		rt.mu.Lock()
		ps := rt.providers[catalog.Spotify]
		settled := ps.retry == nil && ps.retryDelay == 0
		rt.mu.Unlock()
		if settled {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("retry still scheduled after a successful sync")
		}
		time.Sleep(time.Millisecond)
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.syncs != 3 {
		t.Errorf("syncs = %d, want 2 failures and 1 success", src.syncs)
	}
}

func TestRetryBacksOffAndStopsOnClose(t *testing.T) {
	rt := testRuntime(t, &flakySource{done: make(chan struct{})})
	rt.retryMin, rt.retryMax = time.Hour, 4*time.Hour
	ps := rt.providers[catalog.Spotify]
	var delays []time.Duration
	for range 4 {
		rt.scheduleRetry(catalog.Spotify, true)
		delays = append(delays, ps.retryDelay)
	}
	want := []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour, 4 * time.Hour}
	for i := range want {
		if delays[i] != want[i] {
			t.Fatalf("delays = %v, want %v", delays, want)
		}
	}
	rt.close()
	rt.scheduleRetry(catalog.Spotify, true)
	if ps.retry == nil {
		return
	}
	// The timer left from before close was stopped; none is armed after it.
	if ps.retry.Stop() {
		t.Error("a retry is armed after close")
	}
}

// Every configured source gets its status and startup decision: a source
// syncs when older than its refresh, and a zero refresh syncs every time.
func TestSourcesStartupPolicy(t *testing.T) {
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, p := range []string{catalog.Spotify, catalog.Local} {
		if err := store.RecordSyncSuccess(context.Background(), p, "albums"); err != nil {
			t.Fatal(err)
		}
	}
	rt := &catalogRuntime{store: store, providers: map[string]*providerSync{}}
	rt.ctx, rt.cancel = context.WithCancel(context.Background())
	defer rt.cancel()
	rt.setSources(source{Source: &namedSource{catalog.Spotify}, refresh: time.Hour, fill: true},
		source{Source: &namedSource{catalog.Local}})
	if sp := rt.providers[catalog.Spotify]; sp == nil || sp.stale || sp.startup.LastSuccess.IsZero() {
		t.Errorf("fresh Spotify = %+v, want not stale", sp)
	}
	if sp := rt.providers[catalog.Spotify]; sp.fill {
		t.Error("fill set without a filler")
	}
	if lp := rt.providers[catalog.Local]; lp == nil || !lp.stale {
		t.Errorf("local = %+v, want indexed at every startup", lp)
	}
	if _, ok := rt.catalog().(catalog.AlbumTrackFetcher); ok {
		t.Error("a catalog without a filler offers album fetching")
	}
}

type namedSource struct{ name string }

func (s *namedSource) Provider() string    { return s.name }
func (*namedSource) Collections() []string { return []string{"albums"} }
func (*namedSource) Fetch(context.Context, string, catalogsync.Known) (catalog.Snapshot, error) {
	return catalog.Snapshot{}, catalogsync.ErrUnchanged
}
