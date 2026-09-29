// omatunes: tests for the cookie-mode catalog fetchers.

package ytmusic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/catalog"
)

// fakeYTDLP answers yt-dlp runs by URL and counts them.
type fakeYTDLP struct {
	mu        sync.Mutex
	feed      []map[string]any          // playlists feed entries
	playlists map[string]map[string]any // playlist ID → -J output
	videos    map[string][]string       // video ID → categories
	blocked   map[string]bool           // video IDs whose full read is refused
	runs      []string
}

func (f *fakeYTDLP) run(_ context.Context, args ...string) ([]byte, error) {
	url := args[len(args)-1]
	f.mu.Lock()
	f.runs = append(f.runs, url)
	f.mu.Unlock()
	if !slices.Contains(args, "--cookies-from-browser") {
		return nil, errors.New("no cookies passed")
	}
	switch {
	case strings.HasSuffix(url, "/feed/playlists"):
		var lines []string
		for _, e := range f.feed {
			b, _ := json.Marshal(e)
			lines = append(lines, string(b))
		}
		return []byte(strings.Join(lines, "\n") + "\n"), nil
	case strings.Contains(url, "playlist?list="):
		id := url[strings.Index(url, "list=")+5:]
		pl, ok := f.playlists[id]
		if !ok {
			return nil, ytdlpError("ERROR: [youtube:tab] "+id+": The playlist does not exist.", errors.New("exit 1"))
		}
		return json.Marshal(pl)
	case strings.Contains(url, "watch?v="):
		id := url[strings.Index(url, "v=")+2:]
		if f.blocked[id] {
			return nil, ytdlpError("ERROR: [youtube] "+id+": Video unavailable. It was blocked due to the claimed content", errors.New("exit 1"))
		}
		return json.Marshal(map[string]any{"id": id, "categories": f.videos[id]})
	}
	return nil, fmt.Errorf("unexpected url %s", url)
}

func (f *fakeYTDLP) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.runs {
		if strings.Contains(r, prefix) {
			n++
		}
	}
	return n
}

func entry(id, title, channel, channelID string, secs float64) map[string]any {
	return map[string]any{"id": id, "title": title, "channel": channel, "channel_id": channelID, "duration": secs}
}

func newFakeCatalog(t *testing.T) (*CookieCatalog, *fakeYTDLP) {
	t.Helper()
	t.Setenv("CLIAMP_CONFIG_DIR", t.TempDir()) // the classification cache
	f := &fakeYTDLP{
		feed: []map[string]any{
			{"id": "WL", "title": "Watch later"}, {"id": "LL", "title": "Liked videos"},
			{"id": "PLmusic", "title": "Road Trip"}, {"id": "PLshows", "title": "Game Shows"},
			{"id": "PLempty", "title": "Nothing Yet"},
		},
		playlists: map[string]map[string]any{
			"PLmusic": {"playlist_count": 3, "entries": []map[string]any{
				{"id": "gone", "title": "[Deleted video]"},
				entry("v1", "Libertango", "Astor Piazzolla Oficial", "UCpiaz", 246),
				entry("v2", "Gong-Hu", "Rana Park - Topic", "UCrana", 1545.5),
			}},
			"PLshows": {"playlist_count": 1, "entries": []map[string]any{entry("s1", "Episode 1", "TV", "UCtv", 1500)}},
			"PLempty": {"playlist_count": 0, "entries": []map[string]any{}},
			"LM":      {"playlist_count": 1, "entries": []map[string]any{entry("v2", "Gong-Hu", "Rana Park - Topic", "UCrana", 1545)}},
		},
		videos: map[string][]string{"v1": {"Music"}, "v2": {"Music"}, "s1": {"Entertainment"}},
	}
	c := NewCookieCatalog("brave+gnomekeyring")
	c.run = f.run
	return c, f
}

func TestCookiePlaylistRecordsClassifiesOnce(t *testing.T) {
	c, f := newFakeCatalog(t)
	lists, err := c.PlaylistRecords(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Music only: not Watch later or Liked videos, not a show, not empty.
	if len(lists) != 1 || lists[0].Ref != (catalog.Ref{Provider: catalog.YouTube, ProviderID: "PLmusic"}) ||
		lists[0].Name != "Road Trip" || !lists[0].Own {
		t.Fatalf("playlists = %+v", lists)
	}
	// The deleted first video was skipped: v1 was sampled.
	if f.count("watch?v=v1") != 1 || f.count("watch?v=gone") != 0 {
		t.Errorf("sampled %v", f.runs)
	}
	for _, id := range []string{"WL", "LL"} {
		if f.count("list="+id) != 0 {
			t.Errorf("%s was read", id)
		}
	}
	// Classification is cached: a second listing samples nothing.
	before := len(f.runs)
	if _, err := c.PlaylistRecords(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := len(f.runs) - before; got != 1 {
		t.Errorf("second listing ran yt-dlp %d times, want only the feed", got)
	}
}

func TestCookiePlaylistTracks(t *testing.T) {
	c, _ := newFakeCatalog(t)
	tracks, err := c.PlaylistTrackRecords(context.Background(), "PLmusic")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 {
		t.Fatalf("tracks = %+v, want the deleted video skipped", tracks)
	}
	gong := tracks[1]
	if gong.Ref.ProviderID != "v2" || gong.Title != "Gong-Hu" || gong.PlayableURI != "https://music.youtube.com/watch?v=v2" ||
		gong.Duration != 1545500*time.Millisecond || len(gong.Artists) != 1 ||
		gong.Artists[0] != (catalog.ArtistRecord{Ref: catalog.Ref{Provider: catalog.YouTube, ProviderID: "channel:UCrana"}, Name: "Rana Park"}) {
		t.Errorf("Gong-Hu = %+v", gong)
	}
	liked, err := c.LikedTrackRecords(context.Background())
	if err != nil || len(liked) != 1 || liked[0].Ref.ProviderID != "v2" {
		t.Errorf("liked = %+v, %v", liked, err)
	}
}

func TestCookiePlaylistReadFailures(t *testing.T) {
	c, f := newFakeCatalog(t)
	f.playlists["PLmusic"]["playlist_count"] = 4 // one more than was read
	if _, err := c.PlaylistTrackRecords(context.Background(), "PLmusic"); !errors.Is(err, ErrIncomplete) {
		t.Errorf("short read = %v, want ErrIncomplete", err)
	}
	if _, err := c.PlaylistTrackRecords(context.Background(), "PLmissing"); !errors.Is(err, catalog.ErrForbidden) {
		t.Errorf("missing playlist = %v, want ErrForbidden", err)
	}
}

func TestYTDLPError(t *testing.T) {
	tests := []struct {
		stderr    string
		forbidden bool
		want      string
	}{
		{"WARNING: x\nERROR: [youtube:tab] PLx: The playlist does not exist.", true, "does not exist"},
		{"ERROR: [youtube:tab] PLx: This playlist is private", true, "private"},
		{"ERROR: [youtube:tab] playlists: HTTP Error 401: Unauthorized", false, "401"},
		{"", false, "exit 1"},
	}
	for _, tt := range tests {
		err := ytdlpError(tt.stderr, errors.New("exit 1"))
		if errors.Is(err, catalog.ErrForbidden) != tt.forbidden || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("ytdlpError(%q) = %v", tt.stderr, err)
		}
	}
}

// A video refused on a full read (blocked here) passes to the next; a
// playlist whose sampled videos all refuse is left out, uncached, while the
// rest still sync.
func TestCookieClassificationSkipsRefusedVideos(t *testing.T) {
	c, f := newFakeCatalog(t)
	f.blocked = map[string]bool{"v1": true, "s1": true}
	lists, err := c.PlaylistRecords(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 1 || lists[0].Name != "Road Trip" || f.count("watch?v=v2") != 1 {
		t.Fatalf("playlists = %+v, runs %v; want Road Trip classified from its second video", lists, f.runs)
	}
	// Game Shows could not be told: not cached, so it is sampled again.
	f.blocked = nil
	before := f.count("watch?v=s1")
	if _, err := c.PlaylistRecords(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.count("watch?v=s1") != before+1 || f.count("watch?v=v2") != 1 {
		t.Errorf("runs %v; want only Game Shows sampled again", f.runs)
	}
}
