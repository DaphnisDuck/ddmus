//go:build !windows

// omatunes: tests for the whole-collection catalog fetchers.

package spotify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/catalog"
	"github.com/bjarneo/cliamp/catalogsync/spotifysrc"
	"github.com/bjarneo/cliamp/playlist"
)

var _ spotifysrc.Client = (*SpotifyProvider)(nil)

func offsetOf(req *http.Request) int {
	n, _ := strconv.Atoi(req.URL.Query().Get("offset"))
	return n
}

func TestSavedAlbumRecordsPagesEverything(t *testing.T) {
	const total = 60
	p := fakeSpotifyAPI(t, func(req *http.Request) (any, int) {
		if req.URL.Path != "/v1/me/albums" {
			t.Errorf("unexpected path %s", req.URL.Path)
		}
		items := []map[string]any{}
		for i := offsetOf(req); i < min(offsetOf(req)+catalogPageSize, total); i++ {
			items = append(items, map[string]any{
				"added_at": "2024-05-06T07:08:09Z",
				"album": map[string]any{
					"id": fmt.Sprintf("al%d", i), "name": fmt.Sprintf("Album %d", i), "release_date": "1990-01-01",
					"total_tracks": 9, "artists": []map[string]any{{"id": "ar1", "name": "Ozawa"}},
					"images": []map[string]any{{"url": "https://img", "width": 300}},
				},
			})
		}
		return map[string]any{"items": items, "total": total}, 0
	})
	got, err := p.SavedAlbumRecords(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != total {
		t.Fatalf("albums = %d, want %d", len(got), total)
	}
	a := got[0]
	if a.Ref != (catalog.Ref{Provider: catalog.Spotify, ProviderID: "al0"}) || a.Year != 1990 || a.TrackCount != 9 ||
		a.ArtworkURL != "https://img" || len(a.Artists) != 1 || a.Artists[0].Ref.ProviderID != "ar1" ||
		!a.AddedAt.Equal(time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)) {
		t.Errorf("first album = %+v", a)
	}
}

// A library edited mid-read reports a different total; applying the result
// could drop items, so the read fails.
func TestPagedReadFailsWhenTheLibraryChanges(t *testing.T) {
	p := fakeSpotifyAPI(t, func(req *http.Request) (any, int) {
		total := 60
		if offsetOf(req) > 0 {
			total = 59 // an album was unsaved between pages
		}
		items := make([]map[string]any, min(catalogPageSize, total-offsetOf(req)))
		for i := range items {
			items[i] = map[string]any{"album": map[string]any{"id": fmt.Sprintf("al%d", offsetOf(req)+i)}}
		}
		return map[string]any{"items": items, "total": total}, 0
	})
	if _, err := p.SavedAlbumRecords(context.Background()); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("SavedAlbumRecords() = %v, want ErrIncomplete", err)
	}
}

func TestLikedTrackRecordsMapsAndSkips(t *testing.T) {
	p := fakeSpotifyAPI(t, func(req *http.Request) (any, int) {
		return map[string]any{"total": 4, "items": []map[string]any{
			{"added_at": "2024-01-01T00:00:00Z", "track": map[string]any{
				"id": "t1", "name": "I. Trauermarsch", "type": "track", "duration_ms": 780000,
				"track_number": 1, "disc_number": 1, "artists": []map[string]any{{"id": "ar1", "name": "Ozawa"}},
				"album": map[string]any{"id": "al1", "name": "Mahler 5", "release_date": "1990"},
			}},
			{"track": map[string]any{"id": "t2", "name": "Local", "is_local": true}},
			{"track": map[string]any{"id": "e1", "name": "Episode", "type": "episode"}},
			{"track": nil},
		}}, 0
	})
	got, err := p.LikedTrackRecords(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("tracks = %+v, want only the catalogable track", got)
	}
	tr := got[0]
	if tr.PlayableURI != "spotify:track:t1" || tr.Duration != 13*time.Minute || tr.Album == nil ||
		tr.Album.Ref.ProviderID != "al1" || tr.Year != 1990 || len(tr.Artists) != 1 {
		t.Errorf("track = %+v", tr)
	}
}

func TestPlaylistRecordsAndForbiddenItems(t *testing.T) {
	p := fakeSpotifyAPI(t, func(req *http.Request) (any, int) {
		switch req.URL.Path {
		case "/v1/me":
			return map[string]any{"id": "me"}, 0
		case "/v1/me/playlists":
			return map[string]any{"total": 2, "items": []map[string]any{
				{"id": "mine", "name": "Mine", "snapshot_id": "s1", "owner": map[string]any{"id": "me"}, "items": map[string]any{"total": 3}},
				{"id": "theirs", "name": "Theirs", "snapshot_id": "s2", "owner": map[string]any{"id": "other"}},
			}}, 0
		case "/v1/playlists/theirs/items":
			return map[string]any{"error": map[string]any{"status": 403}}, http.StatusForbidden
		case "/v1/playlists/mine/items":
			return map[string]any{"total": 1, "items": []map[string]any{{"item": map[string]any{"id": "t1", "name": "T", "type": "track"}}}}, 0
		}
		t.Errorf("unexpected path %s", req.URL.Path)
		return nil, http.StatusNotFound
	})
	ctx := context.Background()
	lists, err := p.PlaylistRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 2 || !lists[0].Own || lists[1].Own || lists[0].Snapshot != "s1" || lists[0].TrackCount != 3 {
		t.Fatalf("playlists = %+v", lists)
	}
	if tracks, err := p.PlaylistTrackRecords(ctx, "mine"); err != nil || len(tracks) != 1 {
		t.Errorf("mine tracks = %+v, %v", tracks, err)
	}
	if _, err := p.PlaylistTrackRecords(ctx, "theirs"); !errors.Is(err, catalog.ErrForbidden) {
		t.Errorf("theirs error = %v, want catalog.ErrForbidden", err)
	}
}

func TestFollowedArtistRecords(t *testing.T) {
	p := fakeSpotifyAPI(t, func(req *http.Request) (any, int) {
		if req.URL.Query().Get("after") == "" {
			return map[string]any{"artists": map[string]any{"total": 3, "next": "more", "cursors": map[string]any{"after": "a2"},
				"items": []map[string]any{{"id": "a1", "name": "One"}, {"id": "a2", "name": "Two"}}}}, 0
		}
		return map[string]any{"artists": map[string]any{"total": 3,
			"items": []map[string]any{{"id": "a3", "name": "Three"}}}}, 0
	})
	got, err := p.FollowedArtistRecords(context.Background())
	if err != nil || len(got) != 3 || got[2].Ref.ProviderID != "a3" {
		t.Fatalf("FollowedArtistRecords() = %+v, %v", got, err)
	}
}

func TestPlaylistItems404IsUnreadable(t *testing.T) {
	p := fakeSpotifyAPI(t, func(req *http.Request) (any, int) {
		return map[string]any{"error": map[string]any{"status": 404}}, http.StatusNotFound
	})
	if _, err := p.PlaylistTrackRecords(context.Background(), "gone"); !errors.Is(err, catalog.ErrForbidden) {
		t.Errorf("error = %v, want catalog.ErrForbidden", err)
	}
}

// Ownership decides Your vs Followed playlists; if the user cannot be
// identified the sync must fail rather than mark every playlist followed.
func TestPlaylistRecordsFailWithoutUserID(t *testing.T) {
	p := fakeSpotifyAPI(t, func(req *http.Request) (any, int) {
		if req.URL.Path == "/v1/me" {
			return map[string]any{"error": "busy"}, http.StatusServiceUnavailable
		}
		return map[string]any{"total": 0, "items": []any{}}, 0
	})
	if _, err := p.PlaylistRecords(context.Background()); err == nil {
		t.Error("PlaylistRecords() succeeded without a user ID")
	}
}

func TestClosedSessionReturnsNeedsAuth(t *testing.T) {
	p := fakeSpotifyAPI(t, func(req *http.Request) (any, int) { return map[string]any{}, 0 })
	p.mu.Lock()
	p.session = nil
	p.mu.Unlock()
	if _, err := p.webAPI(context.Background(), "GET", "/v1/me", nil); !errors.Is(err, playlist.ErrNeedsAuth) {
		t.Errorf("webAPI() after Close = %v, want ErrNeedsAuth", err)
	}
}
