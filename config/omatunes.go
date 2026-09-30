// omatunes: settings for omatunes' own features, from the [omatunes] section
// of config.toml. Load hands the section's keys to parseKey through one
// tagged hook, so upstream merges of config.go stay cheap.

package config

import "time"

// Defaults for how old a source's last sync may get before startup syncs
// again: Spotify's API is cheap, YouTube's reads cost quota or yt-dlp time.
const (
	DefaultSpotifyRefresh = 30 * time.Minute
	DefaultYouTubeRefresh = 2 * time.Hour
)

// OmatunesConfig is the [omatunes] section.
type OmatunesConfig struct {
	// SpotifyRefresh is how old the last successful Spotify sync may get
	// before startup syncs again; 0 syncs at every startup.
	SpotifyRefresh time.Duration
	// YouTubeRefresh is the same for YouTube Music.
	YouTubeRefresh time.Duration
	// YouTubePlaylists are other people's playlists to sync, as links or
	// IDs: YouTube lists the playlists you save nowhere a sync can read.
	YouTubePlaylists []string
}

func defaultOmatunesConfig() OmatunesConfig {
	return OmatunesConfig{SpotifyRefresh: DefaultSpotifyRefresh, YouTubeRefresh: DefaultYouTubeRefresh}
}

// parseKey applies one key of the [omatunes] section. Invalid values keep
// the default.
func (c *OmatunesConfig) parseKey(key, val string) {
	switch key {
	case "spotify_refresh":
		setDuration(&c.SpotifyRefresh, val)
	case "youtube_refresh":
		setDuration(&c.YouTubeRefresh, val)
	case "youtube_playlists":
		c.YouTubePlaylists = parseStringSlice(val)
	}
}

// setDuration sets d from a duration value, keeping d when it is invalid
// or negative.
func setDuration(d *time.Duration, val string) {
	if v, err := time.ParseDuration(parseString(val)); err == nil && v >= 0 {
		*d = v
	}
}
