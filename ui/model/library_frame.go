package model

// ddmus: the frame's border and the rules between the screen's parts.
//
// The border takes the place of the frame's outer padding (one row above and
// below, one column each side), so the layout loses no row or column to it:
// recomputeLayout's arithmetic is the same with it or without. It is drawn
// from the compact tier up, where the frame has that padding to give; the
// minimal tier keeps its rows for the list.

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bjarneo/cliamp/ui"
)

// SetFrameBorder turns the border and rules on or off ([ddmus] border).
func (m *Model) SetFrameBorder(on bool) {
	m.lib.border = on
	m.recomputeLayout()
}

// libBorderOn reports whether a frame laid out as l draws the border: it is
// on, the library is enabled, and l's tier has padding for it to take.
func (m Model) libBorderOn(l frameLayout) bool {
	return m.lib.border && m.libraryEnabled() && l.paddingV >= 1 && l.paddingH >= 1 &&
		(l.tier == layoutCompact || l.tier == layoutFull)
}

// libFrameStyle is the frame style for m.layout: its padding, with the
// border in the padding's outer cells when there is one. A bordered frame
// is the terminal's height: cliamp's layout reserves a row for a status
// message and lets the frame shrink without one, which would leave a blank
// row under the border.
func (m Model) libFrameStyle() lipgloss.Style {
	l := m.layout
	s := ui.FrameStyle.Border(lipgloss.RoundedBorder(), l.border).BorderForeground(ui.ColorDim).Height(0)
	if l.border {
		return s.Padding(l.paddingV-1, l.paddingH-1).Height(m.height)
	}
	return s.Padding(l.paddingV, l.paddingH)
}

// libSpacerRule is the row between the body and the key bar: a rule with
// the border, else blank. Below the two columns the divider meets it.
func (m Model) libSpacerRule() string {
	if !m.layout.border {
		return ""
	}
	if m.layout.twoColumn {
		half := strings.Repeat("─", columnGutterWidth/2)
		return dimStyle.Render(strings.Repeat("─", m.layout.playlistWidth) + half + "┴" + half +
			strings.Repeat("─", m.layout.settingsWidth))
	}
	return dimStyle.Render(strings.Repeat("─", ui.PanelWidth))
}

// libColumnGutter is the channel between the queue and the settings column:
// a divider down its middle with the border, else blank.
func (m Model) libColumnGutter() string {
	if !m.layout.border {
		return columnGutter
	}
	half := strings.Repeat(" ", columnGutterWidth/2)
	return half + dimStyle.Render("│") + half
}
