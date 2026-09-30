package model

// ddmus: the library's views fill the terminal (M9). More rows in the
// terminal list more entries, the frame is exactly the terminal's size, and
// resizing keeps the cursor on screen.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bjarneo/cliamp/library"
	"github.com/bjarneo/cliamp/lyrics"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/ui"
)

const layoutRows = 300

// bigLibraryRoot has Albums (layoutRows albums, with details) and an album
// of layoutRows tracks.
func bigLibraryRoot() library.Level {
	albums := make([]library.Entry, layoutRows)
	for i := range albums {
		albums[i] = library.Entry{Title: fmt.Sprintf("Album %03d", i), Detail: "Some Artist · 1999", Open: library.Menu("inside")}
	}
	tracks := make([]playlist.Track, layoutRows)
	for i := range tracks {
		tracks[i] = playlist.Track{Path: fmt.Sprintf("t%03d", i), Title: fmt.Sprintf("Track %03d", i), DurationSecs: 200}
	}
	long := library.TrackLevel("Long Album", nil, func(context.Context) ([]playlist.Track, error) { return tracks, nil })
	return library.Menu("Music",
		library.Entry{Title: "Albums", Open: library.Menu("Albums", albums...)},
		library.Entry{Title: "Long Album", Open: long},
	)
}

func resized(t *testing.T, m Model, w, h int) Model {
	t.Helper()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return updated.(Model)
}

// countRows counts the view's lines containing marker.
func countRows(m Model, marker string) int {
	return strings.Count(stripAnsi(m.View().Content), marker)
}

// checkFrame fails unless the view is exactly w×h: no line wider, and no
// fewer lines (a shorter frame is centered with blank bands around it). The
// frame is measured before View fits it to the terminal too, so a frame a
// row too tall, whose last row View would cut, fails.
func checkFrame(t *testing.T, m Model, w, h int) {
	t.Helper()
	f := m
	f.recomputeLayout()
	content := strings.Join(f.mainSections(f.renderBodyRegion(), true, f.usesContentFirstLayout()), "\n")
	if got := lipgloss.Height(ui.FrameStyle.Render(content)); got != h {
		t.Errorf("frame height = %d before fitting, want %d", got, h)
	}
	out := m.View().Content
	if got := lipgloss.Height(out); got != h {
		t.Errorf("view height = %d, want %d", got, h)
	}
	for _, line := range strings.Split(out, "\n") {
		if got := lipgloss.Width(line); got > w {
			t.Fatalf("line width = %d, want <= %d: %q", got, w, stripAnsi(line))
		}
	}
	if lines := strings.Split(stripAnsi(out), "\n"); strings.TrimSpace(lines[0]) == "" && strings.TrimSpace(lines[1]) == "" {
		t.Errorf("the frame starts with blank rows: centered, not filling")
	}
}

// layoutScreens builds each library screen at w×h with a long list in front.
func layoutScreens(t *testing.T, w, h int) map[string]struct {
	m      Model
	marker string
} {
	return layoutScreensWith(t, w, h, frameOpts{vis: true})
}

// frameOpts are the frame settings layoutScreensWith builds with.
type frameOpts struct{ border, vis bool }

// layoutScreensWith is layoutScreens with the border and visualizer on or
// off.
func layoutScreensWith(t *testing.T, w, h int, o frameOpts) map[string]struct {
	m      Model
	marker string // text on each list row
} {
	t.Helper()
	bordered := func(m Model) Model {
		if o.vis {
			m.vis = ui.NewVisualizer(44100)
		}
		m.SetFrameBorder(o.border)
		return m
	}
	albums := resized(t, bordered(newLibraryModel(bigLibraryRoot())), w, h)
	albums = libPress(t, albums, "enter")

	queueAt := func() Model {
		q := resized(t, bordered(newLibraryModel(bigLibraryRoot())), w, h)
		for _, k := range []string{"j", "enter", "enter", "tab"} {
			q = libPress(t, q, k)
		}
		return q
	}
	queue := queueAt()

	lyr := queue
	lyr.lyrics.visible = true
	lyr.lyrics.lines = make([]lyrics.Line, layoutRows)
	for i := range lyr.lyrics.lines {
		lyr.lyrics.lines[i] = lyrics.Line{Text: fmt.Sprintf("Lyric line %03d", i)}
	}
	lyr.recomputeLayout() // the lyrics screen has its own chrome

	info := queuePress(queue, "i")

	upNext := queueAt() // its own playlist: queuing marks the tracks
	for range 150 {
		upNext = queuePress(queuePress(upNext, "a"), "j")
	}
	upNext = queuePress(upNext, "A")

	search, _ := newSearchModel(t)
	search = resized(t, bordered(search), w, h)

	return map[string]struct {
		m      Model
		marker string
	}{
		"Albums":  {albums, "Album "},
		"Queue":   {queue, "Track "},
		"Lyrics":  {lyr, "Lyric line "},
		"Search":  {search, ""},
		"Info":    {info, ""},
		"Up next": {upNext, "Track "},
	}
}

// Every library screen fills the terminal at every size, and its list takes
// every row the chrome leaves.
func TestLibraryScreensFillTheTerminal(t *testing.T) {
	// Without the border or a visualizer, cliamp's layout lets a status line
	// push the frame's blank bottom row off (layout.go); that is not tested.
	for _, o := range []frameOpts{{vis: true}, {border: true, vis: true}, {border: true}} {
		for _, size := range []struct{ w, h int }{{40, 12}, {56, 16}, {80, 24}, {120, 40}, {200, 60}, {300, 50}, {100, 120}} {
			for name, s := range layoutScreensWith(t, size.w, size.h, o) {
				t.Run(fmt.Sprintf("%+v/%dx%d/%s", o, size.w, size.h, name), func(t *testing.T) {
					checkFrame(t, s.m, size.w, size.h)
					if _, rows := s.m.bodySize(); rows != s.m.layout.bodyRows {
						t.Errorf("list rows = %d, body = %d: the list is capped", rows, s.m.layout.bodyRows)
					}
					if s.marker != "" {
						_, rows := s.m.bodySize()
						if got := countRows(s.m, s.marker); got < rows-1 {
							t.Errorf("%d rows listed, want about %d", got, rows)
						}
					}
				})
			}
		}
	}
}

// A taller terminal lists more: the rows grow with the height, one for one.
func TestTallerTerminalListsMore(t *testing.T) {
	for _, screen := range []string{"Albums", "Queue", "Lyrics"} {
		var prev, prevH int
		for _, h := range []int{40, 60, 120} { // below 40 the visualizer yields rows too
			s := layoutScreens(t, 120, h)[screen]
			got := countRows(s.m, s.marker)
			if prevH > 0 && got-prev != h-prevH {
				t.Errorf("%s: %d rows at height %d, %d at %d; want %d more", screen, prev, prevH, got, h, h-prevH)
			}
			prev, prevH = got, h
		}
	}
}

// Resizing down and back up keeps the cursor's row on screen, and the list
// grows back to the taller terminal.
func TestResizeKeepsTheCursorOnScreen(t *testing.T) {
	m := layoutScreens(t, 120, 60)["Albums"].m
	for range 250 {
		m = libPress(t, m, "j")
	}
	for _, size := range []struct{ w, h int }{{80, 24}, {56, 16}, {200, 80}, {120, 60}} {
		m = resized(t, m, size.w, size.h)
		if out := stripAnsi(m.View().Content); !strings.Contains(out, "> Album 250") {
			t.Fatalf("%dx%d: the cursor's row is off screen:\n%s", size.w, size.h, out)
		}
		checkFrame(t, m, size.w, size.h)
		if f := m.libTop(); f.scroll > f.cursor {
			t.Errorf("%dx%d: stored scroll %d is past the cursor %d", size.w, size.h, f.scroll, f.cursor)
		}
	}
	if got := countRows(m, "Album "); got < 40 {
		t.Errorf("back at 120x60: %d rows, want the list grown back", got)
	}
}

// Up to listReadWidth, rows fill the width as before. Past it, the details
// line up at the column most rows fit in, and a longer row runs past it
// whole instead of being cut.
func TestWideRowsEndAtTheirContent(t *testing.T) {
	longTitle := strings.TrimSpace(strings.Repeat("Long Title ", 14)) // 153 cells
	entries := []library.Entry{{Title: longTitle, Detail: "Artist · 1999", Open: library.Menu("y")}}
	for i := range 20 {
		entries = append(entries, library.Entry{Title: fmt.Sprintf("Short %02d", i), Detail: "Artist · 1999", Open: library.Menu("x")})
	}
	root := library.Menu("Music", entries...)
	rowEnd := func(m Model, marker string) int {
		for _, l := range strings.Split(stripAnsi(m.View().Content), "\n") {
			if strings.Contains(l, marker) {
				return lipgloss.Width(strings.TrimRight(l, " "))
			}
		}
		t.Fatalf("no row with %q", marker)
		return 0
	}
	// Rows end two cells short of their width: libEntryLabel reserves the
	// libCursorPrefix, and cursorLine draws two of its cells.
	const pad, short = 3, 2 // the frame's left padding
	for _, tt := range []struct{ w, short, long int }{
		{80, 80 - pad - short, 80 - pad - short},
		{100, 100 - pad - short, 100 - pad - short},
		// Wide: the details start after the title column (9 of 10 titles are
		// "Short NN"), and the long title pushes its own detail right.
		{300, pad + 2 + len("Short 05") + 2 + lipgloss.Width("Artist · 1999") + 2, pad + 2 + lipgloss.Width(longTitle) + 2 + lipgloss.Width("Artist · 1999") + 2},
	} {
		m := resized(t, newLibraryModel(root), tt.w, 40)
		if got := rowEnd(m, "Short 05"); got != tt.short {
			t.Errorf("width %d: a short row ends at %d, want %d", tt.w, got, tt.short)
		}
		if got := rowEnd(m, "Long Title Long"); got != tt.long {
			t.Errorf("width %d: the long row ends at %d, want %d", tt.w, got, tt.long)
		}
	}
	m := resized(t, newLibraryModel(root), 300, 40)
	out := stripAnsi(m.View().Content)
	if !strings.Contains(out, longTitle) {
		t.Error("the long title is cut at 300 columns")
	}
	// The details line up in one column.
	col := -1
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "Short") {
			at := strings.Index(l, "Artist")
			if col >= 0 && at != col {
				t.Fatalf("details at %d and %d: not one column", col, at)
			}
			col = at
		}
	}
}

func TestListRowWidth(t *testing.T) {
	for _, tt := range []struct{ panel, column, need, want int }{
		{80, 30, 300, 80},    // up to listReadWidth rows fill the panel
		{100, 300, 300, 100}, // ...
		{294, 30, 30, 100},   // never narrower than listReadWidth
		{294, 150, 40, 150},  // a short row ends at the column
		{294, 150, 200, 200}, // a long one runs past it
		{294, 150, 400, 294}, // within the panel
	} {
		if got := listRowWidth(tt.panel, tt.column, tt.need); got != tt.want {
			t.Errorf("listRowWidth(%d, %d, %d) = %d, want %d", tt.panel, tt.column, tt.need, got, tt.want)
		}
	}
	needs := make([]int, 100)
	for i := range needs {
		needs[i] = i + 1
	}
	if got := listColumn(needs); got != 90 {
		t.Errorf("listColumn(1..100) = %d, want 90", got)
	}
}

// The queue's durations stay beside its titles on a wide terminal, and Up
// next's too, while the settings column keeps its place.
func TestWideQueueRowsEndAtTheirContent(t *testing.T) {
	screens := layoutScreens(t, 300, 30)
	for _, name := range []string{"Queue", "Up next"} {
		for _, l := range strings.Split(stripAnsi(screens[name].m.View().Content), "\n") {
			if !strings.Contains(l, "Track 010") {
				continue
			}
			if at := strings.Index(l, "3:20"); at < 0 || at > 3+listReadWidth {
				t.Errorf("%s: the duration is at column %d, want within %d:\n%s", name, at, 3+listReadWidth, l)
			}
		}
	}
	if !strings.Contains(stripAnsi(screens["Queue"].m.View().Content), "Settings") {
		t.Error("the queue lost its settings column")
	}
}

// On a short terminal the queue's visualizer gives up rows for the list, and
// keeps one; on a tall one it keeps its height.
func TestShortQueueVisualizerYields(t *testing.T) {
	for _, tt := range []struct{ w, h, minRows int }{{80, 24, libQueueMinRows}, {56, 16, libMinBodyRows}, {120, 60, 30}} {
		m := resized(t, newLibraryModel(bigLibraryRoot()), tt.w, tt.h)
		m.vis = ui.NewVisualizer(44100)
		for _, k := range []string{"j", "enter", "enter", "tab"} {
			m = libPress(t, m, k)
		}
		m.recomputeLayout()
		if _, rows := m.bodySize(); rows < tt.minRows {
			t.Errorf("%dx%d: %d list rows, want at least %d", tt.w, tt.h, rows, tt.minRows)
		}
		if m.layout.visualizerRows < 1 {
			t.Errorf("%dx%d: the visualizer is gone", tt.w, tt.h)
		}
		if tt.h >= 60 && m.layout.visualizerRows != m.layout.baseVisualizerRows {
			t.Errorf("%dx%d: the visualizer shrank to %d with room to spare", tt.w, tt.h, m.layout.visualizerRows)
		}
		checkFrame(t, m, tt.w, tt.h)
	}
}

// With the border on, the frame draws it from the compact tier up, a rule
// above the key bar, and a divider between the queue and its settings that
// meets the rule; off, none of them.
func TestFrameBorderAndRules(t *testing.T) {
	lines := func(m Model) []string { return strings.Split(stripAnsi(m.View().Content), "\n") }
	for _, size := range []struct{ w, h int }{{56, 16}, {80, 24}, {200, 60}} {
		s := layoutScreensWith(t, size.w, size.h, frameOpts{border: true, vis: true})
		for _, name := range []string{"Albums", "Queue", "Info"} {
			l := lines(s[name].m)
			if !strings.HasPrefix(l[0], "╭") || !strings.HasPrefix(l[len(l)-1], "╰") || !strings.HasPrefix(l[1], "│") {
				t.Errorf("%dx%d %s: no border:\n%s", size.w, size.h, name, strings.Join(l, "\n"))
			}
		}
		if size.w >= 80 {
			album := strings.Join(lines(s["Albums"].m), "\n")
			if !strings.Contains(album, "│  "+strings.Repeat("─", 20)) {
				t.Errorf("%dx%d: no rule above the key bar:\n%s", size.w, size.h, album)
			}
			queue := strings.Join(lines(s["Queue"].m), "\n")
			if !strings.Contains(queue, "  │  SRC") || !strings.Contains(queue, "──┴──") {
				t.Errorf("%dx%d: no column divider meeting the rule:\n%s", size.w, size.h, queue)
			}
		}
	}
	// Without a status message cliamp's frame is a row shorter; bordered, it
	// still reaches the terminal's last row.
	quiet := layoutScreensWith(t, 80, 24, frameOpts{border: true, vis: true})["Queue"].m
	quiet.status.text = ""
	if l := lines(quiet); len(l) != 24 || !strings.HasPrefix(l[23], "╰") {
		t.Errorf("without a status the bordered queue is %d rows, last %q", len(l), l[len(l)-1])
	}
	small := layoutScreensWith(t, 40, 12, frameOpts{border: true, vis: true})["Albums"].m
	if l := lines(small); strings.ContainsAny(l[0]+l[len(l)-1], "╭╰") {
		t.Error("the minimal tier draws the border")
	}
	off := layoutScreensWith(t, 120, 40, frameOpts{vis: true})
	for _, name := range []string{"Albums", "Queue"} {
		if out := stripAnsi(off[name].m.View().Content); strings.ContainsAny(out, "╭╰┴") || strings.Contains(out, "  │  ") {
			t.Errorf("%s with the border off draws it", name)
		}
	}
}
