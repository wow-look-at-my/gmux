// resize.go holds the resizing of both grids, the scrollback exchange and tab stops.
package vt

const tabWidth = 8

// Resize changes the screen size. Rows keep their content without reflow.
func (s *Screen) Resize(cols, rows int) {
	cols, rows = max(cols, 1), max(rows, 1)
	if cols == s.cols && rows == s.rows {
		return
	}
	oldCols := s.cols
	s.cols = cols

	// The primary grid exchanges lines with scrollback around its cursor row.
	cy := &s.y
	if s.modes.AltScreen {
		cy = &s.saved[0].y
	}
	s.resizePrimary(rows, cy)
	s.resizeAlt(rows)
	s.rows = rows

	tabs := make([]bool, cols)
	copy(tabs, s.tabs)
	for i := oldCols; i < cols; i++ {
		tabs[i] = i%tabWidth == 0
	}
	s.tabs = tabs

	s.top, s.bottom = 0, rows-1
	s.x = min(s.x, cols-1)
	s.y = min(max(s.y, 0), rows-1)
	s.pendingWrap = false
	for i := range s.saved {
		s.saved[i].x = min(s.saved[i].x, cols-1)
		s.saved[i].y = min(max(s.saved[i].y, 0), rows-1)
	}
	s.version++
}

func (s *Screen) resizePrimary(rows int, cy *int) {
	g := &s.pri
	old := len(g.rows)
	switch {
	case rows < old:
		remove := old - rows
		for remove > 0 && len(g.rows)-1 > *cy && rowBlank(g.rows[len(g.rows)-1]) {
			g.rows = g.rows[:len(g.rows)-1]
			g.wrapped = g.wrapped[:len(g.wrapped)-1]
			remove--
		}
		// Lines leave from the top only until the cursor row fits; the rest leave from the bottom.
		up := min(remove, max(*cy-(rows-1), 0))
		for i := range up {
			s.hist.push(g.rows[i], g.wrapped[i])
		}
		g.rows = append([][]Cell(nil), g.rows[up:up+rows]...)
		g.wrapped = append([]bool(nil), g.wrapped[up:up+rows]...)
		*cy -= up
	case rows > old:
		add := rows - old
		pull := min(add, s.hist.n)
		nr := make([][]Cell, pull, rows)
		nw := make([]bool, pull, rows)
		for i := pull - 1; i >= 0; i-- {
			nr[i], nw[i] = s.hist.pop()
		}
		nr = append(nr, g.rows...)
		nw = append(nw, g.wrapped...)
		for len(nr) < rows {
			nr = append(nr, nil)
			nw = append(nw, false)
		}
		g.rows, g.wrapped = nr, nw
		*cy += pull
	}
	s.fitRows(g)
}

func (s *Screen) resizeAlt(rows int) {
	g := &s.alt
	if rows < len(g.rows) {
		g.rows, g.wrapped = g.rows[:rows], g.wrapped[:rows]
	}
	for len(g.rows) < rows {
		g.rows = append(g.rows, nil)
		g.wrapped = append(g.wrapped, false)
	}
	s.fitRows(g)
}

// fitRows truncates or pads every row of g to the screen width.
func (s *Screen) fitRows(g *grid) {
	for i, row := range g.rows {
		g.rows[i] = fitRow(row, s.cols)
	}
}

func fitRow(row []Cell, cols int) []Cell {
	n := len(row)
	switch {
	case n == cols:
		return row
	case n > cols:
		row = row[:cols]
		if row[cols-1].Width == 2 {
			row[cols-1] = Cell{Width: 1, Style: Style{Bg: row[cols-1].Style.Bg}}
		}
		return row
	case cap(row) < cols:
		nr := make([]Cell, cols)
		copy(nr, row)
		row = nr
	default:
		row = row[:cols]
	}
	fillBlank(row[n:], Style{})
	return row
}

// rowBlank reports whether a row holds only default blanks.
func rowBlank(row []Cell) bool {
	for _, c := range row {
		if c.r != 0 || c.Style != (Style{}) {
			return false
		}
	}
	return true
}
