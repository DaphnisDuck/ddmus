// omatunes: favorite and local stations for the library navigation and the
// catalog. Kept in its own file so upstream merges of provider.go stay
// conflict-free.

package radio

import (
	"sync"

	"github.com/bjarneo/cliamp/playlist"
)

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

// LocalStationTracks returns the built-in station and those from
// radios.toml as playable tracks, in file order.
func (p *Provider) LocalStationTracks() []playlist.Track {
	p.mu.Lock()
	defer p.mu.Unlock()
	tracks := make([]playlist.Track, len(p.stations))
	for i, s := range p.stations {
		tracks[i] = playlist.Track{Path: s.url, Title: s.name, Stream: true, Realtime: true}
	}
	return tracks
}

// favoriteObservers holds each provider's OnFavoritesToggled callback, so
// the upstream Provider struct needs no new field.
var favoriteObservers sync.Map // *Provider → func()

// OnFavoritesToggled registers fn to run after every ToggleFavorite, for
// example to resync the catalogued favorites. fn runs with the provider
// locked and must not call back into it; it should only start work.
func (p *Provider) OnFavoritesToggled(fn func()) { favoriteObservers.Store(p, fn) }

func (p *Provider) favoritesToggled() {
	if fn, ok := favoriteObservers.Load(p); ok {
		fn.(func())()
	}
}

// SearchStationTracks searches the Radio Browser directory by station name
// and returns the playable stations, most voted first. Unlike SearchCatalog
// it leaves the provider's own search results alone.
func (p *Provider) SearchStationTracks(query string) ([]playlist.Track, error) {
	stations, err := Stations(StationQuery{Name: query, Order: SortVotes, Limit: searchLimit})
	if err != nil {
		return nil, err
	}
	return stationTracks(streamableStations(stations)), nil
}
