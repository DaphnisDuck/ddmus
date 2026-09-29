package main

// omatunes: the catalog runtime. It opens the SQLite catalog, runs the
// background Spotify sync and reports it to the library UI. Kept out of
// main.go so upstream merges there stay conflict-free.

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
)

// catalogRuntime owns the catalog for one run. Without a store, browsing
// stays live, as in v0.1.
type catalogRuntime struct {
	store  *sqlite.Store
	engine *catalogsync.Engine
	ctx    context.Context
	cancel context.CancelFunc

	collections int                 // how many collections a complete sync covers
	startup     model.CatalogStatus // stored status, read once at startup
	stale       bool                // startup should sync

	mu     sync.Mutex
	wg     sync.WaitGroup
	closed bool
	send   func(tea.Msg) // set by start, before any sync runs
}

// openCatalog opens the catalog and, when Spotify is configured, a sync
// engine for it. A catalog that cannot open is logged and left out.
func openCatalog(sp *spotify.SpotifyProvider) *catalogRuntime {
	rt := &catalogRuntime{}
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
	if sp != nil {
		src := spotifysrc.New(sp)
		rt.collections = len(src.Collections())
		rt.engine = catalogsync.New(rt.store, rt.notify, src)
		var complete bool
		rt.startup, complete = rt.status()
		rt.stale = !complete || time.Since(rt.startup.LastSuccess) > catalogRefreshAfter
	}
	return rt
}

// catalog returns the catalog for the library, or nil to browse live.
func (rt *catalogRuntime) catalog() catalog.Catalog {
	if rt.store == nil || rt.engine == nil {
		return nil
	}
	return rt.store
}

// configure gives the UI the stored sync status and the refresh action.
func (rt *catalogRuntime) configure(m *model.Model) {
	if rt.engine == nil {
		return
	}
	m.SetCatalogSync(map[string]model.CatalogStatus{catalog.Spotify: rt.startup}, rt.refresh)
}

// status sums up Spotify's collections: the oldest success (zero unless
// every collection has synced) and any stored error.
func (rt *catalogRuntime) status() (st model.CatalogStatus, complete bool) {
	collections, err := rt.store.SyncStatus(rt.ctx, catalog.Spotify)
	if err != nil {
		return st, false
	}
	complete = len(collections) >= rt.collections
	for i, c := range collections {
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

// start connects sync events to the program and syncs if the catalog is
// stale or incomplete.
func (rt *catalogRuntime) start(prog *tea.Program) {
	if rt.engine == nil {
		return
	}
	rt.send = prog.Send
	if rt.stale {
		rt.refresh()
	}
}

// refresh starts a Spotify sync in the background. A sync already running
// makes it a no-op.
func (rt *catalogRuntime) refresh() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.engine == nil || rt.closed {
		return
	}
	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		err := rt.engine.Sync(rt.ctx, catalog.Spotify)
		if err != nil && !errors.Is(err, catalogsync.ErrRunning) && !errors.Is(err, context.Canceled) {
			applog.Info("catalog sync: %v", err)
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
