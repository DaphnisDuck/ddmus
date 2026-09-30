package model

// ddmus: the Ctrl+K / ? overlay while the library owns the main screen.
// It lists the library's own keys and only those cliamp commands whose keys
// the gate passes through, so no swallowed key is advertised.

import (
	"slices"

	"github.com/bjarneo/cliamp/library"
)

// libKeyHelp is one row of the library's own keys.
type libKeyHelp struct {
	keys     []string // KeyPressMsg.String values handled for this row
	keyLabel string
	label    string
}

var (
	libBrowseKeys = []libKeyHelp{
		{[]string{"up", "k", "down", "j"}, "j/k", "Move (Up/Down)"},
		{[]string{"g", "home", "G", "end"}, "g/G", "Top / Bottom (Home/End)"},
		{[]string{"pgup", "ctrl+u", "pgdown", "ctrl+d"}, "PgUp/PgDn", "Page up / down (Ctrl+U/D)"},
		{[]string{"enter", "l", "right"}, "Enter/l", "Open or play"},
		{[]string{"esc", "h", "left", "backspace"}, "Esc/h", "Back (Backspace)"},
		{[]string{"q"}, "q", "Back; quit at the top"},
		{[]string{"/"}, "/", "Search"},
		{[]string{"space"}, "Space", "Play / Pause"},
		{[]string{"tab"}, "Tab", "Queue"},
	}
	libOrderKey   = libKeyHelp{[]string{"o"}, "o", "Change order"}
	libRefreshKey = libKeyHelp{[]string{"r"}, "r", "Sync"}

	// In search results, the back keys return to the query instead.
	libResultKeys = []libKeyHelp{
		{[]string{"up", "k", "down", "j"}, "j/k", "Move (Up/Down)"},
		{[]string{"g", "home", "G", "end"}, "g/G", "Top / Bottom (Home/End)"},
		{[]string{"pgup", "ctrl+u", "pgdown", "ctrl+d"}, "PgUp/PgDn", "Page up / down (Ctrl+U/D)"},
		{[]string{"enter", "l", "right"}, "Enter/l", "Open or play"},
		{[]string{"/", "esc", "h", "left", "backspace"}, "/ Esc h", "Edit the query"},
		{[]string{"q"}, "q", "Back"},
		{[]string{"space"}, "Space", "Play / Pause"},
		{[]string{"tab"}, "Tab", "Queue"},
	}
	libSearchInputKeys = []libKeyHelp{
		{[]string{"enter", "down", "tab"}, "Enter/Tab", "Results (Down)"},
		{[]string{"esc"}, "Esc", "Close search"},
		{[]string{"ctrl+u"}, "Ctrl+U", "Clear"},
	}
	libQueueKeys = []libKeyHelp{
		{[]string{"tab", "esc", "b"}, "Tab/Esc", "Library"},
	}
)

// libKeymapSection is the current context's own keys plus the cliamp
// commands reachable through pass, as the overlay's first section.
type libKeymapSection struct {
	title string
	own   []libKeyHelp
	pass  map[string]bool // cliamp keys live in this context
}

// libraryKeymapSections describes what the library does with keys right now:
// the current context, then the player keys it passes through.
func (m Model) libraryKeymapSections() []libKeymapSection {
	player := libKeymapSection{title: "player", pass: map[string]bool{"ctrl+k": true}} // Ctrl+K opens before the gate
	_, searching := m.libSearchLevel()
	if !searching || !m.lib.searchInput { // the query input takes every other key as text
		for k := range libraryPassthroughKeys {
			player.pass[k] = true
		}
	}
	var current libKeymapSection
	switch {
	case !m.lib.visible:
		current = libKeymapSection{title: "Queue", own: libQueueKeys, pass: queuePassthroughKeys}
	case searching && m.lib.searchInput:
		current = libKeymapSection{title: "Library Search", own: libSearchInputKeys}
	case searching:
		current = libKeymapSection{title: "Search Results", own: libResultKeys}
	default:
		own := slices.Clip(libBrowseKeys)
		if _, ok := m.libTop().level.(library.OrderedLevel); ok {
			own = append(own, libOrderKey)
		}
		if m.lib.refresh != nil {
			own = append(own, libRefreshKey)
		}
		current = libKeymapSection{title: "Library", own: own}
	}
	return []libKeymapSection{current, player}
}

// libraryKeymapEntries builds the overlay rows from libraryKeymapSections.
// A cliamp command is listed when every one of its keys is live in the
// section, so a command the gate breaks in part (Nj needs digits) is not.
func (m Model) libraryKeymapEntries() []keymapEntry {
	var out []keymapEntry
	seen := make(map[string]bool)
	for i, s := range m.libraryKeymapSections() {
		title := s.title
		if i == 0 {
			title = "current: " + title
		}
		out = append(out, keymapEntry{action: "— " + title + " —", divider: true})
		for _, row := range s.own {
			out = append(out, keymapEntry{key: row.keyLabel, action: row.label})
		}
		for _, command := range commandRegistry {
			if !command.Keymap || !command.enabled(m) || command.Mode&commandModeMain == 0 ||
				!allKeys(command.Keys, s.pass) {
				continue
			}
			label := command.label(m)
			if id := command.KeyLabel + "\x00" + label; !seen[id] {
				seen[id] = true
				out = append(out, keymapEntry{key: command.KeyLabel, action: label})
			}
		}
	}
	return out
}

func allKeys(keys []string, live map[string]bool) bool {
	return len(keys) > 0 && !slices.ContainsFunc(keys, func(k string) bool { return !live[k] })
}
