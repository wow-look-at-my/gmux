// reflow.go holds the rewrapping of the primary screen and its scrollback when the width changes.
package vt

// wrapper cuts text lines into rows of one width.
type wrapper struct {
	cols    int
	rows    [][]Cell
	wrapped []bool
}

// add cuts one text line into rows. at is a cell of the line, the cell after its end, or negative. The answer is
// the row and the column where that cell lands. A column equal to the width is the place after a full row.
func (w *wrapper) add(line []Cell, at int) (row, col int) {
	cur := make([]Cell, 0, w.cols)
	for i := 0; i < len(line); i++ {
		c := line[i]
		wide := c.Width == 2 && w.cols >= 2
		if c.Width == 0 || c.Width == 2 && !wide {
			// Half of a wide glyph with no place for the whole of it.
			c = Cell{Width: 1, Style: Style{Bg: c.Style.Bg}}
		}
		if need := 1 + btoi(wide); len(cur)+need > w.cols {
			cur = w.flush(cur, true)
		}
		if i == at {
			row, col = len(w.rows), len(cur)
		}
		cur = append(cur, c)
		if !wide {
			continue
		}
		tail := Cell{Style: c.Style}
		if i+1 < len(line) && line[i+1].Width == 0 {
			i++
			tail = line[i]
			if i == at {
				row, col = len(w.rows), len(cur)
			}
		}
		cur = append(cur, tail)
	}
	if at >= len(line) {
		row, col = len(w.rows), len(cur)
	}
	w.flush(cur, false)
	return row, col
}

// flush ends a row, filled with blanks to the width, and returns an empty one.
func (w *wrapper) flush(cur []Cell, wrapped bool) []Cell {
	n := len(cur)
	cur = cur[:w.cols]
	fillBlank(cur[n:], Style{})
	w.rows = append(w.rows, cur)
	w.wrapped = append(w.wrapped, wrapped)
	return make([]Cell, 0, w.cols)
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// blank reports whether a cell is an empty one in the default style.
func (c Cell) blank() bool { return c.r == 0 && c.g == 0 && c.Width == 1 && c.Style == (Style{}) }

// trimBlank cuts the empty cells off the end of a row.
func trimBlank(row []Cell) []Cell {
	for len(row) > 0 && row[len(row)-1].blank() {
		row = row[:len(row)-1]
	}
	return row
}

// line is row i of the scrollback and the primary grid taken together, and whether it runs on into the next row.
func (s *Screen) line(i int) ([]Cell, bool) {
	if i < s.hist.n {
		slot := s.hist.slot(i)
		return s.hist.lines[slot], s.hist.wrapped[slot]
	}
	return s.pri.rows[i-s.hist.n], s.pri.wrapped[i-s.hist.n]
}

// reflow rewraps the scrollback and the primary grid to the screen's width,
// for a grid of the given height. Rows that the width cut from one text line
// are joined, and the line is cut again. x, y and pending are the primary
// screen's cursor. The cursor stays on the cell it was on.
func (s *Screen) reflow(rows int, x, y *int, pending *bool) {
	cursor := s.hist.n + *y
	// The text ends at the cursor row, or at the last row below it that holds something.
	end := cursor
	for i := s.hist.n + len(s.pri.rows) - 1; i > cursor; i-- {
		if row, _ := s.line(i); !rowBlank(row) {
			end = i
			break
		}
	}

	w := wrapper{cols: s.cols}
	var text []Cell
	// past is how far the cursor stands beyond the end of its line's text.
	at, past, cy, cx := -1, 0, 0, 0
	for i := 0; i <= end; i++ {
		row, wrapped := s.line(i)
		wrapped = wrapped && i < end
		if !wrapped {
			row = trimBlank(row)
		} else if next, _ := s.line(i + 1); len(row) > 0 && row[len(row)-1].blank() && len(next) > 0 && next[0].Width == 2 {
			// The blank is where a wide glyph did not fit, and is no part of the text.
			row = row[:len(row)-1]
		}
		if i == cursor {
			at = len(text) + *x + btoi(*pending)
		}
		text = append(text, row...)
		if i == cursor && at > len(text) {
			at, past = len(text), at-len(text)
		}
		if wrapped {
			continue
		}
		r, c := w.add(text, at)
		if at >= 0 {
			cy, cx, at = r, c, -1
		}
		text = text[:0]
	}

	// After a full row the cursor waits to wrap.
	*pending = cx == s.cols
	if !*pending {
		cx += past
	}
	*x = min(cx, s.cols-1)

	// The grid shows the end of the text. The cursor row stays on it, so rows below a cursor far up are lost.
	top := min(max(len(w.rows)-rows, 0), cy)
	*y = cy - top
	s.hist.clear()
	for i := range top {
		s.hist.push(w.rows[i], w.wrapped[i])
	}
	g := &s.pri
	g.rows = make([][]Cell, 0, rows)
	g.wrapped = make([]bool, 0, rows)
	for i := top; i < len(w.rows) && len(g.rows) < rows; i++ {
		g.rows = append(g.rows, w.rows[i])
		g.wrapped = append(g.wrapped, w.wrapped[i])
	}
	if top+rows < len(w.rows) {
		// The text went on below the grid, and the row that continued the last one is gone.
		g.wrapped[rows-1] = false
	}
	for len(g.rows) < rows {
		row := make([]Cell, s.cols)
		fillBlank(row, Style{})
		g.rows = append(g.rows, row)
		g.wrapped = append(g.wrapped, false)
	}
}
