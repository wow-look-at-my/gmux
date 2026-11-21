// erase.go holds erasing, character insertion and character deletion.
package vt

// blankRange blanks cells from..to-1 of row y with the current background,
// first splitting wide glyphs that straddle the range edges.
func (s *Screen) blankRange(y, from, to int) {
	row := s.g.rows[y]
	from, to = max(from, 0), min(to, s.cols)
	if from >= to {
		return
	}
	s.splitWide(row, from)
	s.splitWide(row, to-1)
	fillBlank(row[from:to], s.style)
	s.dirty = true
}

// eraseDisplay performs ED.
func (s *Screen) eraseDisplay(mode int) {
	switch mode {
	case 0:
		s.blankRange(s.y, s.x, s.cols)
		for y := s.y + 1; y < s.rows; y++ {
			s.blankRow(s.g.rows[y])
			s.g.wrapped[y] = false
		}
		s.g.wrapped[s.y] = false
	case 1:
		for y := range s.y {
			s.blankRow(s.g.rows[y])
			s.g.wrapped[y] = false
		}
		s.blankRange(s.y, 0, s.x+1)
	case 2:
		for y := range s.rows {
			s.blankRow(s.g.rows[y])
			s.g.wrapped[y] = false
		}
	case 3:
		s.hist.clear()
	}
	s.pendingWrap = false
}

// eraseLine performs EL.
func (s *Screen) eraseLine(mode int) {
	switch mode {
	case 0:
		s.blankRange(s.y, s.x, s.cols)
		s.g.wrapped[s.y] = false
	case 1:
		s.blankRange(s.y, 0, s.x+1)
	case 2:
		s.blankRange(s.y, 0, s.cols)
		s.g.wrapped[s.y] = false
	}
	s.pendingWrap = false
}

// eraseChars performs ECH.
func (s *Screen) eraseChars(n int) {
	s.blankRange(s.y, s.x, s.x+n)
	s.pendingWrap = false
}

// insertCells shifts the cursor row right by n from the cursor (ICH and insert mode).
func (s *Screen) insertCells(n int) {
	row := s.g.rows[s.y]
	n = min(n, s.cols-s.x)
	s.splitWide(row, s.x)
	copy(row[s.x+n:], row[s.x:])
	if last := len(row) - 1; row[last].Width == 2 {
		row[last] = Cell{Width: 1, Style: Style{Bg: row[last].Style.Bg}}
	}
	fillBlank(row[s.x:s.x+n], s.style)
	s.pendingWrap = false
	s.dirty = true
}

// deleteCells removes n cells at the cursor and blanks the right end (DCH).
func (s *Screen) deleteCells(n int) {
	row := s.g.rows[s.y]
	n = min(n, s.cols-s.x)
	s.splitWide(row, s.x)
	s.splitWide(row, s.x+n-1)
	copy(row[s.x:], row[s.x+n:])
	fillBlank(row[s.cols-n:], s.style)
	s.pendingWrap = false
	s.dirty = true
}
