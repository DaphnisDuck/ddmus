package model

// omatunes: the library's search screen. It is a frame on the library stack
// whose level is a library.SearchLevel: typing swaps in the level for the
// new query and reloads it after a short pause, so the usual load machinery
// supersedes stale results. Results open like any library row.

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/library"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/provider"
	"github.com/bjarneo/cliamp/ui"
)

// librarySearchDebounce lets key repeat and fast typing settle before a
// query runs; each query is local and takes a few milliseconds.
const librarySearchDebounce = 60 * time.Millisecond

// librarySearchField names the search input for the shared text editor.
const librarySearchField = "lib-search"

// librarySearchTickMsg runs the query typed before it, unless more typing
// has superseded it.
type librarySearchTickMsg struct{ gen uint64 }

// libSearchLevel returns the top frame's search level, if it is one.
func (m *Model) libSearchLevel() (library.SearchLevel, bool) {
	if !m.libraryEnabled() {
		return nil, false
	}
	sl, ok := m.libTop().level.(library.SearchLevel)
	return sl, ok
}

// librarySearch opens search from anywhere in the library, with the input
// focused. It returns to a search already on the stack rather than stacking
// another; without a catalog it opens the provider's own search.
func (m *Model) librarySearch() tea.Cmd {
	for i, f := range m.lib.stack {
		if _, ok := f.level.(library.SearchLevel); ok {
			for len(m.lib.stack) > i+1 {
				m.libraryPop()
			}
			m.lib.searchInput = true
			return nil
		}
	}
	for _, e := range m.lib.stack[0].entries {
		if sl, ok := e.Open.(library.SearchLevel); ok {
			return m.libraryOpenSearch(sl)
		}
		if e.Intent == library.IntentSearch {
			m.openProviderSearchWith(e.Provider)
			return nil
		}
	}
	return nil
}

// libraryOpenSearch pushes a search frame showing the last query's results.
func (m *Model) libraryOpenSearch(sl library.SearchLevel) tea.Cmd {
	m.lib.searchInput = true
	level := sl.WithQuery(m.lib.lastQuery)
	if m.lib.lastQuery == "" {
		m.lib.stack = append(m.lib.stack, libFrame{level: level})
		return nil
	}
	return m.libraryPush(level)
}

// handleLibrarySearchInput edits the query while the input has focus.
func (m *Model) handleLibrarySearchInput(msg tea.KeyPressMsg, sl library.SearchLevel) tea.Cmd {
	f := m.libTop()
	switch msg.String() {
	case "ctrl+c":
		return m.quit()
	case "esc":
		m.lib.searchInput = false
		return m.libraryPop()
	case "enter", "down", "tab":
		if len(f.entries) > 0 {
			m.lib.searchInput = false
			f.cursor, f.scroll = 0, 0
			m.libAdjustScroll()
		}
		return nil
	}
	query := sl.Query()
	if !m.editText(librarySearchField, &query, msg) || query == sl.Query() {
		return nil
	}
	m.lib.lastQuery = query
	f.level = sl.WithQuery(query)
	if f.cancel != nil {
		f.cancel()
		f.cancel = nil
	}
	f.gen = nextRequest(&m.lib.gen) // drops a load of the old query
	if query == "" {
		f.entries, f.err, f.cursor, f.scroll = nil, nil, 0, 0
		return nil
	}
	gen := nextRequest(&m.lib.searchGen)
	return tea.Tick(librarySearchDebounce, func(time.Time) tea.Msg { return librarySearchTickMsg{gen: gen} })
}

// handleLibrarySearchTick runs the settled query.
func (m *Model) handleLibrarySearchTick(msg librarySearchTickMsg) tea.Cmd {
	if msg.gen != m.lib.searchGen {
		return nil
	}
	if _, ok := m.libSearchLevel(); !ok {
		return nil
	}
	f := m.libTop()
	f.cursor, f.scroll = 0, 0
	return m.libraryLoad()
}

// libraryPlayFrom plays what a row's PlayFrom resolves (a searched track's
// album), from the index it names.
func (m *Model) libraryPlayFrom(e library.Entry) tea.Cmd {
	gen := nextRequest(&m.lib.gen)
	m.lib.playGen = gen
	playFrom, title := e.PlayFrom, e.Title
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), libraryLoadTimeout)
		defer cancel()
		tracks, index, err := playFrom(ctx)
		return libraryPlayMsg{gen: gen, title: title, tracks: tracks, index: index, err: err}
	}
}

// openProviderSearchQuery opens the provider's own search already running
// query, for search's "Search Spotify for …" row.
func (m *Model) openProviderSearchQuery(prov playlist.Provider, query string) tea.Cmd {
	m.openProviderSearchWith(prov)
	s, ok := prov.(provider.Searcher)
	if !ok || !m.spotSearch.visible || query == "" {
		return nil
	}
	m.spotSearch.query = query
	m.spotSearch.loading = true
	return fetchSpotSearchCmd(m.newSpotRequestContext(30*time.Second), s, prov.Name(), query, nextRequest(&m.requests.spotSearch))
}

// libSearchPrompt is the search frame's input line: highlighted while it
// has focus, dim once the results do.
func (m *Model) libSearchPrompt(sl library.SearchLevel) string {
	if m.lib.searchInput {
		return m.promptHeader(librarySearchField, "Search", sl.Query())
	}
	return dimStyle.Render(truncate("  Search: "+sl.Query(), ui.PanelWidth))
}

// libSearchHint is shown under an empty query.
var libSearchHint = []string{
	"  Type to search Spotify, Local and your radio stations.",
	"  artist:  album:  title:  genre:  source:spotify|local|radio",
	"  type:artist|album|track|playlist|station   \"exact phrase\"",
}
