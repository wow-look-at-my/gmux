// Package vt is a VT and xterm terminal emulator core. It consumes the bytes a
// program writes to a pty and keeps a grid of styled cells plus scrollback.
// A Screen is not safe for concurrent use; the caller locks.
package vt

import "strings"

// CursorShape is the shape the cursor is drawn with.
type CursorShape uint8

// The cursor shapes.
const (
	CursorBlock CursorShape = iota
	CursorUnderline
	CursorBar
)

// Cursor is the cursor position and appearance.
type Cursor struct {
	X, Y    int
	Visible bool
	Shape   CursorShape
	Blink   bool
}

// Options configures a Screen.
type Options struct {
	Scrollback int // lines of history for the primary screen; zero means the default, negative means none
}

const defaultScrollback = 10000

// grid is the cells of one screen.
type grid struct {
	rows    [][]Cell
	wrapped []bool
}

// savedCursor is the state DECSC stores.
type savedCursor struct {
	x, y        int
	style       Style
	charsets    [2]charset
	gl          int
	origin      bool
	pendingWrap bool
}

// Screen is a terminal emulator.
type Screen struct {
	cols, rows int
	pri, alt   grid
	g          *grid
	hist       history

	x, y        int
	pendingWrap bool
	style       Style
	charsets    [2]charset
	gl          int
	saved       [2]savedCursor
	top, bottom int
	tabs        []bool
	modes       Modes
	cursorOn    bool
	shape       CursorShape
	blink       bool
	lastRune    rune
	lastWidth   int

	title, cwd string
	onReply    func([]byte)
	onBell     func()
	onTitle    func(string)
	onClip     func([]byte)
	reply      []byte

	version uint64
	dirty   bool

	graphemes []string
	gindex    map[string]uint16

	parser
}

// New returns a Screen of the given size.
func New(cols, rows int, opts Options) *Screen {
	cols, rows = max(cols, 1), max(rows, 1)
	n := opts.Scrollback
	if n == 0 {
		n = defaultScrollback
	}
	s := &Screen{cols: cols, rows: rows}
	s.hist.max = max(n, 0)
	s.pri = newGrid(cols, rows)
	s.alt = newGrid(cols, rows)
	s.reset()
	return s
}

func newGrid(cols, rows int) grid {
	g := grid{rows: make([][]Cell, rows), wrapped: make([]bool, rows)}
	for i := range g.rows {
		g.rows[i] = make([]Cell, cols)
		fillBlank(g.rows[i], Style{})
	}
	return g
}

// reset puts every piece of state back to its power-on value except scrollback.
func (s *Screen) reset() {
	s.g = &s.pri
	clearGrid(&s.pri, Style{})
	clearGrid(&s.alt, Style{})
	s.x, s.y, s.pendingWrap = 0, 0, false
	s.style = Style{}
	s.charsets = [2]charset{}
	s.gl = 0
	s.saved = [2]savedCursor{}
	s.top, s.bottom = 0, s.rows-1
	s.modes = Modes{AutoWrap: true}
	s.cursorOn, s.shape, s.blink = true, CursorBlock, false
	s.lastRune, s.lastWidth = 0, 0
	s.tabs = make([]bool, s.cols)
	for i := 0; i < s.cols; i += tabWidth {
		s.tabs[i] = true
	}
	s.parser = parser{osc: s.parser.osc[:0]}
}

func clearGrid(g *grid, st Style) {
	for i := range g.rows {
		fillBlank(g.rows[i], st)
		g.wrapped[i] = false
	}
}

// fillBlank sets every cell of row to a blank with the background of st.
func fillBlank(row []Cell, st Style) {
	b := Cell{Width: 1, Style: Style{Bg: st.Bg}}
	for i := range row {
		row[i] = b
	}
}

// Reset performs a full reset (RIS).
func (s *Screen) Reset() {
	s.reset()
	s.version++
}

// Size returns the screen size.
func (s *Screen) Size() (cols, rows int) { return s.cols, s.rows }

// Row returns row y of the screen now showing, or nil when y is out of range.
// The slice is a read-only view valid until the next Write or Resize.
func (s *Screen) Row(y int) []Cell {
	if y < 0 || y >= s.rows {
		return nil
	}
	return s.g.rows[y]
}

// RowWrapped reports whether row y continues onto the next row because of autowrap.
func (s *Screen) RowWrapped(y int) bool {
	if y < 0 || y >= s.rows {
		return false
	}
	return s.g.wrapped[y]
}

// HistoryLen returns the number of lines in scrollback.
func (s *Screen) HistoryLen() int { return s.hist.n }

// HistoryRow returns history line i; zero is the oldest line.
func (s *Screen) HistoryRow(i int) []Cell {
	if i < 0 || i >= s.hist.n {
		return nil
	}
	return s.hist.lines[s.hist.slot(i)]
}

// HistoryWrapped reports whether history line i continues onto the next line.
func (s *Screen) HistoryWrapped(i int) bool {
	if i < 0 || i >= s.hist.n {
		return false
	}
	return s.hist.wrapped[s.hist.slot(i)]
}

// Cursor returns the cursor state.
func (s *Screen) Cursor() Cursor {
	return Cursor{X: s.x, Y: s.y, Visible: s.cursorOn, Shape: s.shape, Blink: s.blink}
}

// Modes returns the terminal modes.
func (s *Screen) Modes() Modes { return s.modes }

// Title returns the window title.
func (s *Screen) Title() string { return s.title }

// Cwd returns the reported working directory, or "" when never reported.
func (s *Screen) Cwd() string { return s.cwd }

// OnReply sets the function that receives answers to queries. The slice is valid only during the call.
func (s *Screen) OnReply(fn func([]byte)) { s.onReply = fn }

// OnBell sets the function called on BEL.
func (s *Screen) OnBell(fn func()) { s.onBell = fn }

// OnTitle sets the function called when the title changes.
func (s *Screen) OnTitle(fn func(string)) { s.onTitle = fn }

// OnClipboard sets the function that receives decoded clipboard set requests.
func (s *Screen) OnClipboard(fn func(data []byte)) { s.onClip = fn }

// Version increases on every Write or Resize that changed visible state.
func (s *Screen) Version() uint64 { return s.version }

// String returns the visible screen as plain text with trailing blanks trimmed.
func (s *Screen) String() string {
	var b strings.Builder
	for y, row := range s.g.rows {
		if y > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strings.TrimRight(s.rowText(row), " "))
	}
	return b.String()
}

func (s *Screen) rowText(row []Cell) string {
	var b strings.Builder
	for _, c := range row {
		b.WriteString(s.Text(c))
	}
	return b.String()
}

func (s *Screen) sendReply() {
	if s.onReply != nil {
		s.onReply(s.reply)
	}
}
