package sqlite

import (
	"context"
	"testing"
	"time"
)

func TestRateLimitedUntilRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openTemp(t)
	if got, err := s.RateLimitedUntil(ctx, "spotify"); err != nil || !got.IsZero() {
		t.Fatalf("before any block: %v, %v; want zero", got, err)
	}
	for _, until := range []time.Time{time.UnixMilli(1_790_000_000_000), time.UnixMilli(1_790_000_600_000)} {
		if err := s.SetRateLimitedUntil(ctx, "spotify", until); err != nil {
			t.Fatal(err)
		}
		if got, err := s.RateLimitedUntil(ctx, "spotify"); err != nil || !got.Equal(until) {
			t.Errorf("got %v, %v; want %v", got, err, until)
		}
	}
	if got, _ := s.RateLimitedUntil(ctx, "youtube"); !got.IsZero() {
		t.Errorf("another provider: %v, want zero", got)
	}
}
