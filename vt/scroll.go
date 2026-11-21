// scroll.go holds the scrollback ring and the scrolling of the grid.
package vt

// history is a ring of lines that left the top of the primary screen.
// Until the ring first fills, start stays at zero and lines grows by append.
type history struct {
	lines   [][]Cell
	wrapped []bool
	start   int
	n       int
	max     int
}

func (h *history) slot(i int) int { return (h.start + i) % len(h.lines) }

// push adds row as the newest line and returns a slice the caller may reuse, or nil.
func (h *history) push(row []Cell, w bool) []Cell {
	if h.max <= 0 {
		return row
	}
	if h.n == len(h.lines) && len(h.lines) < h.max {
		h.lines = append(h.lines, row)
		h.wrapped = append(h.wrapped, w)
		h.n++
		return nil
	}
	var i int
	if h.n < h.max {
		i = h.slot(h.n)
		h.n++
	} else {
		i = h.start
		h.start = (h.start + 1) % len(h.lines)
	}
	old := h.lines[i]
	h.lines[i], h.wrapped[i] = row, w
	return old
}

// pop removes and returns the newest line.
func (h *history) pop() ([]Cell, bool) {
	i := h.slot(h.n - 1)
	row, w := h.lines[i], h.wrapped[i]
	h.lines[i] = nil
	h.n--
	return row, w
}

func (h *history) clear() {
	clear(h.lines)
	h.n, h.start = 0, 0
}

// blankRow returns buf, or a new slice when buf is too small, sized to cols and blanked.
func (s *Screen) blankRow(buf []Cell) []Cell {
	if cap(buf) < s.cols {
		buf = make([]Cell, s.cols)
	}
	buf = buf[:s.cols]
	fillBlank(buf, s.style)
	return buf
}

// scrollUp moves rows top..bottom up by n. Rows that leave the top of the
// primary screen enter scrollback when hist is set.
func (s *Screen) scrollUp(top, bottom, n int, hist bool) {
	g := s.g
	n = min(n, bottom-top+1)
	hist = hist && top == 0 && g == &s.pri
	for range n {
		old, ow := g.rows[top], g.wrapped[top]
		copy(g.rows[top:bottom], g.rows[top+1:bottom+1])
		copy(g.wrapped[top:bottom], g.wrapped[top+1:bottom+1])
		if hist {
			old = s.hist.push(old, ow)
		}
		g.rows[bottom] = s.blankRow(old)
		g.wrapped[bottom] = false
	}
	s.dirty = true
}

// scrollDown moves rows top..bottom down by n and blanks the rows at the top.
func (s *Screen) scrollDown(top, bottom, n int) {
	g := s.g
	n = min(n, bottom-top+1)
	for range n {
		old := g.rows[bottom]
		copy(g.rows[top+1:bottom+1], g.rows[top:bottom])
		copy(g.wrapped[top+1:bottom+1], g.wrapped[top:bottom])
		g.rows[top] = s.blankRow(old)
		g.wrapped[top] = false
	}
	s.dirty = true
}

// index moves the cursor down, scrolling at the bottom margin (IND).
func (s *Screen) index() {
	s.pendingWrap = false
	if s.y == s.bottom {
		s.scrollUp(s.top, s.bottom, 1, true)
	} else if s.y < s.rows-1 {
		s.y++
	}
}

// reverseIndex moves the cursor up, scrolling at the top margin (RI).
func (s *Screen) reverseIndex() {
	s.pendingWrap = false
	if s.y == s.top {
		s.scrollDown(s.top, s.bottom, 1)
	} else if s.y > 0 {
		s.y--
	}
}

// insertLines inserts n blank lines at the cursor row inside the margins (IL).
func (s *Screen) insertLines(n int) {
	if s.y < s.top || s.y > s.bottom {
		return
	}
	s.scrollDown(s.y, s.bottom, n)
	s.x, s.pendingWrap = 0, false
}

// deleteLines deletes n lines at the cursor row inside the margins (DL).
func (s *Screen) deleteLines(n int) {
	if s.y < s.top || s.y > s.bottom {
		return
	}
	s.scrollUp(s.y, s.bottom, n, false)
	s.x, s.pendingWrap = 0, false
}
