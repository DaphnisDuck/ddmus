// omatunes: whole-collection fetchers for the catalog sync in cookie mode.
// yt-dlp reads the account's playlists feed, playlists and Liked Music with
// the browser session, and each fetcher returns catalog records carrying
// YouTube IDs, or an error: never a partial collection. Kept in its own file
// so upstream merges of the provider files stay conflict-free.

package ytmusic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/bjarneo/cliamp/applog"
	"github.com/bjarneo/cliamp/catalog"
)

// ErrIncomplete means a playlist read ended with a different number of
// entries than YouTube reported, usually because it changed mid-read.
var ErrIncomplete = errors.New("youtube: playlist changed while reading")

// notOwnLists are account lists that are never synced as playlists: Watch
// later, Liked videos (all liked videos, music or not), and Liked Music,
// which is synced as the liked collection.
var notOwnLists = map[string]bool{"WL": true, "LL": true, likedMusicID: true}

// likedMusicID is YouTube Music's Liked Music playlist.
const likedMusicID = "LM"

// CookieCatalog reads a YouTube account for the catalog through yt-dlp,
// signed in with a browser's cookies.
type CookieCatalog struct {
	browser string
	// run runs yt-dlp with args and returns its stdout; replaced in tests.
	run func(ctx context.Context, args ...string) ([]byte, error)
}

// NewCookieCatalog returns a CookieCatalog for browser, a yt-dlp
// --cookies-from-browser value such as "brave+gnomekeyring".
func NewCookieCatalog(browser string) *CookieCatalog {
	return &CookieCatalog{browser: browser, run: runYTDLP}
}

// classificationScope keys this account's playlist classification in
// cliamp's cache file, apart from OAuth accounts.
func (c *CookieCatalog) classificationScope() string { return "cookies:" + c.browser }

// PlaylistRecords returns the account's music playlists, without tracks.
// A playlist is music when a sampled video's YouTube category is Music, as
// in OAuth mode; each playlist is sampled once and the answer cached.
func (c *CookieCatalog) PlaylistRecords(ctx context.Context) ([]catalog.PlaylistRecord, error) {
	out, err := c.ytdlp(ctx, "https://www.youtube.com/feed/playlists", "--flat-playlist", "-j")
	if err != nil {
		return nil, fmt.Errorf("youtube: playlists: %w", err)
	}
	type listed struct{ id, title string }
	var lists []listed
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var e struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("youtube: parse playlists: %w", err)
		}
		if e.ID != "" && !notOwnLists[e.ID] {
			lists = append(lists, listed{e.ID, e.Title})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("youtube: read playlists: %w", err)
	}

	scope := c.classificationScope()
	music := loadClassification(scope)
	if music == nil {
		music = map[string]bool{}
	}
	changed := false
	var records []catalog.PlaylistRecord
	for _, l := range lists {
		isMusic, known := music[l.id]
		if !known {
			if isMusic, err = c.isMusic(ctx, l.id); err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				// Left out, and not cached, until a later sync can tell;
				// one unreadable playlist must not fail the others.
				applog.Info("youtube: cannot classify playlist %q yet: %v", l.title, err)
				continue
			}
			music[l.id], changed = isMusic, true
		}
		if isMusic {
			records = append(records, catalog.PlaylistRecord{Ref: youtubeRef(l.id), Name: l.title, Own: true})
		}
	}
	if changed {
		saveClassification(scope, music)
	}
	return records, nil
}

// isMusic samples a playlist's first readable video, of its first five, and
// reports whether YouTube files it under Music. A video listed as playable
// can still refuse a full read (blocked in this region, say), so a refused
// one passes to the next. A playlist with no video is not music; one whose
// videos all refuse is an error.
func (c *CookieCatalog) isMusic(ctx context.Context, playlistID string) (bool, error) {
	out, err := c.ytdlp(ctx, playlistURL(playlistID), "--flat-playlist", "-J", "--playlist-end", "5")
	if err != nil {
		return false, err
	}
	var pl struct {
		Entries []ytdlpEntry `json:"entries"`
	}
	if err := json.Unmarshal(out, &pl); err != nil {
		return false, fmt.Errorf("parse playlist: %w", err)
	}
	lastErr := error(nil)
	for _, e := range pl.Entries {
		if _, ok := e.record(); !ok {
			continue
		}
		out, err := c.ytdlp(ctx, watchURL(e.ID), "-j", "--skip-download")
		if errors.Is(err, catalog.ErrForbidden) {
			lastErr = err
			continue
		}
		if err != nil {
			return false, err
		}
		var video struct {
			Categories []string `json:"categories"`
		}
		if err := json.Unmarshal(out, &video); err != nil {
			return false, fmt.Errorf("parse video: %w", err)
		}
		return slices.Contains(video.Categories, "Music"), nil
	}
	return false, lastErr
}

// PlaylistTrackRecords returns a playlist's tracks in order. A playlist
// that is gone or private fails with an error wrapping catalog.ErrForbidden.
func (c *CookieCatalog) PlaylistTrackRecords(ctx context.Context, playlistID string) ([]catalog.TrackRecord, error) {
	return c.playlistTracks(ctx, playlistID)
}

// LikedTrackRecords returns the account's Liked Music.
func (c *CookieCatalog) LikedTrackRecords(ctx context.Context) ([]catalog.TrackRecord, error) {
	return c.playlistTracks(ctx, likedMusicID)
}

func (c *CookieCatalog) playlistTracks(ctx context.Context, playlistID string) ([]catalog.TrackRecord, error) {
	out, err := c.ytdlp(ctx, playlistURL(playlistID), "--flat-playlist", "-J")
	if err != nil {
		return nil, fmt.Errorf("youtube: playlist %s: %w", playlistID, err)
	}
	var pl struct {
		Count   int          `json:"playlist_count"`
		Entries []ytdlpEntry `json:"entries"`
	}
	if err := json.Unmarshal(out, &pl); err != nil {
		return nil, fmt.Errorf("youtube: parse playlist %s: %w", playlistID, err)
	}
	// Every entry counts toward completeness, playable or not.
	if len(pl.Entries) != pl.Count {
		return nil, fmt.Errorf("youtube: playlist %s: read %d of %d: %w", playlistID, len(pl.Entries), pl.Count, ErrIncomplete)
	}
	tracks := make([]catalog.TrackRecord, 0, len(pl.Entries))
	for _, e := range pl.Entries {
		if rec, ok := e.record(); ok {
			tracks = append(tracks, rec)
		}
	}
	return tracks, nil
}

// ytdlpEntry is a flat playlist entry.
type ytdlpEntry struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Channel      string  `json:"channel"`
	ChannelID    string  `json:"channel_id"`
	Duration     float64 `json:"duration"`
	Availability string  `json:"availability"`
}

// record maps an entry. Private and deleted videos have no track.
func (e ytdlpEntry) record() (catalog.TrackRecord, bool) {
	if e.ID == "" || e.Availability == "private" || e.Title == "[Private video]" || e.Title == "[Deleted video]" {
		return catalog.TrackRecord{}, false
	}
	rec := catalog.TrackRecord{
		Ref: youtubeRef(e.ID), Title: e.Title, PlayableURI: watchURL(e.ID),
		Duration: time.Duration(e.Duration * float64(time.Second)),
	}
	if artist, ok := channelArtist(e.ChannelID, e.Channel); ok {
		rec.Artists = []catalog.ArtistRecord{artist}
	}
	return rec, true
}

// channelArtist is a video's uploading channel as its artist: an official
// "X - Topic" channel is the artist X. Channels are keyed by ID.
func channelArtist(id, name string) (catalog.ArtistRecord, bool) {
	name = strings.TrimSpace(cleanChannelName(name))
	if id == "" || name == "" {
		return catalog.ArtistRecord{}, false
	}
	return catalog.ArtistRecord{Ref: youtubeRef("channel:" + id), Name: name}, true
}

func youtubeRef(id string) catalog.Ref { return catalog.Ref{Provider: catalog.YouTube, ProviderID: id} }

func playlistURL(id string) string { return "https://www.youtube.com/playlist?list=" + id }

// watchURL is the track's playable address: the path cliamp's YouTube
// playback already resolves.
func watchURL(id string) string { return "https://music.youtube.com/watch?v=" + id }

// ytdlp runs yt-dlp on url with the browser's cookies.
func (c *CookieCatalog) ytdlp(ctx context.Context, url string, args ...string) ([]byte, error) {
	full := append([]string{"--cookies-from-browser", c.browser, "--socket-timeout", "15", "--no-warnings"}, args...)
	return c.run(ctx, append(full, "--", url)...)
}

// runYTDLP runs yt-dlp and returns its stdout. A failure carries yt-dlp's
// last error line; a missing or private playlist wraps catalog.ErrForbidden,
// so a sync skips it and keeps what it had.
func runYTDLP(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	cmd.WaitDelay = 3 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return nil, ytdlpError(stderr.String(), err)
}

// ytdlpError turns yt-dlp's stderr into an error.
func ytdlpError(stderr string, cause error) error {
	msg := ""
	for _, line := range strings.Split(strings.TrimSpace(stderr), "\n") {
		if strings.HasPrefix(line, "ERROR:") {
			msg = strings.TrimSpace(strings.TrimPrefix(line, "ERROR:"))
		}
	}
	if msg == "" {
		return fmt.Errorf("yt-dlp: %w", cause)
	}
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "does not exist") || strings.Contains(lower, "playlist is private") ||
		strings.Contains(lower, "not available") || strings.Contains(lower, "video unavailable") {
		return fmt.Errorf("yt-dlp: %s: %w", msg, catalog.ErrForbidden)
	}
	return fmt.Errorf("yt-dlp: %s", msg)
}
