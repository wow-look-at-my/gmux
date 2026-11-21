// panes.go holds the widget that draws a session's active window: every pane's screen, and the lines between them.

package ui

import (
	"strconv"

	"github.com/wow-look-at-my/go-tmux/layout"
	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/go-tmux/vt"
	"github.com/wow-look-at-my/tml/cell"
	"github.com/wow-look-at-my/tml/widget"
)

// panes is the element behind <Panes>. One instance belongs to one client.
type panes struct {
	client *Client

	// line is the glyph row a pane's row is built in, kept between frames.
	line cell.Line
	// styles remembers the canvas style of each terminal style seen.
	styles map[vt.Style]cell.Style
	// texts remembers the string of each rune drawn, so a frame allocates none.
	texts map[rune]string
	// lines marks the cells of the window area that hold a divider.
	lines []bool
}

func newPanes(client *Client) *panes {
	return &panes{client: client, styles: map[vt.Style]cell.Style{}, texts: map[rune]string{}}
}

// Measure takes all the space on offer: the window area is whatever the bars leave.
func (w *panes) Measure(maxW, maxH int) (int, int) { return max(0, maxW), max(0, maxH) }

var (
	borderStyle = cell.Style{Fg: cell.Indexed(8)}
	activeStyle = cell.Style{Fg: cell.Indexed(12)}
	fillStyle   = cell.Style{Fg: cell.Indexed(8)}
	noteStyle   = cell.Style{Fg: cell.Indexed(0), Bg: cell.Indexed(11)}
)

// Draw paints the window. Consider a client bigger than the session's area. That client shows the rest as a
// dotted field, the way a window shared with a smaller terminal has to look.
func (w *panes) Draw(c cell.Canvas) {
	view := w.client.mux.View(w.client.session)
	width, height := c.Size()
	if view.Area.W < width || view.Area.H < height {
		c.Repeat(0, 0, width, height, "·", fillStyle)
	}
	c.Erase(0, 0, min(width, view.Area.W), min(height, view.Area.H))
	w.dividers(c, view)
	for _, pane := range view.Panes {
		w.pane(c.Sub(pane.Rect.X, pane.Rect.Y, pane.Rect.W, pane.Rect.H), pane)
	}
}

// junctions are the line glyphs by which neighbours hold a line: up, down, left, right, as bits in that order.
var junctions = [16]string{
	" ", "│", "│", "│", "─", "┘", "┐", "┤", "─", "└", "┌", "├", "─", "┴", "┬", "┼",
}

// dividers draws the lines between panes, joined where they meet. The lines around the active pane take the accent.
func (w *panes) dividers(c cell.Canvas, view mux.WindowView) {
	if len(view.Dividers) == 0 {
		return
	}
	area := view.Area
	if need := area.W * area.H; cap(w.lines) < need {
		w.lines = make([]bool, need)
	} else {
		w.lines = w.lines[:need]
		clear(w.lines)
	}
	at := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < area.W && y < area.H && w.lines[y*area.W+x]
	}
	for _, d := range view.Dividers {
		for y := d.Rect.Y; y < d.Rect.Y+d.Rect.H; y++ {
			for x := d.Rect.X; x < d.Rect.X+d.Rect.W; x++ {
				if x >= 0 && y >= 0 && x < area.W && y < area.H {
					w.lines[y*area.W+x] = true
				}
			}
		}
	}
	var active layout.Rect
	for _, pane := range view.Panes {
		if pane.Active {
			r := pane.Rect
			active = layout.Rect{X: r.X - 1, Y: r.Y - 1, W: r.W + 2, H: r.H + 2}
		}
	}
	for _, d := range view.Dividers {
		for y := d.Rect.Y; y < d.Rect.Y+d.Rect.H; y++ {
			for x := d.Rect.X; x < d.Rect.X+d.Rect.W; x++ {
				mask := 0
				if at(x, y-1) {
					mask |= 1
				}
				if at(x, y+1) {
					mask |= 2
				}
				if at(x-1, y) {
					mask |= 4
				}
				if at(x+1, y) {
					mask |= 8
				}
				glyph := junctions[mask]
				// A line one cell long has no neighbour to take its direction from.
				if mask == 0 {
					glyph = junctions[1]
					if d.Dir == layout.Rows {
						glyph = junctions[4]
					}
				}
				style := borderStyle
				if active.Contains(x, y) {
					style = activeStyle
				}
				c.Text(x, y, glyph, style)
			}
		}
	}
}

// Gutter is the columns a pane keeps free at its right edge, for its scrollbar.
const Gutter = 1

// paneBar is a pane's scrollbar: its history above a view of some rows, scrolled back by an offset.
func paneBar(history, offset, rows int) widget.Scrollbar {
	return widget.Scrollbar{View: rows, Y: history - offset, MaxY: history}
}

// scrollbar draws a pane's scrollbar in its gutter. A pane with no history to scroll has none: a shell that has
// printed less than a screen, or a full-screen program.
func (w *panes) scrollbar(c cell.Canvas, f mux.Frame, active bool, x, height int) {
	history := f.Screen.HistoryLen()
	if history == 0 || f.Screen.Modes().AltScreen {
		return
	}
	style := borderStyle
	if active {
		style = activeStyle
	}
	paneBar(history, f.Offset, height).Draw(c, x, style)
}

// pane draws one pane's screen on a canvas of the pane's size. The terminal is narrower than the pane by the gutter.
func (w *panes) pane(c cell.Canvas, pane mux.PaneView) {
	width, height := c.Size()
	gutter := min(w.client.mux.Gutter(), max(0, width-1))
	width -= gutter
	pane.Pane.Draw(func(f mux.Frame) {
		for y := range height {
			w.row(c, f, y, width)
		}
		if gutter > 0 {
			w.scrollbar(c, f, pane.Active, width, height)
		}
		if f.Offset > 0 {
			note := "[" + strconv.Itoa(f.Offset) + "/" + strconv.Itoa(f.Screen.HistoryLen()) + "]"
			c.Text(max(0, width-len(note)), 0, note, noteStyle)
			return
		}
		cursor := f.Screen.Cursor()
		if pane.Active && cursor.Visible && w.client.overlay() == "" {
			c.Restyle(cursor.X, cursor.Y, 1, 1, func(drawn cell.Cell) cell.Style {
				return drawn.Style.Reverse(!drawn.Style.Has(cell.Reverse))
			})
		}
	})
}

// row draws one row of a pane's view.
func (w *panes) row(c cell.Canvas, f mux.Frame, y, width int) {
	cells := f.Row(y)
	textLine := f.Line(y)
	line := w.line[:0]
	for x := 0; x < len(cells) && x < width; x++ {
		src := cells[x]
		if src.Width == 0 {
			continue
		}
		style := w.style(src.Style)
		if f.InSelection(x, textLine) {
			style = style.Reverse(!style.Has(cell.Reverse))
		}
		text := w.text(f.Screen, src)
		if src.Style.Attrs&vt.Invisible != 0 {
			text = " "
		}
		line = append(line, cell.Glyph{Text: text, Width: int(src.Width), Style: style})
	}
	w.line = line
	c.Line(0, y, line, cell.Style{})
}

// text is a cell's grapheme as a string, without allocating for a rune already seen.
func (w *panes) text(screen *vt.Screen, src vt.Cell) string {
	text := screen.Text(src)
	if len(text) <= 1 {
		return text
	}
	r := src.Rune()
	if string(r) != text {
		// A cluster of several runes is the screen's own interned string.
		return text
	}
	if known, ok := w.texts[r]; ok {
		return known
	}
	if len(w.texts) > maxTexts {
		clear(w.texts)
	}
	w.texts[r] = text
	return text
}

// maxTexts and maxStyles bound the caches. A program that cycles through colours must not grow them forever.
const (
	maxTexts  = 4096
	maxStyles = 4096
)

// style is a terminal style as a canvas style.
func (w *panes) style(src vt.Style) cell.Style {
	if src == (vt.Style{}) {
		return cell.Style{}
	}
	if known, ok := w.styles[src]; ok {
		return known
	}
	style := cell.Style{Fg: colour(src.Fg), Bg: colour(src.Bg)}
	for _, pair := range attrPairs {
		if src.Attrs&pair.from != 0 {
			style = style.With(pair.to, true)
		}
	}
	if len(w.styles) > maxStyles {
		clear(w.styles)
	}
	w.styles[src] = style
	return style
}

// attrPairs maps each terminal attribute to the canvas attribute that draws it.
var attrPairs = []struct {
	from vt.Attr
	to   cell.Attr
}{
	{vt.Bold, cell.Bold}, {vt.Faint, cell.Faint}, {vt.Italic, cell.Italic}, {vt.Underline, cell.Underline},
	{vt.Blink, cell.Blink}, {vt.Reverse, cell.Reverse}, {vt.Strikethrough, cell.Strikethrough},
}

func colour(src vt.Color) cell.Color {
	if index, ok := src.Index(); ok {
		return cell.Indexed(index)
	}
	if r, g, b, ok := src.RGB(); ok {
		return cell.RGB(r, g, b)
	}
	return cell.Color{}
}
