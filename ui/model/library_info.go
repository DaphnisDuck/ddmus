package model

// ddmus: album artwork in the track info view (i in the queue).
//
// The artwork package finds and loads a track's artwork; kittyimg draws it.
// This file lays the info view out with the artwork beside the metadata and
// runs the load: opening the view asks for the selected track's artwork in
// the background, the result is kept for the session under its key, and the
// view shows the image kept under the selected track's key, so a late
// result can never land on another track. One hook at the end of Update
// sends the image to the terminal, gives it a new size when the view's room
// changes, and frees it before ddmus quits. Without an image-capable
// terminal, or with [ddmus] artwork = false, lib.art is nil and none of
// this runs.

import (
	"context"
	"errors"
	"image"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/bjarneo/cliamp/applog"
	"github.com/bjarneo/cliamp/artwork"
	"github.com/bjarneo/cliamp/ui/kittyimg"
)

// artLoadTimeout bounds one artwork load, a download included.
const artLoadTimeout = 30 * time.Second

// Info view layout: the artwork on the left, the metadata on the right.
const (
	// artMetaMinWidth is the metadata's narrowest column beside the
	// artwork; narrower, and the artwork is left out.
	artMetaMinWidth = 32
	// artMinRows is the smallest artwork shown, in rows.
	artMinRows = 8
	// artIndent and artGap are the blank cells left of and right of it.
	artIndent, artGap = 2, 1
)

// libArt is the artwork state, shared by the model's copies.
type libArt struct {
	id     int // the terminal's image id: 1–255, a 256-color foreground
	load   func(context.Context, artwork.Ref) (image.Image, error)
	images map[string]*libArtImage // by Ref.Key; nil: the track has none
	busy   map[string]bool         // loads in flight

	// The terminal's cell size in pixels, once it answers; 1×2 until then.
	cellW, cellH int
	queried      bool

	// The image the terminal holds, and its placement.
	sent       string
	cols, rows int
}

// libArtImage is a loaded artwork and the sequence that sends it to the
// terminal, built with the load, off the UI goroutine.
type libArtImage struct {
	img  image.Image
	send string
}

// artworkLoadedMsg is a finished load of the artwork under key.
type artworkLoadedMsg struct {
	key string
	art *libArtImage
	err error
}

// SetArtwork turns artwork on in the info view, loaded with load; nil turns
// it off. Call it after SetLibrary.
func (m *Model) SetArtwork(load func(context.Context, artwork.Ref) (image.Image, error)) {
	if load == nil {
		m.lib.art = nil
		return
	}
	// The id is per process, so two ddmus in one terminal rarely share it.
	m.lib.art = &libArt{id: 1 + os.Getpid()%255, load: load, images: map[string]*libArtImage{}, busy: map[string]bool{}}
}

// ArtworkCleanup returns what frees the image ddmus left with the terminal,
// or "". main writes it after the program ends, for the ways out (a signal)
// that never reach Update's quit.
func (m Model) ArtworkCleanup() string {
	if a := m.lib.art; a != nil && a.sent != "" {
		return kittyimg.Delete(a.id)
	}
	return ""
}

// libArtworkOpen starts loading the selected track's artwork as the info
// view opens, unless it is loaded, loading, or known to be missing. The
// first open also asks the terminal for its cell size.
func (m *Model) libArtworkOpen() tea.Cmd {
	a := m.lib.art
	if a == nil {
		return nil
	}
	var cmds []tea.Cmd
	if !a.queried {
		a.queried = true
		cmds = append(cmds, tea.Raw(kittyimg.CellSizeQuery))
	}
	ref := artwork.Resolve(m.selectedMetadataTrack())
	if _, known := a.images[ref.Key]; ref.IsZero() || known || a.busy[ref.Key] {
		return tea.Batch(cmds...)
	}
	a.busy[ref.Key] = true
	load, id := a.load, a.id
	cmds = append(cmds, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), artLoadTimeout)
		defer cancel()
		img, err := load(ctx, ref)
		if err != nil {
			return artworkLoadedMsg{key: ref.Key, err: err}
		}
		send, err := kittyimg.Transmit(img, id)
		return artworkLoadedMsg{key: ref.Key, art: &libArtImage{img: img, send: send}, err: err}
	})
	return tea.Batch(cmds...)
}

// handleArtworkMsg keeps a finished load and the terminal's cell size.
func (m *Model) handleArtworkMsg(msg tea.Msg) bool {
	a := m.lib.art
	switch msg := msg.(type) {
	case artworkLoadedMsg:
		if a == nil {
			return true
		}
		delete(a.busy, msg.key)
		switch {
		case msg.err == nil:
			a.images[msg.key] = msg.art
		case errors.Is(msg.err, artwork.ErrNone):
			a.images[msg.key] = nil // none: not asked for again
		default:
			applog.Info("artwork: %v", msg.err) // tried again at the next open
		}
		return true
	case uv.CellSizeEvent:
		if a != nil && msg.Width > 0 && msg.Height > 0 {
			a.cellW, a.cellH = msg.Width, msg.Height
		}
		return true
	}
	return false
}

// libInfoArt returns the selected track's loaded artwork and its key, when
// the info view is open.
func (m Model) libInfoArt() (*libArtImage, string) {
	a := m.lib.art
	if a == nil || !m.showInfo {
		return nil, ""
	}
	ref := artwork.Resolve(m.selectedMetadataTrack())
	if ref.IsZero() {
		return nil, ""
	}
	return a.images[ref.Key], ref.Key
}

// artBox returns the cells an image of size fills beside the metadata in a
// body of width×rows, keeping its aspect with cells cellW×cellH pixels, or
// ok false when there is no room for it: the metadata keeps its column
// first, the artwork takes at most half the width.
func artBox(width, rows int, size image.Point, cellW, cellH int) (cols, artRows int, ok bool) {
	if size.X <= 0 || size.Y <= 0 {
		return 0, 0, false
	}
	if cellW <= 0 || cellH <= 0 {
		cellW, cellH = 1, 2
	}
	maxCols := min(width-artIndent-artGap-artMetaMinWidth, width/2, kittyimg.MaxCells)
	artRows = min(rows, kittyimg.MaxCells)
	// cols/rows in cells, times cellW/cellH, is the image's width/height.
	cols = (artRows*size.X*cellH + size.Y*cellW - 1) / (size.Y * cellW)
	if cols > maxCols {
		cols = maxCols
		artRows = cols * size.Y * cellW / (size.X * cellH)
	}
	if artRows < artMinRows || cols < 1 {
		return 0, 0, false
	}
	return cols, artRows, true
}

// libInfoArtBox is the artwork's box in the info view as it is laid out
// now, or ok false when the view shows no artwork.
func (m Model) libInfoArtBox() (art *libArtImage, key string, cols, rows int, ok bool) {
	art, key = m.libInfoArt()
	if art == nil {
		return nil, "", 0, 0, false
	}
	width, bodyRows := m.bodySize()
	cols, rows, ok = artBox(width, bodyRows, art.img.Bounds().Size(), m.lib.art.cellW, m.lib.art.cellH)
	return art, key, cols, rows, ok
}

// libInfoBody renders the info view with the artwork beside the metadata,
// or ok false to leave it to cliamp's metadata-only view.
func (m Model) libInfoBody() (string, bool) {
	_, _, cols, artRows, ok := m.libInfoArtBox()
	if !ok {
		return "", false
	}
	width, rows := m.bodySize()
	art := kittyimg.Placeholders(m.lib.art.id, cols, artRows)
	pad := strings.Repeat(" ", cols)
	lines := m.infoLines()
	start := min(m.infoScroll, max(0, len(lines)-rows))
	// infoLines' rows start with a two-cell indent of their own.
	metaWidth := width - artIndent - cols - artGap
	out := make([]string, rows)
	for i := range rows {
		left := pad
		if i < len(art) {
			left = art[i]
		}
		meta := ""
		if j := start + i; j < len(lines) {
			meta = ansi.Truncate(lines[j], metaWidth, "…")
		}
		out[i] = strings.Repeat(" ", artIndent) + left + strings.Repeat(" ", artGap) + meta
	}
	return bodyLines(out, rows), true
}

// libArtworkSync brings the terminal in line with the info view after msg:
// it sends the selected track's image once, places it anew when its box
// changes (a resize, the key bar, the cell size), asks for the cell size
// again after a resize (a font change moves it), and frees the image before
// ddmus quits. It returns cmd with those sequences. Ticks and other
// messages that cannot change the box pass straight through.
func (m *Model) libArtworkSync(msg tea.Msg, cmd tea.Cmd) tea.Cmd {
	a := m.lib.art
	if a == nil {
		return cmd
	}
	if m.quitting {
		if a.sent == "" {
			return cmd
		}
		a.sent = ""
		return tea.Sequence(tea.Raw(kittyimg.Delete(a.id)), cmd)
	}
	var out []tea.Cmd
	switch msg.(type) {
	case tea.WindowSizeMsg:
		if a.queried {
			out = append(out, tea.Raw(kittyimg.CellSizeQuery))
		}
	case tea.KeyPressMsg, uv.CellSizeEvent, artworkLoadedMsg:
	default:
		return cmd
	}
	if !m.showInfo {
		return tea.Batch(append(out, cmd)...) // the image stays with the terminal for the next open
	}
	// The update may have changed what the layout depends on (the info view
	// opening, the key bar); lay out as View will before measuring the box.
	m.recomputeLayout()
	art, key, cols, rows, ok := m.libInfoArtBox()
	switch {
	case !ok:
	case a.sent != key:
		out = append(out, tea.Raw(art.send+kittyimg.Place(a.id, cols, rows)))
		a.sent, a.cols, a.rows = key, cols, rows
	case a.cols != cols || a.rows != rows:
		out = append(out, tea.Raw(kittyimg.Place(a.id, cols, rows)))
		a.cols, a.rows = cols, rows
	}
	return tea.Batch(append(out, cmd)...)
}
