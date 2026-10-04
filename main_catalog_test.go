package main

// ddmus: tests for the catalog runtime's sync retries.

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/catalog"
	"github.com/bjarneo/cliamp/catalog/sqlite"
	"github.com/bjarneo/cliamp/catalogsync"
	"github.com/bjarneo/cliamp/config"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/ui/model"
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
	rt.setSources(source{Source: src, refresh: time.Hour, worker: rt.filler})
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

var errDown = errors.New("down")

// A sync failed by a rate limit retries when the block ends, not sooner.
func TestRetryWaitsForTheRateLimit(t *testing.T) {
	rt := testRuntime(t, &flakySource{done: make(chan struct{})})
	rt.retryMin, rt.retryMax = time.Minute, 30*time.Minute
	rl := fmt.Errorf("sync spotify/albums: %w", &catalog.RateLimitError{RetryAfter: 20 * time.Hour})
	if got := rt.scheduleRetry(catalog.Spotify, errors.Join(errDown, rl)); got != 20*time.Hour {
		t.Errorf("retry in %v, want 20h", got)
	}
	if got := rt.scheduleRetry(catalog.Spotify, &catalog.RateLimitError{RetryAfter: time.Second}); got != 2*time.Minute {
		t.Errorf("short limit: retry in %v, want the 2m backoff", got)
	}
	if got := rt.scheduleRetry(catalog.Spotify, nil); got != 0 {
		t.Errorf("after a success: retry in %v, want none", got)
	}
}

func TestRetryBacksOffAndStopsOnClose(t *testing.T) {
	rt := testRuntime(t, &flakySource{done: make(chan struct{})})
	rt.retryMin, rt.retryMax = time.Hour, 4*time.Hour
	ps := rt.providers[catalog.Spotify]
	var delays []time.Duration
	for range 4 {
		rt.scheduleRetry(catalog.Spotify, errDown)
		delays = append(delays, ps.retryDelay)
	}
	want := []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour, 4 * time.Hour}
	for i := range want {
		if delays[i] != want[i] {
			t.Fatalf("delays = %v, want %v", delays, want)
		}
	}
	rt.close()
	rt.scheduleRetry(catalog.Spotify, errDown)
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
	rt.setSources(source{Source: &namedSource{catalog.Spotify}, refresh: time.Hour},
		source{Source: &namedSource{catalog.Local}, lists: []string{catalog.CollectionAlbums}})
	if sp := rt.providers[catalog.Spotify]; sp == nil || sp.stale || sp.startup.LastSuccess.IsZero() {
		t.Errorf("fresh Spotify = %+v, want not stale", sp)
	}
	// A source's menu offers its sync's collections and the lists it adds.
	if got := rt.collections(catalog.Local); !slices.Equal(got, []string{"albums", "albums"}) {
		t.Errorf("local collections = %v, want its sync's and its extra list", got)
	}
	if lp := rt.providers[catalog.Local]; lp == nil || !lp.stale {
		t.Errorf("local = %+v, want indexed at every startup", lp)
	}
	if _, ok := rt.catalog().(catalog.AlbumTrackFetcher); ok {
		t.Error("a catalog without a filler offers album fetching")
	}
}

// A sync that failed after a recent success runs again at the next startup.
func TestSourcesStartupRetriesAFailedSync(t *testing.T) {
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.RecordSyncSuccess(ctx, catalog.Spotify, "albums"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordSyncFailure(ctx, catalog.Spotify, "albums", errors.New("offline")); err != nil {
		t.Fatal(err)
	}
	rt := &catalogRuntime{store: store, providers: map[string]*providerSync{}}
	rt.ctx, rt.cancel = context.WithCancel(ctx)
	defer rt.cancel()
	rt.setSources(source{Source: &namedSource{catalog.Spotify}, refresh: time.Hour})
	if sp := rt.providers[catalog.Spotify]; sp == nil || !sp.stale {
		t.Errorf("failed Spotify = %+v, want synced at startup", sp)
	}
}

type namedSource struct{ name string }

func (s *namedSource) Provider() string    { return s.name }
func (*namedSource) Collections() []string { return []string{"albums"} }
func (*namedSource) Fetch(context.Context, string, catalogsync.Known) (catalog.Snapshot, error) {
	return catalog.Snapshot{}, catalogsync.ErrUnchanged
}

// A quiet source (radio) syncs without reporting status to the UI.
func TestQuietSourceReportsNothing(t *testing.T) {
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	rt := &catalogRuntime{store: store, providers: map[string]*providerSync{}}
	rt.ctx, rt.cancel = context.WithCancel(context.Background())
	defer rt.cancel()
	rt.setSources(source{Source: &namedSource{catalog.Spotify}}, source{Source: &namedSource{catalog.Radio}, quiet: true})
	var sent []tea.Msg
	rt.send = func(m tea.Msg) { sent = append(sent, m) }
	for _, p := range []string{catalog.Radio, catalog.Spotify} {
		if err := rt.engine.Sync(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range sent {
		if msg, ok := m.(model.CatalogSyncMsg); !ok || msg.Provider != catalog.Spotify {
			t.Errorf("sent %+v, want only Spotify's events", m)
		}
	}
	if len(sent) == 0 {
		t.Error("Spotify's events were not sent")
	}
}

// gatedSource blocks each Fetch until released and counts the runs.
type gatedSource struct {
	namedSource
	mu      sync.Mutex
	runs    int
	started chan struct{}
	release chan struct{}
}

func (g *gatedSource) Fetch(ctx context.Context, c string, k catalogsync.Known) (catalog.Snapshot, error) {
	g.mu.Lock()
	g.runs++
	g.mu.Unlock()
	g.started <- struct{}{}
	<-g.release
	return g.namedSource.Fetch(ctx, c, k)
}

// A quiet source's sync requested while one runs (a favorite toggled
// mid-sync) is served by one more run, not dropped.
func TestQuietSyncRequestedMidRunRunsAgain(t *testing.T) {
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	src := &gatedSource{namedSource: namedSource{catalog.Radio}, started: make(chan struct{}, 4), release: make(chan struct{})}
	rt := &catalogRuntime{store: store, providers: map[string]*providerSync{}}
	rt.ctx, rt.cancel = context.WithCancel(context.Background())
	rt.setSources(source{Source: src, quiet: true})
	t.Cleanup(rt.close)

	rt.sync(catalog.Radio)
	<-src.started          // the first run is fetching
	rt.sync(catalog.Radio) // requested mid-run: finds it running
	close(src.release)
	<-src.started // the running sync ran again
	rt.wg.Wait()
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.runs != 2 {
		t.Errorf("runs = %d, want 2", src.runs)
	}
}

// lateWorker is a worker whose Hold blocks from its second call on, until
// late is closed, and which reports each Run.
type lateWorker struct {
	mu    sync.Mutex
	holds int
	late  chan struct{}
	ran   chan struct{}
}

func (w *lateWorker) Hold() func() {
	w.mu.Lock()
	w.holds++
	wait := w.holds > 1
	w.mu.Unlock()
	if wait {
		<-w.late
	}
	return func() {}
}

func (w *lateWorker) Run(context.Context) error {
	w.ran <- struct{}{}
	return nil
}

// A request made while a quiet sync runs is counted by that sync and starts
// nothing of its own. Were it to start its own, and get there only after the
// running sync had finished, both would serve it: a third run.
func TestQuietSyncMidRunRequestStartsNoSecondSync(t *testing.T) {
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	src := &gatedSource{namedSource: namedSource{catalog.Radio}, started: make(chan struct{}, 4), release: make(chan struct{})}
	w := &lateWorker{late: make(chan struct{}), ran: make(chan struct{}, 4)}
	rt := &catalogRuntime{store: store, providers: map[string]*providerSync{}}
	rt.ctx, rt.cancel = context.WithCancel(context.Background())
	rt.setSources(source{Source: src, quiet: true, worker: w})
	t.Cleanup(rt.close)

	rt.sync(catalog.Radio)
	<-src.started          // the first run is fetching
	rt.sync(catalog.Radio) // requested mid-run
	src.release <- struct{}{}
	<-src.started // the running sync ran again
	src.release <- struct{}{}
	<-w.ran // the running sync is done: it started its worker

	// Anything the mid-run request started gets to its sync only now.
	close(w.late)
	done := make(chan struct{})
	go func() { rt.wg.Wait(); close(done) }()
	select {
	case <-src.started:
		src.release <- struct{}{}
		<-done
		t.Fatal("the mid-run request ran a sync of its own: 3 runs, want 2")
	case <-done:
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.runs != 2 {
		t.Errorf("runs = %d, want 2", src.runs)
	}
}

// fakePlayer is a minimal provider for wiring tests.
type fakePlayer struct{ name string }

func (p fakePlayer) Name() string                              { return p.name }
func (fakePlayer) Playlists() ([]playlist.PlaylistInfo, error) { return nil, nil }
func (fakePlayer) Tracks(string) ([]playlist.Track, error)     { return nil, nil }

// Spotify's menu is built from what its sync covers; without a catalog it
// browses live.
func TestLibrarySourcesSyncedMenus(t *testing.T) {
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	rt := &catalogRuntime{store: store, providers: map[string]*providerSync{}}
	rt.ctx, rt.cancel = context.WithCancel(context.Background())
	defer rt.cancel()
	rt.setSources(source{Source: &namedSource{catalog.Spotify}})
	providers := []model.ProviderEntry{{Key: "spotify", Provider: fakePlayer{"Spotify"}}}

	src := librarySources(providers, "", rt)
	if len(src.Synced) != 1 || src.Synced[0].Provider != catalog.Spotify || !slices.Equal(src.Synced[0].Collections, []string{"albums"}) {
		t.Errorf("synced = %+v, want Spotify with its sync's collections", src.Synced)
	}
	if src := librarySources(providers, "", &catalogRuntime{providers: map[string]*providerSync{}}); len(src.Synced) != 0 || src.Catalog != nil {
		t.Errorf("without a catalog: synced %+v, catalog %v", src.Synced, src.Catalog)
	}
}

// The YouTube sync reads the account the way it is signed in.
func TestYouTubeClientFollowsSignIn(t *testing.T) {
	orig := ytdlpAvailable
	t.Cleanup(func() { ytdlpAvailable = orig })
	ytdlpAvailable = func() bool { return true }
	cookies := config.YouTubeMusicConfig{CookiesFrom: "brave+gnomekeyring"}
	oauth := config.YouTubeMusicConfig{ClientID: "id", ClientSecret: "secret"}
	both := config.YouTubeMusicConfig{CookiesFrom: "brave", ClientID: "id", ClientSecret: "secret"}
	disabled := both
	disabled.Disabled = true
	tests := []struct {
		name string
		yt   config.YouTubeMusicConfig
		want string
	}{
		{"cookies", cookies, "*ytmusic.CookieCatalog"},
		{"oauth", oauth, "*ytmusic.OAuthCatalog"},
		{"both", both, "youtubesrc.Mixed"},
		{"disabled", disabled, "<nil>"},
		{"not signed in", config.YouTubeMusicConfig{}, "<nil>"},
		{"half an oauth client", config.YouTubeMusicConfig{ClientID: "id"}, "<nil>"},
	}
	for _, tt := range tests {
		if got := fmt.Sprintf("%T", youtubeClient(tt.yt)); got != tt.want {
			t.Errorf("%s: client = %s, want %s", tt.name, got, tt.want)
		}
	}
	ytdlpAvailable = func() bool { return false }
	if c := youtubeClient(both); c != nil {
		t.Errorf("without yt-dlp: client = %T, want none", c)
	}
}
