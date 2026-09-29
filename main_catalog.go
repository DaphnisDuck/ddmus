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
	"github.com/bjarneo/cliamp/catalogsync/spotifysrc"
	"github.com/bjarneo/cliamp/external/spotify"
	"github.com/bjarneo/cliamp/internal/appdir"
	"github.com/bjarneo/cliamp/ui/model"
)

const (
	// catalogRefreshAfter is how old the last successful sync may get before
	// startup syncs again. M2.7 makes it configurable.
	catalogRefreshAfter = 30 * time.Minute
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
	// openCatalog, and the values are guarded by mu.
	providers map[string]*providerSync

	mu       sync.Mutex
	wg       sync.WaitGroup
	closed   bool
	send     func(tea.Msg) // set by start, before any sync runs
	retryMin time.Duration
	retryMax time.Duration
}

// providerSync is one provider's sync state.
type providerSync struct {
	collections int                 // how many collections a complete sync covers
	startup     model.CatalogStatus // stored status, read once at startup
	stale       bool                // startup should sync
	retry       *time.Timer         // pending retry of a failed sync
	retryDelay  time.Duration       // the last retry's delay; zero after a success
}

// openCatalog opens the catalog with a sync source for Spotify, when it is
// configured, and for musicDir, when it is set. A catalog that cannot open
// is logged and left out.
func openCatalog(sp *spotify.SpotifyProvider, musicDir string) *catalogRuntime {
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
	var sources []catalogsync.Source
	if sp != nil {
		src := spotifysrc.New(sp)
		sources = append(sources, src)
		rt.filler = catalogsync.NewFiller(rt.store, src, catalogsync.DefaultPacing)
	}
	if musicDir != "" {
		sources = append(sources, localsrc.New(musicDir, rt.store))
	}
	rt.setSources(sources...)
	return rt
}

// setSources creates the engine for sources and reads their stored status.
// Spotify syncs at startup when stale or incomplete; the local index always
// runs, since it rereads only changed files and writes nothing when none
// changed.
func (rt *catalogRuntime) setSources(sources ...catalogsync.Source) {
	if len(sources) == 0 {
		return
	}
	rt.engine = catalogsync.New(rt.store, rt.notify, sources...)
	for _, src := range sources {
		ps := &providerSync{collections: len(src.Collections())}
		var complete bool
		ps.startup, complete = rt.status(src.Provider(), ps.collections)
		ps.stale = src.Provider() == catalog.Local || !complete ||
			time.Since(ps.startup.LastSuccess) > catalogRefreshAfter
		rt.providers[src.Provider()] = ps
	}
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
		status[provider] = ps.startup
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
		} else if provider == catalog.Spotify {
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

// sync starts a sync of provider in the background. A Spotify sync pauses
// the album fill while it runs and fills newly saved albums after. A sync
// already running makes it a no-op.
func (rt *catalogRuntime) sync(provider string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.engine == nil || rt.closed || rt.providers[provider] == nil {
		return
	}
	spotify := provider == catalog.Spotify && rt.filler != nil
	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		release := func() {}
		if spotify {
			release = rt.filler.Hold()
		}
		err := rt.engine.Sync(rt.ctx, provider)
		release()
		if errors.Is(err, catalogsync.ErrRunning) || rt.ctx.Err() != nil {
			return // the running sync, or quitting, owns what happens next
		}
		if err != nil {
			applog.Info("catalog sync: %v", err)
		}
		rt.scheduleRetry(provider, err != nil)
		if spotify {
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
	ps.retryDelay = min(max(ps.retryDelay*2, rt.retryMin), rt.retryMax)
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
