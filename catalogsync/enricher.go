package catalogsync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bjarneo/cliamp/catalog"
)

// MetadataSource reads a track's real title, artists, album and year, for
// a provider whose sync knows only what its videos are called (YouTube).
type MetadataSource interface {
	// Provider is the catalog provider name, e.g. catalog.YouTube.
	Provider() string
	// TrackMetadata reads one track. It returns a zero TrackMetadata when
	// there is nothing to find, a *catalog.RateLimitError when the service
	// asks to slow down, and an error wrapping catalog.ErrForbidden when the
	// track cannot be read at all.
	TrackMetadata(ctx context.Context, track catalog.Ref) (catalog.TrackMetadata, error)
}

// EnrichStore is the part of the catalog the Enricher reads and writes.
type EnrichStore interface {
	UnenrichedTracks(ctx context.Context, provider string, limit int) ([]catalog.Track, error)
	EnrichTrack(ctx context.Context, track catalog.Ref, meta catalog.TrackMetadata) error
	// RefreshDerived rebuilds the provider's albums and artists from its
	// enriched library tracks.
	RefreshDerived(ctx context.Context, provider string) error
}

// enrichBatch is how many tracks the Enricher reads before refreshing the
// derived albums and artists, so the lists grow while it works.
const enrichBatch = 10

// Enricher fills in the library tracks of a provider with their real
// metadata, one at a time in the background, newest first. Like the
// Filler, it pauses while held, waits out rate limits with a doubling
// backoff, and gives up after repeated other failures. A track is read
// once: one with nothing to find, or that cannot be read, is marked read.
type Enricher struct {
	store EnrichStore
	src   MetadataSource
	pace  Pacing
	sleep func(ctx context.Context, d time.Duration) error // replaced in tests
	// changed, if set, is called after the derived albums and artists are
	// refreshed, so the UI can reload.
	changed func()
	pauser
}

// NewEnricher returns an Enricher of src's tracks in store. changed may
// be nil.
func NewEnricher(store EnrichStore, src MetadataSource, pace Pacing, changed func()) *Enricher {
	return &Enricher{store: store, src: src, pace: pace, sleep: sleepCtx, changed: changed}
}

// Run enriches every unread library track, refreshing the derived lists
// as it goes and at the end. It returns ErrRunning if a run is already
// going, ctx's error when cancelled, and an error after MaxFailures
// consecutive failures other than rate limits.
func (e *Enricher) Run(ctx context.Context) error {
	provider := e.src.Provider()
	if !e.begin() {
		return fmt.Errorf("enrich %s: %w", provider, ErrRunning)
	}
	defer e.end()

	// The library may have changed since the last run: refresh first.
	if err := e.refresh(ctx); err != nil {
		return err
	}
	var st fillState
	for {
		tracks, err := e.store.UnenrichedTracks(ctx, provider, enrichBatch)
		if err != nil {
			return fmt.Errorf("enrich %s: %w", provider, err)
		}
		if len(tracks) == 0 {
			return nil
		}
		for _, t := range tracks {
			if err := e.enrich(ctx, t, &st); err != nil {
				return fmt.Errorf("enrich %s: %q: %w", provider, t.Title, err)
			}
		}
		if err := e.refresh(ctx); err != nil {
			return err
		}
	}
}

func (e *Enricher) refresh(ctx context.Context) error {
	if err := e.store.RefreshDerived(ctx, e.src.Provider()); err != nil {
		return fmt.Errorf("enrich %s: %w", e.src.Provider(), err)
	}
	if e.changed != nil {
		e.changed()
	}
	return nil
}

// enrich reads one track, retrying it through rate limits and passing
// failures.
func (e *Enricher) enrich(ctx context.Context, t catalog.Track, st *fillState) error {
	for {
		if err := e.waitIdle(ctx); err != nil {
			return err
		}
		meta, err := e.src.TrackMetadata(ctx, t.Ref)
		if errors.Is(err, catalog.ErrForbidden) {
			meta, err = catalog.TrackMetadata{}, nil // unreadable: marked read
		}
		if err == nil {
			err = e.store.EnrichTrack(ctx, t.Ref, meta)
		}
		var rl *catalog.RateLimitError
		switch {
		case err == nil, errors.Is(err, catalog.ErrNotFound): // gone since listing
			st.backoff, st.failures = 0, 0
			return e.sleep(ctx, e.pace.Delay)
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.As(err, &rl):
			if err := e.sleep(ctx, max(st.next(e.pace), rl.RetryAfter)); err != nil {
				return err
			}
		default:
			st.failures++
			if st.failures >= e.pace.MaxFailures {
				return err
			}
			if err := e.sleep(ctx, st.next(e.pace)); err != nil {
				return err
			}
		}
	}
}
