package model

// omatunes: library navigation stack behavior, driven through Update so the
// handleKey/Update hooks are exercised too.

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/library"
	"github.com/bjarneo/cliamp/playlist"
)

func libKey(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

// libPress sends key through Update and runs a returned library command,
// delivering its message too.
func libPress(t *testing.T, m Model, key string) Model {
	t.Helper()
	updated, cmd := m.Update(libKey(key))
	m = updated.(Model)
	return libRun(t, m, cmd)
}

func libRun(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case libraryLoadedMsg, libraryPlayMsg, libraryAuthDoneMsg:
		updated, next := m.Update(msg)
		return libRun(t, updated.(Model), next)
	}
	return m
}

func tracksOf(paths ...string) []playlist.Track {
	out := make([]playlist.Track, len(paths))
	for i, p := range paths {
		out[i] = playlist.Track{Path: p, Title: p}
	}
	return out
}

func newLibraryModel(root library.Level) Model {
	m := Model{player: &playbackFakeEngine{}, playlist: playlist.New(), plVisible: 10}
	m.SetLibrary(root)
	return m
}

func testLibraryRoot() library.Level {
	album := library.TrackLevel("Mahler 5", nil, func(context.Context) ([]playlist.Track, error) {
		return tracksOf("I", "II", "III"), nil
	})
	albums := library.Menu("Albums",
		library.Entry{Title: "Bruckner 7", Open: library.Menu("Bruckner 7")},
		library.Entry{Title: "Mahler 5", Open: album},
	)
	return library.Menu("Music",
		library.Entry{Title: "Local", Open: library.Menu("Local")},
		library.Entry{Title: "Spotify", Open: library.Menu("Spotify", library.Entry{Title: "Albums", Open: albums})},
		library.Entry{Title: "Search", Intent: library.IntentSearch},
	)
}

func libCrumb(m Model) string { return m.libBreadcrumb() }

func TestLibraryPushPopRestoresCursor(t *testing.T) {
	m := newLibraryModel(testLibraryRoot())
	if m.activeScreen() != screenLibrary {
		t.Fatalf("activeScreen = %v, want library", m.activeScreen())
	}
	for _, k := range []string{"j", "l", "enter", "j", "l"} {
		m = libPress(t, m, k)
	}
	if got := libCrumb(m); got != "Music / Spotify / Albums / Mahler 5" {
		t.Fatalf("breadcrumb = %q", got)
	}
	if n := len(m.libTop().entries); n != 3 {
		t.Fatalf("album tracks = %d, want 3", n)
	}

	m = libPress(t, m, "h")
	if got := libCrumb(m); got != "Music / Spotify / Albums" || m.libTop().cursor != 1 {
		t.Fatalf("after back: %q cursor %d, want Albums with cursor on Mahler 5", got, m.libTop().cursor)
	}
	m = libPress(t, m, "esc")
	m = libPress(t, m, "q") // q steps back below the root
	if got := libCrumb(m); got != "Music" || m.libTop().cursor != 1 {
		t.Fatalf("at root: %q cursor %d, want Music with cursor on Spotify", got, m.libTop().cursor)
	}
	if m.quitting {
		t.Fatal("q below the root quit")
	}
	updated, _ := m.Update(libKey("q"))
	if !updated.(Model).quitting {
		t.Error("q at the root did not quit")
	}
}

func TestLibraryMovementKeys(t *testing.T) {
	tests := []struct {
		keys []string
		want int
	}{
		{[]string{"j"}, 1},
		{[]string{"k"}, 2}, // wraps to the bottom
		{[]string{"G"}, 2},
		{[]string{"G", "g"}, 0},
		{[]string{"j", "j", "j"}, 0}, // wraps to the top
	}
	for _, tt := range tests {
		m := newLibraryModel(testLibraryRoot())
		for _, k := range tt.keys {
			m = libPress(t, m, k)
		}
		if got := m.libTop().cursor; got != tt.want {
			t.Errorf("keys %v: cursor = %d, want %d", tt.keys, got, tt.want)
		}
	}
}

func TestLibraryDropsStaleLoad(t *testing.T) {
	slowA := library.NewLevel("A", func(context.Context) ([]library.Entry, error) {
		return []library.Entry{{Title: "from A"}}, nil
	})
	root := library.Menu("Music", library.Entry{Title: "A", Open: slowA}, library.Entry{Title: "B", Open: library.Menu("B")})
	m := newLibraryModel(root)

	updated, staleCmd := m.Update(libKey("enter")) // open A, keep its load pending
	m = updated.(Model)
	m = libPress(t, m, "h")
	m = libPress(t, m, "j")
	m = libPress(t, m, "enter") // open B and load it
	updated, _ = m.Update(staleCmd())
	m = updated.(Model)
	if got := libCrumb(m); got != "Music / B" || len(m.libTop().entries) != 0 {
		t.Fatalf("stale A load leaked into %q: %+v", got, m.libTop().entries)
	}
}

func TestLibraryPlaysTrackInContext(t *testing.T) {
	m := newLibraryModel(testLibraryRoot())
	for _, k := range []string{"j", "l", "l", "j", "l", "j", "enter"} {
		m = libPress(t, m, k)
	}
	if m.playlist.Len() != 3 || m.playlist.Index() != 1 {
		t.Fatalf("queue = %d tracks at %d, want album of 3 at track II", m.playlist.Len(), m.playlist.Index())
	}
	if m.activeScreen() != screenLibrary {
		t.Error("playing left the library")
	}
}

func TestLibraryGatesKeys(t *testing.T) {
	m := newLibraryModel(testLibraryRoot())
	tests := []struct {
		key         string
		wantHandled bool
	}{
		{"S", true}, // provider jump: swallowed
		{"t", true}, // theme picker: swallowed
		{"o", true}, // file browser: swallowed
		{"+", false},
		{"s", false},
	}
	for _, tt := range tests {
		_, handled := m.handleLibraryKey(libKey(tt.key))
		if handled != tt.wantHandled {
			t.Errorf("key %q handled = %v, want %v", tt.key, handled, tt.wantHandled)
		}
	}

	m = libPress(t, m, "tab")
	if m.activeScreen() != screenMain {
		t.Fatalf("tab: activeScreen = %v, want the queue", m.activeScreen())
	}
	for key, wantHandled := range map[string]bool{"t": true, "j": false, "enter": false} {
		if _, handled := m.handleLibraryKey(libKey(key)); handled != wantHandled {
			t.Errorf("queue key %q handled = %v, want %v", key, handled, wantHandled)
		}
	}
	m = libPress(t, m, "tab")
	if m.activeScreen() != screenLibrary {
		t.Error("tab from the queue did not return to the library")
	}
}

type libAuthProv struct {
	authed bool
}

func (p *libAuthProv) Name() string                                { return "Auth" }
func (p *libAuthProv) Playlists() ([]playlist.PlaylistInfo, error) { return nil, nil }
func (p *libAuthProv) Tracks(string) ([]playlist.Track, error) {
	if !p.authed {
		return nil, playlist.ErrNeedsAuth
	}
	return tracksOf("x"), nil
}
func (p *libAuthProv) Authenticate() error { p.authed = true; return nil }

func TestLibrarySignInRetriesLoad(t *testing.T) {
	prov := &libAuthProv{}
	liked := library.TrackLevel("Liked", prov, func(context.Context) ([]playlist.Track, error) { return prov.Tracks("") })
	m := newLibraryModel(library.Menu("Music", library.Entry{Title: "Liked", Open: liked}))

	m = libPress(t, m, "enter")
	if m.libTop().err == nil {
		t.Fatal("expected a sign-in error before authenticating")
	}
	m = libPress(t, m, "enter") // sign in, then reload
	if m.libTop().err != nil || len(m.libTop().entries) != 1 {
		t.Fatalf("after sign-in: err %v, entries %d", m.libTop().err, len(m.libTop().entries))
	}
}
