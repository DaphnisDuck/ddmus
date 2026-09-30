package catalogsync

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
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
	// RecordEnrichFailure counts a run in which the track could not be
	// read, marking it read at giveUpAfter counted runs.
	RecordEnrichFailure(ctx context.Context, track catalog.Ref, giveUpAfter int) (gaveUp bool, err error)
}

const (
	// enrichBatch is how many tracks the Enricher reads before refreshing
	// the derived albums and artists, so the lists grow while it works.
	enrichBatch = 10
	// enrichAttempts is how often a run tries one track before skipping it.
	enrichAttempts = 2
	// enrichGiveUpRuns is how many runs may fail to read a track before it
	// is marked read, like a track that cannot be read at all.
	enrichGiveUpRuns = 3
)

// Enricher fills in the library tracks of a provider with their real
// metadata, one at a time in the background, newest first. Like the
// Filler, it pauses while held, waits out rate limits with a doubling
// backoff, and skips a track that keeps failing. A track is read once: one
// with nothing to find, or that cannot be read, is marked read, and so is
// one that failed in enrichGiveUpRuns runs.
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
// as it goes and at the end. A track that fails enrichAttempts times is
// skipped for the rest of the run and its failure counted. MaxFailures
// skipped in a row look like an outage rather than bad tracks: the run
// ends with the error, and that streak is not counted against its tracks.
// Run returns ErrRunning if a run is already going, ctx's error when
// cancelled, and an error naming the last failure when tracks were skipped.
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
	var (
		st      fillState
		skip    = make(map[catalog.Ref]bool)
		failed  []catalog.Ref // skipped between successes: counted
		streak  []catalog.Ref // skipped since the last success
		lastErr error
	)
	for {
		tracks, err := e.store.UnenrichedTracks(ctx, provider, enrichBatch+len(skip))
		if err != nil {
			return fmt.Errorf("enrich %s: %w", provider, err)
		}
		tracks = slices.DeleteFunc(tracks, func(t catalog.Track) bool { return skip[t.Ref] })
		if len(tracks) == 0 {
			return e.countFailures(ctx, append(failed, streak...), lastErr)
		}
		found := false
		for _, t := range tracks {
			ok, err := e.enrich(ctx, t, &st)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				skip[t.Ref] = true
				streak = append(streak, t.Ref)
				lastErr = fmt.Errorf("enrich %s: %q: %w", provider, t.Title, err)
				if len(streak) >= e.pace.MaxFailures {
					if cerr := e.countFailures(ctx, failed, nil); cerr != nil {
						return errors.Join(lastErr, cerr)
					}
					return lastErr
				}
				continue
			}
			failed, streak = append(failed, streak...), nil
			found = found || ok
		}
		// A batch that found nothing changed no album or artist.
		if found {
			if err := e.refresh(ctx); err != nil {
				return err
			}
		}
	}
}

// countFailures records a failed run for each of tracks and returns
// lastErr, so the run's end reports that tracks were skipped.
func (e *Enricher) countFailures(ctx context.Context, tracks []catalog.Ref, lastErr error) error {
	for _, t := range tracks {
		if _, err := e.store.RecordEnrichFailure(ctx, t, enrichGiveUpRuns); err != nil && !errors.Is(err, catalog.ErrNotFound) {
			return errors.Join(lastErr, err)
		}
	}
	if lastErr != nil {
		return fmt.Errorf("skipped %d tracks: %w", len(tracks), lastErr)
	}
	return nil
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

// enrich reads one track, retrying it through rate limits and up to
// enrichAttempts other failures, and reports whether it found music
// details. After its last failure it waits the backoff, so a run meeting
// an outage slows down before the next track.
func (e *Enricher) enrich(ctx context.Context, t catalog.Track, st *fillState) (bool, error) {
	for attempt := 1; ; {
		if err := e.waitIdle(ctx); err != nil {
			return false, err
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
			st.backoff = 0
			return err == nil && meta.Found(), e.sleep(ctx, e.pace.Delay)
		case ctx.Err() != nil:
			return false, ctx.Err()
		case errors.As(err, &rl):
			if err := e.sleep(ctx, max(st.next(e.pace), rl.RetryAfter)); err != nil {
				return false, err
			}
		default:
			if serr := e.sleep(ctx, st.next(e.pace)); serr != nil || attempt >= enrichAttempts {
				return false, cmp.Or(serr, err)
			}
			attempt++
		}
	}
}
