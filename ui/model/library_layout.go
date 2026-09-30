package model

// ddmus: the body's size for the library's views.
//
// recomputeLayout (layout.go) counts the chrome for the terminal size and
// the screen, and leaves the rest to the body. With the library enabled the
// lists take all of it: more rows in the terminal mean more rows listed, and
// the frame fills the terminal instead of centering in it. Views size
// themselves from bodySize rather than from ui.PanelWidth and row counts of
// their own, so a detail view (track info) can lay itself out for the room
// it has without another layout change.

import (
	"fmt"
	"slices"

	"charm.land/lipgloss/v2"

	"github.com/bjarneo/cliamp/library"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/ui"
)

// bodySize returns the width and rows of the body region below the chrome:
// the list column beside the settings pane in the two-column playback
// screen, else the frame's inner width. Rows are zero while the terminal is
// too small.
func (m Model) bodySize() (width, rows int) {
	width = ui.PanelWidth
	if m.layout.twoColumn {
		width = m.layout.playlistWidth
	}
	return width, m.effectivePlaylistVisible()
}

// libQueueMinRows is the list a playback screen (the queue, or track info,
// lyrics or Up next over it) keeps on a short terminal: the visualizer
// gives up rows for it first (libVisualizerYield), then the queue's key bar
// (libFitKeyBar).
const libQueueMinRows = 8

// libVisualizerYield returns how many of visRows visualizer rows a playback
// screen gives up so its list keeps libQueueMinRows rows (below the queue's
// whole key bar), in a body of body rows before the bar. The visualizer
// keeps one row.
func (m *Model) libVisualizerYield(width, body, visRows int) int {
	if visRows <= 1 || !m.libraryEnabled() {
		return 0
	}
	bar := 0
	if !m.hideHelpBar && m.libraryOnScreen() {
		bar = max(0, len(m.libKeyBarLines(width))-1) // the rows beyond the one counted
	}
	return min(max(0, libQueueMinRows+bar-body), visRows-1)
}

// listReadWidth is how wide list rows grow with the terminal before they
// stop at their content: up to it, rows fill the width as they always have.
// Past it, a list's detail or duration column sits where most of its rows
// fit (listColumn), so a title and its detail stay within reach of the eye
// instead of a screen apart; a longer row runs past the column as far as it
// needs, so it is not cut short either.
const listReadWidth = 100

// listColumnShare is the share of a list's rows that fit in its column; the
// rest run past it.
const listColumnShare = 0.9

// listRowWidth returns the width one row fills in panel columns: the
// list's column, or the row's own need when longer, within the panel.
func listRowWidth(panel, column, need int) int {
	if panel <= listReadWidth {
		return panel
	}
	return min(panel, max(listReadWidth, column, need))
}

// listColumn returns the width that listColumnShare of needs fit in.
func listColumn(needs []int) int {
	if len(needs) == 0 {
		return 0
	}
	sorted := slices.Sorted(slices.Values(needs))
	return sorted[int(float64(len(sorted)-1)*listColumnShare)]
}

// libCursorPrefix is the "> " cursor and its gap before each library row.
const libCursorPrefix = 4

// libEntryTitleWidth is the width of entry's title as its row shows it.
func libEntryTitleWidth(e library.Entry) int {
	w := lipgloss.Width(library.CleanText(e.Title))
	if e.Favorite {
		w += 2 // "★ "
	}
	return w
}

// libIsTrackRow reports whether entry renders as a numbered track row.
func libIsTrackRow(e library.Entry) bool { return e.Track != nil && !e.Track.Realtime }

// libEntryNeed is the width entry's row needs, laid out as libEntryLabel
// lays it out on a wide terminal (titles in a titleCol column); number is
// its track number.
func libEntryNeed(e library.Entry, number, titleCol int) int {
	if libIsTrackRow(e) {
		return libCursorPrefix + lipgloss.Width(fmt.Sprintf("%d. ", number)) + trackRowNeed(*e.Track, false)
	}
	w := max(libEntryTitleWidth(e), titleCol)
	if d := library.CleanText(e.Detail); d != "" {
		w += 2 + lipgloss.Width(d)
	}
	if e.Open != nil {
		w += 2 // " ›"
	}
	return libCursorPrefix + w
}

// setEntries sets the frame's rows and measures them for listRowWidth: the
// title column of its browsable rows, each row's need, and the list's
// column.
func (f *libFrame) setEntries(entries []library.Entry) {
	f.entries = entries
	var titles []int
	for _, e := range entries {
		if !libIsTrackRow(e) && e.Detail != "" {
			titles = append(titles, libEntryTitleWidth(e))
		}
	}
	f.titleCol = listColumn(titles)
	numbers := libTrackNumbers(entries)
	f.needs = make([]int, len(entries))
	for i, e := range entries {
		f.needs[i] = libEntryNeed(e, numbers[i], f.titleCol)
	}
	f.column = listColumn(f.needs)
}

// trackRowNeed is the width of a track row after its prefix: the name, the
// album suffix when there are no album headers, and the duration.
func trackRowNeed(t playlist.Track, albumSuffix bool) int {
	w := lipgloss.Width(trackViewName(t))
	if albumSuffix && t.Album != "" {
		w += 3 + lipgloss.Width(t.Album) // " · album"
	}
	if t.Unplayable {
		w += len(" (unavailable)")
	}
	if d := formatTrackTime(t.DurationSecs); d != "" {
		w += 1 + lipgloss.Width(d)
	}
	return w
}

// trackColumn is listColumn over tracks' rows.
func trackColumn(tracks []playlist.Track, albumSuffix bool) int {
	needs := make([]int, len(tracks))
	for i, t := range tracks {
		needs[i] = trackRowNeed(t, albumSuffix)
	}
	return listColumn(needs)
}

// queueFit caches the queue's column, which needs a pass over every track,
// until the playlist changes.
type queueFit struct {
	revision uint64
	headers  bool
	column   int
}

// libQueueRowWidth is the width track t's row fills in the queue
// (renderPlaylist), after a prefix of markers and track number.
func (m Model) libQueueRowWidth(t playlist.Track, prefix int) int {
	if !m.libraryEnabled() || m.lib.queueFit == nil || ui.PanelWidth <= listReadWidth {
		return ui.PanelWidth
	}
	c := m.lib.queueFit
	if rev := m.playlist.Revision(); c.revision != rev || c.headers != m.showAlbumHeaders || c.column == 0 {
		*c = queueFit{revision: rev, headers: m.showAlbumHeaders, column: trackColumn(m.playlist.Tracks(), !m.showAlbumHeaders)}
	}
	extra := prefix
	if n := m.playlist.QueueLen(); n > 0 {
		extra += len(fmt.Sprintf(" [Q%d]", n))
	}
	return listRowWidth(ui.PanelWidth, extra+c.column, extra+trackRowNeed(t, !m.showAlbumHeaders))
}

// libUpNextRowWidth is the width track t's row fills in Up next
// (renderQueueBody), whose column is column.
func (m Model) libUpNextRowWidth(t playlist.Track, prefix, column int) int {
	if !m.libraryEnabled() || ui.PanelWidth <= listReadWidth {
		return ui.PanelWidth
	}
	return listRowWidth(ui.PanelWidth, prefix+column, prefix+trackRowNeed(t, false))
}

// width is the cells renderPlaylist's markers take: cursor, state, each
// reserved column, and a space.
func (c markerColumns) width() int {
	w := 3
	for _, on := range []bool{c.queue, c.bookmark, c.favorite, c.played} {
		if on {
			w++
		}
	}
	return w
}
