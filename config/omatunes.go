// omatunes: settings for omatunes' own features, from the [omatunes] section
// of config.toml. Load hands the section's keys to parseKey through one
// tagged hook, so upstream merges of config.go stay cheap.

package config

import "time"

// DefaultSpotifyRefresh is how old the last Spotify sync may get before
// startup syncs again, when spotify_refresh is not set.
const DefaultSpotifyRefresh = 30 * time.Minute

// OmatunesConfig is the [omatunes] section.
type OmatunesConfig struct {
	// SpotifyRefresh is how old the last successful Spotify sync may get
	// before startup syncs again; 0 syncs at every startup.
	SpotifyRefresh time.Duration
}

func defaultOmatunesConfig() OmatunesConfig {
	return OmatunesConfig{SpotifyRefresh: DefaultSpotifyRefresh}
}

// parseKey applies one key of the [omatunes] section. Invalid values keep
// the default.
func (c *OmatunesConfig) parseKey(key, val string) {
	switch key {
	case "spotify_refresh":
		if d, err := time.ParseDuration(parseString(val)); err == nil && d >= 0 {
			c.SpotifyRefresh = d
		}
	}
}
