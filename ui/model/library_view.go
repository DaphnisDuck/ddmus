package model

// omatunes: rendering for the library navigation stack.

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/bjarneo/cliamp/library"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/ui"
)

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
	f.scroll = libScroll(libraryRows(f.entries), f.cursor, f.scroll, m.effectivePlaylistVisible())
}

func (m Model) libBreadcrumb() string {
	parts := make([]string, len(m.lib.stack))
	for i, f := range m.lib.stack {
		parts[i] = f.level.Title()
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
			badge = providerLabel(provider) + " " + badge
		}
		badges = append(badges, badge)
	}
	return strings.Join(badges, " · ")
}

// providerLabel is a catalog provider name for display: "spotify" → "Spotify".
func providerLabel(provider string) string {
	if provider == "" {
		return ""
	}
	return strings.ToUpper(provider[:1]) + provider[1:]
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

func (m *Model) libHelpLine() string {
	parts := []string{helpKey("j/k", "Move"), helpKey("l", "Open")}
	quit := helpKey("q", "Quit")
	if len(m.lib.stack) > 1 {
		parts = append(parts, helpKey("h", "Back"))
		quit = helpKey("q", "Back")
	}
	parts = append(parts, helpKey("/", "Search"), helpKey("Space", "Pause"), helpKey("Tab", "Queue"))
	if m.lib.refresh != nil {
		parts = append(parts, helpKey("r", "Sync"))
	}
	parts = append(parts, quit)
	return fitHelpLine(strings.Join(parts, " "))
}

func (m *Model) renderLibraryBody() string {
	budget := m.effectivePlaylistVisible()
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

	rows := libraryRows(f.entries)
	scroll := libScroll(rows, f.cursor, f.scroll, budget)
	lines := make([]string, 0, budget)
	for _, row := range rows[scroll:] {
		if len(lines) >= budget {
			break
		}
		if row.index < 0 {
			lines = append(lines, dimStyle.Render(labeledSeparator("", row.section)))
			continue
		}
		lines = append(lines, cursorLine(libEntryLabel(f.entries[row.index], row.index), row.index == f.cursor))
	}
	return bodyLines(lines, budget)
}

// libEntryLabel renders "Title      Detail ›"; tracks use the numbered track
// row with duration. Browsable rows end in "›".
func libEntryLabel(e library.Entry, i int) string {
	if e.Track != nil {
		return formatTrackRow(i+1, trackViewName(*e.Track), e.Track.DurationSecs)
	}
	width := ui.PanelWidth - 4 // cursor prefix
	suffix := ""
	if e.Open != nil {
		suffix = " ›"
	}
	title := e.Title
	if e.Favorite {
		title = "★ " + title
	}
	avail := width - lipgloss.Width(suffix)
	detail := truncate(e.Detail, avail/2)
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
