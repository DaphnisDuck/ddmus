package model

// ddmus: the Ctrl+K / ? overlay lists only keys the library acts on or
// passes through to cliamp.

import (
	"slices"
	"testing"

	"github.com/bjarneo/cliamp/library"
)

func keymapActions(m Model) []string {
	var out []string
	for _, e := range m.buildKeymapEntries() {
		if !e.divider {
			out = append(out, e.action)
		}
	}
	return out
}

func TestLibraryKeymapListsLiveKeysOnly(t *testing.T) {
	browse := newLibraryModel(testLibraryRoot())
	browse.lib.refresh = func(string) {}

	queue := newLibraryModel(testLibraryRoot())
	queue = libPress(t, queue, "tab")

	input, _ := newSearchModel(t)
	input = libPress(t, input, "/")
	input = typeText(t, input, "x")

	results := libPress(t, input, "enter")

	ordered := newLibraryModel(library.Menu("Music", library.Entry{Title: "Albums", Open: orderedLevel{backward: new(bool)}}))
	ordered = libPress(t, ordered, "enter")

	upstream := Model{}

	tests := []struct {
		name      string
		m         Model
		want, not []string
	}{
		{"browse", browse,
			[]string{"Move (Up/Down)", "Open or play", "Back (Backspace)", "Search", "Queue", "Sync", "Stop", "Next track", "Previous track", "Seek +/-large step", "Volume up/down", "Help"},
			[]string{"Change order", "Track info / metadata", "Search active provider or YouTube", "Toggle queue (play next)", "Back to provider", "Seek +/-5s", "Quit", "Focus", "Filter/search list"}},
		{"ordered list", ordered, []string{"Change order"}, []string{"Sync"}},
		{"queue", queue,
			[]string{"Library", "Play selected track", "Filter/search list", "Seek +/-5s", "Quit", "Stop", "Help",
				"Toggle shuffle", "Cycle repeat", "Toggle queue (play next)", "Queue manager", "Track info / metadata",
				"Remove selected track from playlist", "Move track up/down", "Cycle EQ preset", "Toggle mono",
				"Speed up/down (+/-0.25x)", "Show lyrics", "Jump to time"},
			[]string{"Seek to N x 10% of track (e.g. 7j = 70%)", "Back to provider", "Focus", "Open or play",
				"Favorite track", "Metadata", "Choose theme"}},
		{"search input", input,
			[]string{"Results (Down)", "Close search", "Clear", "Help"},
			[]string{"Stop", "Volume up/down", "Move (Up/Down)"}},
		{"search results", results,
			[]string{"Edit the query", "Move (Up/Down)", "Stop"},
			[]string{"Search", "Sync"}},
		{"library off", upstream,
			[]string{"Track info / metadata", "Quit"},
			[]string{"Open or play"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := keymapActions(tt.m)
			for _, w := range tt.want {
				if !slices.Contains(got, w) {
					t.Errorf("missing %q in %q", w, got)
				}
			}
			for _, n := range tt.not {
				if slices.Contains(got, n) {
					t.Errorf("lists %q in %q", n, got)
				}
			}
		})
	}
}

// Every cliamp key the gate passes through is listed, and every key of a
// listed cliamp command reaches cliamp.
func TestLibraryKeymapMatchesGate(t *testing.T) {
	m := newLibraryModel(testLibraryRoot())
	listed := make(map[string]bool)
	for _, e := range m.buildKeymapEntries() {
		listed[e.key+"\x00"+e.action] = true
	}
	live := map[string]bool{"ctrl+k": true}
	for k := range libraryPassthroughKeys {
		live[k] = true
	}
	owned := make(map[string]bool)
	for _, row := range libBrowseKeys {
		for _, k := range row.keys {
			owned[k] = true
		}
	}
	covered := make(map[string]bool)
	for _, c := range commandRegistry {
		if !listed[c.KeyLabel+"\x00"+c.label(m)] || slices.ContainsFunc(c.Keys, func(k string) bool { return owned[k] }) {
			continue // not listed, or the library's own row of the same name
		}
		for _, k := range c.Keys {
			if !live[k] {
				t.Errorf("%q (%s) is listed but the library swallows %q", c.Label, c.KeyLabel, k)
			}
			covered[k] = true
		}
	}
	for k := range libraryPassthroughKeys {
		if !covered[k] {
			t.Errorf("passthrough key %q is not listed", k)
		}
	}
}
