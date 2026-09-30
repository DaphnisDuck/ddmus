package model

// ddmus: rendering for the library navigation stack.

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/bjarneo/cliamp/internal/appmeta"
	"github.com/bjarneo/cliamp/library"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/ui"
)

// brandTitle heads every screen (renderTitle in view.go).
var brandTitle = appmeta.DisplayName()

// libRow is one rendered line: an entry, or a section heading (index < 0).
type libRow struct {
	index   int
	section string
}

func libraryRows(entries []library.Entry) []libRow {
	rows := make([]libRow, 0, len(entries))
	last := ""
	for i, e := range entries {
		if e.Section != "" && e.Section != last {
			rows = append(rows, libRow{index: -1, section: e.Section})
		}
		last = e.Section
		rows = append(rows, libRow{index: i})
	}
	return rows
}

// libScroll returns the first visible row so the cursor's row (and the
// heading directly above it) fits in budget rows.
func libScroll(rows []libRow, cursor, scroll, budget int) int {
	if budget <= 0 || len(rows) <= budget {
		return 0
	}
	r := 0
	for i, row := range rows {
		if row.index == cursor {
			r = i
			break
		}
	}
	top := r
	if r > 0 && rows[r-1].index < 0 {
		top = r - 1
	}
	if top < scroll {
		scroll = top
	}
	if r >= scroll+budget {
		scroll = r - budget + 1
	}
	return max(0, min(scroll, len(rows)-budget))
}

func (m *Model) libAdjustScroll() {
	if !m.libraryEnabled() {
		return
	}
	f := m.libTop()
	f.scroll = libScroll(libraryRows(f.entries), f.cursor, f.scroll, m.libListBudget())
}

// libListBudget is how many rows the list gets: the body, less the search
// input line on the search screen.
func (m *Model) libListBudget() int {
	_, budget := m.bodySize()
	if _, ok := m.libSearchLevel(); ok {
		budget--
	}
	return max(budget, 1)
}

func (m Model) libBreadcrumb() string {
	parts := make([]string, len(m.lib.stack))
	for i, f := range m.lib.stack {
		parts[i] = library.CleanText(f.level.Title())
	}
	crumb := strings.Join(parts, " / ")
	// Keep the tail: the current level matters more than the root.
	if limit := ui.PanelWidth - 14; limit > 4 && lipgloss.Width(crumb) > limit {
		for len(parts) > 1 && lipgloss.Width("… / "+strings.Join(parts, " / ")) > limit {
			parts = parts[1:]
		}
		crumb = "… / " + strings.Join(parts, " / ") // sepHeader clips the rest
	}
	return crumb
}

func (m *Model) libHeaderLine() string {
	f := m.libTop()
	label := m.libBreadcrumb()
	if ol, ok := f.level.(library.OrderedLevel); ok {
		label += " · " + ol.OrderName()
	}
	if badge := m.libSyncBadge(); badge != "" {
		label += "  " + badge
	}
	if (f.loading() && len(f.entries) == 0) || f.err != nil {
		return sepHeader(label)
	}
	return sepHeaderN(label, f.cursor+1, len(f.entries))
}

// libSyncBadge is the small catalog sync indicator: ↻ while syncing, the
// age of the last success, or a note that the cached library is showing.
// With several providers, each badge names its provider.
func (m *Model) libSyncBadge() string {
	var badges []string
	for _, provider := range slices.Sorted(maps.Keys(m.lib.sync)) {
		st := m.lib.sync[provider]
		var badge string
		switch {
		case st.running:
			badge = "↻ syncing"
		case st.lastErr != "":
			badge = "sync failed · cached"
		case !st.lastSuccess.IsZero():
			badge = "✓ synced " + syncAge(time.Since(st.lastSuccess))
		default:
			continue
		}
		if len(m.lib.sync) > 1 {
			badge = library.SourceLabel(provider) + " " + badge
		}
		badges = append(badges, badge)
	}
	return strings.Join(badges, " · ")
}

func syncAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	}
}

// libHelpLine is the library overlay's help piece. renderTierHelp draws the
// key bar (library_keymap.go) before reaching it; this keeps the overlay whole.
func (m *Model) libHelpLine() string {
	bar, _ := m.libKeyBar(ui.PanelWidth)
	return bar
}

func (m *Model) renderLibraryBody() string {
	if sl, ok := m.libSearchLevel(); ok && !m.lib.signingIn {
		budget := m.libListBudget()
		body := m.renderLibraryList(budget)
		if sl.Query() == "" {
			lines := make([]string, len(libSearchHint))
			for i, h := range libSearchHint {
				lines[i] = dimStyle.Render(truncate(h, ui.PanelWidth))
			}
			body = bodyLines(lines, budget)
		}
		return m.libSearchPrompt(sl) + "\n" + body
	}
	_, rows := m.bodySize()
	return m.renderLibraryList(rows)
}

// renderLibraryList renders the top frame's rows, or its loading, sign-in
// or error state, in budget lines.
func (m *Model) renderLibraryList(budget int) string {
	f := m.libTop()
	switch {
	case m.lib.signingIn:
		lines := []string{loadingLine("Signing in — finish in your browser…")}
		if m.lib.authURL != "" {
			lines = append(lines, dimStyle.Render("  If no browser opened, visit:"), dimStyle.Render("  "+truncate(m.lib.authURL, ui.PanelWidth-4)))
		}
		return bodyLines(lines, budget)
	case f.loading() && len(f.entries) == 0:
		return bodyLines([]string{loadingLine("Loading " + f.level.Title() + "…")}, budget)
	case errors.Is(f.err, playlist.ErrNeedsAuth):
		return bodyMessage("Sign-in required. Press Enter to sign in.", budget)
	case f.err != nil:
		return bodyLines([]string{
			dimStyle.Render("  " + truncate(f.err.Error(), ui.PanelWidth-4)),
			dimStyle.Render("  Press Enter to retry."),
		}, budget)
	case len(f.entries) == 0:
		return bodyMessage("Nothing here.", budget)
	}

	panel := ui.PanelWidth
	titleCol := 0 // the title column, on a wide terminal
	if panel > listReadWidth {
		titleCol = f.titleCol
	}
	defer ui.WithPanelWidth(listRowWidth(panel, f.column, 0))() // section headings
	rows := libraryRows(f.entries)
	scroll := libScroll(rows, f.cursor, f.scroll, budget)
	numbers := libTrackNumbers(f.entries)
	// While the search input has focus, Enter does not act on a row, so no
	// row shows the cursor.
	_, searching := m.libSearchLevel()
	showCursor := !searching || !m.lib.searchInput
	lines := make([]string, 0, budget)
	for _, row := range rows[scroll:] {
		if len(lines) >= budget {
			break
		}
		if row.index < 0 {
			lines = append(lines, dimStyle.Render(labeledSeparator("", library.CleanText(row.section))))
			continue
		}
		need := 0
		if row.index < len(f.needs) {
			need = f.needs[row.index]
		}
		restore := ui.WithPanelWidth(listRowWidth(panel, f.column, need))
		lines = append(lines, cursorLine(libEntryLabel(f.entries[row.index], numbers[row.index], titleCol),
			showCursor && row.index == f.cursor))
		restore()
	}
	return bodyLines(lines, budget)
}

// libTrackNumbers numbers each section's tracks from 1 (0 for other rows),
// so an album counts its tracks and a search result section its own.
func libTrackNumbers(entries []library.Entry) []int {
	numbers := make([]int, len(entries))
	n := 0
	for i, e := range entries {
		if i > 0 && e.Section != entries[i-1].Section {
			n = 0
		}
		if e.Track != nil && !e.Track.Realtime {
			n++
			numbers[i] = n
		}
	}
	return numbers
}

// libEntryLabel renders "Title      Detail ›"; tracks use the numbered track
// row with duration. A station is a live stream with neither, so it is a
// plain row. Browsable rows end in "›". Up to listReadWidth the detail is
// right-aligned; on a wider terminal (titleCol > 0) it starts after a
// titleCol-wide title column, or after a longer title.
func libEntryLabel(e library.Entry, number, titleCol int) string {
	if e.Track != nil && !e.Track.Realtime {
		return formatTrackRow(number, library.CleanText(trackViewName(*e.Track)), e.Track.DurationSecs)
	}
	width := ui.PanelWidth - libCursorPrefix
	suffix := ""
	if e.Open != nil {
		suffix = " ›"
	}
	title := library.CleanText(e.Title)
	if e.Favorite {
		title = "★ " + title
	}
	avail := width - lipgloss.Width(suffix)
	if d := library.CleanText(e.Detail); titleCol > 0 && d != "" {
		// Wide: the title column, then the detail, which keeps up to half
		// the row when the title is long.
		title = truncate(title, max(avail-2-min(lipgloss.Width(d), avail/2), 4))
		gap := max(titleCol-lipgloss.Width(title), 0) + 2
		return title + strings.Repeat(" ", gap) + truncate(d, max(avail-lipgloss.Width(title)-gap, 0)) + suffix
	}
	detail := truncate(library.CleanText(e.Detail), avail/2)
	titleW := avail
	if detail != "" {
		titleW -= lipgloss.Width(detail) + 2
	}
	title = truncate(title, max(titleW, 4))
	if detail == "" {
		return title + suffix
	}
	pad := max(avail-lipgloss.Width(title)-lipgloss.Width(detail), 2)
	return title + strings.Repeat(" ", pad) + detail + suffix
}
