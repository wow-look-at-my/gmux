// view.go holds what a client sees of a pane: the scrolled view and the selection.

package mux

import (
	"strings"

	"github.com/wow-look-at-my/go-tmux/vt"
)

// Point is a cell of a pane's text.
type Point struct{ X, Line int }

// before reports whether p comes ahead of q in reading order.
func (p Point) before(q Point) bool {
	return p.Line < q.Line || (p.Line == q.Line && p.X < q.X)
}

// Granularity is what a selection grows by.
type Granularity uint8

const (
	// ByCell selects from one cell to another.
	ByCell Granularity = iota
	// ByWord selects whole words.
	ByWord
	// ByLine selects whole lines.
	ByLine
)

type selection struct {
	anchor, head Point
	by           Granularity
	on           bool
}

// Frame is a pane's screen as it stands, handed to a drawing function while the pane is locked.
type Frame struct {
	Screen *vt.Screen
	// Offset is how many lines of history the view shows above the live screen.
	Offset int
	// Start and End bound the selection, both included. Selected is false when there is none.
	Start, End Point
	Selected   bool
}

// Row is row y of the view: a line of history while the view is scrolled back that far, else a row of the screen.
func (f Frame) Row(y int) []vt.Cell {
	line := f.Line(y)
	if history := f.Screen.HistoryLen(); line < history {
		return f.Screen.HistoryRow(line)
	} else {
		return f.Screen.Row(line - history)
	}
}

// Line is the text line that row y of the view shows.
func (f Frame) Line(y int) int { return f.Screen.HistoryLen() - f.Offset + y }

// InSelection reports whether the cell at column x of text line is selected.
func (f Frame) InSelection(x, line int) bool {
	if !f.Selected {
		return false
	}
	at := Point{X: x, Line: line}
	return !at.before(f.Start) && !f.End.before(at)
}

// Draw calls fn with the pane's screen. The screen must not be kept after fn returns.
func (p *Pane) Draw(fn func(Frame)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	frame := Frame{Screen: p.screen, Offset: p.offset}
	frame.Start, frame.End, frame.Selected = p.selected()
	fn(frame)
}

// Scroll moves the view by a number of lines: positive toward older text. It reports whether the view moved.
func (p *Pane) Scroll(lines int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	next := max(0, min(p.offset+lines, p.screen.HistoryLen()))
	if next == p.offset {
		return false
	}
	p.offset = next
	return true
}

// ScrollTo puts the view a number of lines back from the live screen.
func (p *Pane) ScrollTo(offset int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.offset = max(0, min(offset, p.screen.HistoryLen()))
}

// line is the text line row y of the view shows. The caller holds the lock.
func (p *Pane) line(y int) int { return p.screen.HistoryLen() - p.offset + y }

// cells is one text line. The caller holds the lock.
func (p *Pane) cells(line int) []vt.Cell {
	history := p.screen.HistoryLen()
	if line < 0 {
		return nil
	}
	if line < history {
		return p.screen.HistoryRow(line)
	}
	return p.screen.Row(line - history)
}

// wrapped reports whether a text line runs on into the next one. The caller holds the lock.
func (p *Pane) wrapped(line int) bool {
	history := p.screen.HistoryLen()
	if line < history {
		return p.screen.HistoryWrapped(line)
	}
	return p.screen.RowWrapped(line - history)
}

// SelectStart begins a selection at a cell of the view.
func (p *Pane) SelectStart(x, y int, by Granularity) {
	p.mu.Lock()
	defer p.mu.Unlock()
	at := Point{X: x, Line: p.line(y)}
	p.sel = selection{anchor: at, head: at, by: by, on: by != ByCell}
}

// SelectTo extends the selection to a cell of the view.
func (p *Pane) SelectTo(x, y int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	at := Point{X: x, Line: p.line(y)}
	if at != p.sel.anchor {
		p.sel.on = true
	}
	p.sel.head = at
}

// SelectClear drops the selection. It reports whether there was one.
func (p *Pane) SelectClear() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	had := p.sel.on
	p.sel = selection{}
	return had
}

// selected is the selection as an ordered pair of points, grown to whole words or lines when it selects by those.
// The caller holds the lock.
func (p *Pane) selected() (start, end Point, ok bool) {
	if !p.sel.on {
		return Point{}, Point{}, false
	}
	start, end = p.sel.anchor, p.sel.head
	if end.before(start) {
		start, end = end, start
	}
	switch p.sel.by {
	case ByWord:
		start.X, _ = p.word(start)
		_, end.X = p.word(end)
	case ByLine:
		start.X = 0
		end.X = max(0, len(p.cells(end.Line))-1)
	}
	return start, end, true
}

// word is the first and last column of the run of non-blank cells around a point. A blank cell is a word of its
// own. The caller holds the lock.
func (p *Pane) word(at Point) (first, last int) {
	cells := p.cells(at.Line)
	if at.X < 0 || at.X >= len(cells) {
		return at.X, at.X
	}
	blank := func(x int) bool { return cells[x].Rune() == ' ' && cells[x].Width != 0 }
	if blank(at.X) {
		return at.X, at.X
	}
	first, last = at.X, at.X
	for first > 0 && !blank(first-1) {
		first--
	}
	for last+1 < len(cells) && !blank(last+1) {
		last++
	}
	return first, last
}

// SelectedText is the text of the selection. Lines the terminal wrapped join back into one.
func (p *Pane) SelectedText() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	start, end, ok := p.selected()
	if !ok {
		return ""
	}
	var out strings.Builder
	for line := start.Line; line <= end.Line; line++ {
		cells := p.cells(line)
		from, to := 0, len(cells)-1
		if line == start.Line {
			from = max(0, start.X)
		}
		if line == end.Line {
			to = min(to, end.X)
		}
		var text strings.Builder
		for x := from; x <= to; x++ {
			text.WriteString(p.screen.Text(cells[x]))
		}
		joined := line < end.Line && p.wrapped(line)
		if joined {
			out.WriteString(text.String())
			continue
		}
		out.WriteString(strings.TrimRight(text.String(), " "))
		if line < end.Line {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// Refresh reads what every pane of every session runs, and renames the windows that follow their program. It tells
// the watchers of each session where something changed.
func (m *Mux) Refresh() {
	m.mu.Lock()
	type entry struct {
		s     *Session
		w     *Window
		panes []*Pane
	}
	var entries []entry
	for _, s := range m.sessions {
		for _, w := range s.windows {
			e := entry{s: s, w: w}
			for _, p := range w.panes {
				e.panes = append(e.panes, p)
			}
			entries = append(entries, e)
		}
	}
	m.mu.Unlock()

	changed := map[*Session]struct{}{}
	for _, e := range entries {
		for _, p := range e.panes {
			if p.Refresh() {
				changed[e.s] = struct{}{}
			}
		}
	}

	m.mu.Lock()
	for _, e := range entries {
		if e.w.named {
			continue
		}
		if p := e.w.panes[e.w.active]; p != nil {
			if name := p.commandName(); name != e.w.Name {
				e.w.Name = name
				changed[e.s] = struct{}{}
			}
		}
	}
	m.mu.Unlock()
	for s := range changed {
		m.emit(Event{Kind: Changed, Session: s})
	}
}
