// ddmus: tests for the library and catalog station helpers.

package radio

import "testing"

func TestLocalStationTracksAndFavoritesObserver(t *testing.T) {
	t.Setenv("CLIAMP_CONFIG_DIR", t.TempDir())
	p := New(Options{Favorites: &Favorites{byURL: map[string]struct{}{}}})
	local := p.LocalStationTracks()
	if len(local) == 0 || local[0].Path != BuiltinURL || !local[0].Stream {
		t.Fatalf("local stations = %+v, want the built-in station first", local)
	}

	toggled := 0
	p.OnFavoritesToggled(func() { toggled++ })
	p.AppendCatalog([]CatalogStation{{Name: "WBGO", URL: "https://wbgo"}})
	if _, _, err := p.ToggleFavorite("c:0"); err != nil {
		t.Fatal(err)
	}
	if toggled != 1 {
		t.Errorf("observer calls = %d, want 1", toggled)
	}
	if favs := p.FavoriteTracks(); len(favs) != 1 || favs[0].Path != "https://wbgo" {
		t.Errorf("favorites = %+v", favs)
	}
}
