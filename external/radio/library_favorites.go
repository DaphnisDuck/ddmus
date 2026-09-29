// omatunes: favorite stations for the library navigation. Kept in its own
// file so upstream merges of provider.go stay conflict-free.

package radio

import "github.com/bjarneo/cliamp/playlist"

// FavoriteTracks returns the favorite stations as playable tracks, in saved
// order. Unlike Playlists it is unaffected by an active catalog search.
func (p *Provider) FavoriteTracks() []playlist.Track {
	stations := p.favorites.Stations()
	tracks := make([]playlist.Track, len(stations))
	for i, s := range stations {
		tracks[i] = stationTrack(s)
	}
	return tracks
}
