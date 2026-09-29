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
	rt := &catalogRuntime{store: store, retryMin: 5 * time.Millisecond, retryMax: 10 * time.Millisecond}
	rt.ctx, rt.cancel = context.WithCancel(context.Background())
	rt.engine = catalogsync.New(store, nil, src)
	rt.filler = catalogsync.NewFiller(store, src, catalogsync.DefaultPacing)
	t.Cleanup(rt.close)
	return rt
}

func TestFailedSyncRetriesUntilItSucceeds(t *testing.T) {
	src := &flakySource{fails: 2, done: make(chan struct{})}
	rt := testRuntime(t, src)
	rt.refresh()
	select {
	case <-src.done:
	case <-time.After(5 * time.Second):
		t.Fatal("sync never succeeded after failures")
	}
	// Once the success lands, no further retry is pending and the delay resets.
	deadline := time.Now().Add(5 * time.Second)
	for {
		rt.mu.Lock()
		settled := rt.retry == nil && rt.retryDelay == 0
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
	var delays []time.Duration
	for range 4 {
		rt.scheduleRetry(true)
		delays = append(delays, rt.retryDelay)
	}
	want := []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour, 4 * time.Hour}
	for i := range want {
		if delays[i] != want[i] {
			t.Fatalf("delays = %v, want %v", delays, want)
		}
	}
	rt.close()
	rt.scheduleRetry(true)
	if rt.retry == nil {
		return
	}
	// The timer left from before close was stopped; none is armed after it.
	if rt.retry.Stop() {
		t.Error("a retry is armed after close")
	}
}
