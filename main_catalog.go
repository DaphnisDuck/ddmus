package main

// omatunes: the catalog runtime. It opens the SQLite catalog, runs the
// background Spotify sync and album-track fill and the local folder index,
// and reports them to the library UI. Kept out of main.go so upstream merges
// there stay conflict-free.

import (
	"context"
	"errors"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/applog"
	"github.com/bjarneo/cliamp/catalog"
	"github.com/bjarneo/cliamp/catalog/sqlite"
	"github.com/bjarneo/cliamp/catalogsync"
	"github.com/bjarneo/cliamp/catalogsync/localsrc"
	"github.com/bjarneo/cliamp/catalogsync/radiosrc"
	"github.com/bjarneo/cliamp/catalogsync/spotifysrc"
	"github.com/bjarneo/cliamp/config"
	"github.com/bjarneo/cliamp/external/radio"
	"github.com/bjarneo/cliamp/external/spotify"
	"github.com/bjarneo/cliamp/internal/appdir"
	"github.com/bjarneo/cliamp/ui/model"
)

const (
	// catalogOpenTimeout bounds opening and migrating the catalog at startup.
	catalogOpenTimeout = 10 * time.Second
	// catalogStopTimeout bounds how long quitting waits for a sync to stop.
	catalogStopTimeout = 5 * time.Second
	// A failed sync retries after syncRetryMin, doubling up to syncRetryMax,
	// so an outage clears without a restart or r.
	syncRetryMin = time.Minute
	syncRetryMax = 30 * time.Minute
)

// catalogRuntime owns the catalog for one run: the Spotify sync and album
// fill, and the local folder index. Without a store, browsing stays live, as
// in v0.1.
type catalogRuntime struct {
	store  *sqlite.Store
	engine *catalogsync.Engine
	filler *catalogsync.Filler // nil without Spotify
	ctx    context.Context
	cancel context.CancelFunc

	// providers holds each synced provider's state; its keys are fixed by
	// openCatalog, and retry fields are guarded by mu.
	providers map[string]*providerSync

	mu       sync.Mutex
	wg       sync.WaitGroup
	closed   bool
	send     func(tea.Msg) // set by start, before any sync runs
	retryMin time.Duration
	retryMax time.Duration
}

// source is a sync source and how the runtime schedules it.
type source struct {
	catalogsync.Source
	// refresh is how old the last successful sync may get before startup
	// syncs again; 0 syncs at every startup.
	refresh time.Duration
	// fill runs the album-track filler after each sync.
	fill bool
	// quiet sources (radio: local files, instant) report no sync status
	// to the UI.
	quiet bool
}

// providerSync is one provider's sync state.
type providerSync struct {
	fill        bool                // from source.fill
	quiet       bool                // from source.quiet
	collections []string            // what a complete sync covers
	startup     model.CatalogStatus // stored status, read once at startup
	stale       bool                // startup should sync
	retry       *time.Timer         // pending retry of a failed sync
	retryDelay  time.Duration       // the last retry's delay; zero after a success
	// requests counts sync requests. A quiet source's running sync that
	// sees it grow runs once more, serving requests made while it ran.
	requests int
}

// openCatalog opens the catalog with a sync source for Spotify, when it is
// configured, for the music folder, when there is one, and for your radio
// stations. A catalog that cannot open is logged and left out.
func openCatalog(sp *spotify.SpotifyProvider, rp *radio.Provider, cfg config.Config) *catalogRuntime {
	rt := &catalogRuntime{providers: map[string]*providerSync{}, retryMin: syncRetryMin, retryMax: syncRetryMax}
	rt.ctx, rt.cancel = context.WithCancel(context.Background())
	path, err := appdir.LibraryDBPath()
	if err == nil {
		ctx, cancel := context.WithTimeout(rt.ctx, catalogOpenTimeout)
		rt.store, err = sqlite.Open(ctx, path)
		cancel()
	}
	if err != nil {
		applog.Warn("catalog unavailable, browsing live: %v", err)
		return rt
	}
	var sources []source
	if sp != nil {
		src := spotifysrc.New(sp)
		sources = append(sources, source{Source: src, refresh: cfg.Omatunes.SpotifyRefresh, fill: true})
		rt.filler = catalogsync.NewFiller(rt.store, src, catalogsync.DefaultPacing)
	}
	if dir := musicDir(cfg.InitialDirectory); dir != "" {
		// Indexed at every startup: it rereads only changed files and
		// writes nothing when none changed.
		sources = append(sources, source{Source: localsrc.New(dir, rt.store)})
	}
	if rp != nil {
		// Favorites and radios.toml, read from local files at every startup.
		sources = append(sources, source{Source: radiosrc.New(rp), quiet: true})
	}
	rt.setSources(sources...)
	if rp != nil && rt.engine != nil {
		rp.OnFavoritesToggled(func() { rt.sync(catalog.Radio) })
	}
	return rt
}

// setSources creates the engine for sources and reads their stored status.
// A source syncs at startup when incomplete or older than its refresh.
func (rt *catalogRuntime) setSources(sources ...source) {
	if len(sources) == 0 || rt.store == nil {
		return
	}
	engineSources := make([]catalogsync.Source, len(sources))
	for i, src := range sources {
		engineSources[i] = src.Source
	}
	rt.engine = catalogsync.New(rt.store, rt.notify, engineSources...)
	for _, src := range sources {
		ps := &providerSync{fill: src.fill && rt.filler != nil, quiet: src.quiet, collections: src.Collections()}
		var complete bool
		ps.startup, complete = rt.status(src.Provider(), len(ps.collections))
		ps.stale = !complete || time.Since(ps.startup.LastSuccess) > src.refresh
		rt.providers[src.Provider()] = ps
	}
}

// collections returns what provider's sync covers, or nil when it is not
// synced, so its library menu offers matching lists.
func (rt *catalogRuntime) collections(provider string) []string {
	if ps := rt.providers[provider]; ps != nil && rt.catalog() != nil {
		return ps.collections
	}
	return nil
}

// catalog returns the catalog for the library, or nil to browse live.
func (rt *catalogRuntime) catalog() catalog.Catalog {
	if rt.store == nil || rt.engine == nil {
		return nil
	}
	if rt.filler == nil {
		return rt.store
	}
	return fillingCatalog{rt.store, rt.filler}
}

var _ catalogsync.AlbumStore = (*sqlite.Store)(nil)

// fillingCatalog is the store plus the filler's on-demand album fetch, so
// opening an uncached album caches it.
type fillingCatalog struct {
	catalog.Catalog
	filler *catalogsync.Filler
}

func (c fillingCatalog) FetchAlbumTracks(ctx context.Context, album catalog.Album) ([]catalog.Track, error) {
	return c.filler.FetchAlbumTracks(ctx, album)
}

// configure gives the UI the stored sync status and the refresh action.
func (rt *catalogRuntime) configure(m *model.Model) {
	if rt.engine == nil {
		return
	}
	status := make(map[string]model.CatalogStatus, len(rt.providers))
	for provider, ps := range rt.providers {
		if !ps.quiet {
			status[provider] = ps.startup
		}
	}
	m.SetCatalogSync(status, rt.refresh)
}

// status sums up a provider's collections: the oldest success (zero unless
// every collection has synced) and any stored error.
func (rt *catalogRuntime) status(provider string, collections int) (st model.CatalogStatus, complete bool) {
	stored, err := rt.store.SyncStatus(rt.ctx, provider)
	if err != nil {
		return st, false
	}
	complete = len(stored) >= collections
	for i, c := range stored {
		if c.LastSuccess.IsZero() {
			complete = false
		}
		if i == 0 || c.LastSuccess.Before(st.LastSuccess) {
			st.LastSuccess = c.LastSuccess
		}
		if c.LastError != "" {
			st.LastError = c.LastError
		}
	}
	if !complete {
		st.LastSuccess = time.Time{}
	}
	return st, complete
}

// start connects sync events to the program and syncs each provider that
// needs it. Uncached Spotify albums are filled after its sync, or at once
// when none is due.
func (rt *catalogRuntime) start(prog *tea.Program) {
	if rt.engine == nil {
		return
	}
	rt.send = prog.Send
	for provider, ps := range rt.providers {
		if ps.stale {
			rt.sync(provider)
		} else if ps.fill {
			rt.fill()
		}
	}
}

// refresh syncs provider now, or every provider when it is "". It is what
// "r" in the library calls.
func (rt *catalogRuntime) refresh(provider string) {
	if provider != "" {
		rt.sync(provider)
		return
	}
	for p := range rt.providers {
		rt.sync(p)
	}
}

// sync starts a sync of provider in the background. A sync of a provider
// with an album fill pauses the fill while it runs and fills newly saved
// albums after. A sync already running makes it a no-op, except for a quiet
// source: its sync is instant and follows a local change (a favorite
// toggled), so the running sync runs once more to include it.
func (rt *catalogRuntime) sync(provider string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	ps := rt.providers[provider]
	if rt.engine == nil || rt.closed || ps == nil {
		return
	}
	ps.requests++
	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		release := func() {}
		if ps.fill {
			release = rt.filler.Hold()
		}
		var err error
		for {
			rt.mu.Lock()
			seen := ps.requests
			rt.mu.Unlock()
			// A request whose Sync finds one running (ErrRunning) returns
			// below; the running one sees its count and runs again.
			err = rt.engine.Sync(rt.ctx, provider)
			rt.mu.Lock()
			again := ps.quiet && err == nil && ps.requests != seen && !rt.closed
			rt.mu.Unlock()
			if !again {
				break
			}
		}
		release()
		if errors.Is(err, catalogsync.ErrRunning) || rt.ctx.Err() != nil {
			return // the running sync, or quitting, owns what happens next
		}
		if err != nil {
			applog.Info("catalog sync: %v", err)
		}
		rt.scheduleRetry(provider, err != nil)
		if ps.fill {
			rt.fill()
		}
	}()
}

// scheduleRetry arranges provider's next sync after one finished: after a
// failure, a retry on a doubling delay; after a success, none, and the
// delay resets.
func (rt *catalogRuntime) scheduleRetry(provider string, failed bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	ps := rt.providers[provider]
	if ps == nil {
		return
	}
	if ps.retry != nil {
		ps.retry.Stop()
		ps.retry = nil
	}
	if !failed {
		ps.retryDelay = 0
		return
	}
	if rt.closed {
		return
	}
	ps.retryDelay = catalogsync.NextBackoff(ps.retryDelay, rt.retryMin, rt.retryMax)
	applog.Info("catalog sync %s: retrying in %v", provider, ps.retryDelay)
	ps.retry = time.AfterFunc(ps.retryDelay, func() { rt.sync(provider) })
}

// fill caches uncached saved albums' tracks in the background. A fill
// already running makes it a no-op; that run picks up new albums itself.
func (rt *catalogRuntime) fill() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.filler == nil || rt.closed {
		return
	}
	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		err := rt.filler.Run(rt.ctx)
		if err != nil && !errors.Is(err, catalogsync.ErrRunning) && !errors.Is(err, context.Canceled) {
			applog.Info("catalog album fill: %v", err)
		}
	}()
}

// notify forwards engine events, in order, from the syncing goroutine.
// start runs just before the program does, and prog.Send returns once the
// program has exited, so this cannot hang.
func (rt *catalogRuntime) notify(ev catalogsync.Event) {
	if rt.send == nil {
		return
	}
	if ps := rt.providers[ev.Provider]; ps != nil && ps.quiet {
		return
	}
	var phase model.CatalogSyncPhase
	switch ev.Kind {
	case catalogsync.Started:
		phase = model.CatalogSyncStarted
	case catalogsync.CollectionDone:
		phase = model.CatalogSyncCollectionDone
	case catalogsync.Finished:
		phase = model.CatalogSyncFinished
	}
	rt.send(model.CatalogSyncMsg{Provider: ev.Provider, Phase: phase, Collection: ev.Collection, Err: ev.Err})
}

// close stops any sync, waits briefly for it, and closes the catalog.
func (rt *catalogRuntime) close() {
	rt.mu.Lock()
	rt.closed = true
	for _, ps := range rt.providers {
		if ps.retry != nil {
			ps.retry.Stop()
		}
	}
	rt.mu.Unlock()
	rt.cancel()
	done := make(chan struct{})
	go func() { rt.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(catalogStopTimeout):
		// Leave the store open: the sync still holds it. The process is
		// exiting, and SQLite's WAL keeps the file consistent regardless.
		applog.Warn("catalog sync did not stop within %v", catalogStopTimeout)
		return
	}
	if rt.store != nil {
		if err := rt.store.Close(); err != nil {
			applog.Warn("close catalog: %v", err)
		}
	}
}
