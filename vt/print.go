// print.go holds the printing of graphic characters: wrap, wide glyphs, combining marks and charsets.
package vt

import "github.com/clipperhouse/displaywidth"

// charset is a character set designated into G0 or G1.
type charset uint8

const (
	charsetASCII charset = iota
	charsetGraphics
)

// decGraphics maps the DEC special graphics range, starting at the underscore, to runes.
var decGraphics = [...]rune{
	' ', '◆', '▒', '␉', '␌', '␍', '␊', '°', '±', '␤', '␋', '┘', '┐', '┌', '└', '┼',
	'⎺', '⎻', '─', '⎼', '⎽', '├', '┤', '┴', '┬', '│', '≤', '≥', 'π', '≠', '£', '·',
}

// fastASCII reports whether printable ASCII can take the direct path.
func (s *Screen) fastASCII() bool {
	return s.charsets[s.gl] == charsetASCII && !s.modes.Insert
}

// printASCII writes a run of printable ASCII bytes straight into the grid.
func (s *Screen) printASCII(p []byte) {
	s.dirty = true
	s.lastRune, s.lastWidth = rune(p[len(p)-1]), 1
	st := s.style
	for len(p) > 0 {
		if s.pendingWrap {
			s.wrapLine()
		}
		row := s.g.rows[s.y]
		x := s.x
		n := min(len(p), s.cols-x)
		last, rest := p[n-1], p[n:]
		if !s.modes.AutoWrap && len(rest) > 0 {
			// Without autowrap every byte past the margin lands in the last column.
			last, rest = p[len(p)-1], nil
		}
		if row[x].Width == 0 && x > 0 {
			row[x-1] = Cell{Width: 1, Style: Style{Bg: row[x-1].Style.Bg}}
		}
		if end := x + n; end < s.cols && row[end].Width == 0 {
			row[end] = Cell{Width: 1, Style: Style{Bg: row[end].Style.Bg}}
		}
		dst := row[x : x+n]
		for i, b := range p[:n-1] {
			dst[i] = Cell{r: rune(b), Width: 1, Style: st}
		}
		dst[n-1] = Cell{r: rune(last), Width: 1, Style: st}
		p = rest
		if x+n >= s.cols {
			s.x = s.cols - 1
			s.pendingWrap = s.modes.AutoWrap
		} else {
			s.x = x + n
		}
	}
}

// wrapLine performs a deferred autowrap.
func (s *Screen) wrapLine() {
	s.g.wrapped[s.y] = true
	s.pendingWrap = false
	s.x = 0
	s.index()
}

// printRune prints one rune through charset mapping, width and combining rules.
func (s *Screen) printRune(r rune) {
	s.dirty = true
	if r < 0x80 {
		if s.charsets[s.gl] == charsetGraphics && r >= '_' && r <= '~' {
			r = decGraphics[r-'_']
		}
		s.put(r, 1)
		return
	}
	if r < 0xa0 {
		return
	}
	w := displaywidth.Rune(r)
	if w == 0 {
		s.combine(r)
		return
	}
	if px, ok := s.prevCell(); ok && s.endsWithZWJ(s.g.rows[s.y][px]) {
		s.combine(r)
		return
	}
	s.put(r, min(w, 2))
}

// put writes a glyph of width w at the cursor.
func (s *Screen) put(r rune, w int) {
	s.lastRune, s.lastWidth = r, w
	if s.pendingWrap {
		s.wrapLine()
	}
	if w == 2 && s.cols < 2 {
		w = 1
	}
	row := s.g.rows[s.y]
	if w == 2 && s.x == s.cols-1 {
		if !s.modes.AutoWrap {
			s.x--
		} else {
			s.splitWide(row, s.x)
			row[s.x] = Cell{Width: 1, Style: Style{Bg: s.style.Bg}}
			s.wrapLine()
			row = s.g.rows[s.y]
		}
	}
	if s.modes.Insert {
		s.insertCells(w)
	}
	s.splitWide(row, s.x)
	if w == 2 {
		s.splitWide(row, s.x+1)
		row[s.x+1] = Cell{Style: s.style}
	}
	row[s.x] = Cell{r: r, Width: uint8(w), Style: s.style}
	s.x += w
	if s.x >= s.cols {
		s.x = s.cols - 1
		s.pendingWrap = s.modes.AutoWrap
	}
}

// splitWide blanks the other half of a wide glyph that has a half at x.
func (s *Screen) splitWide(row []Cell, x int) {
	switch {
	case row[x].Width == 0 && x > 0:
		row[x-1] = Cell{Width: 1, Style: Style{Bg: row[x-1].Style.Bg}}
	case row[x].Width == 2 && x+1 < len(row):
		row[x+1] = Cell{Width: 1, Style: Style{Bg: row[x+1].Style.Bg}}
	}
}

// prevCell returns the column of the glyph the last printed character went into.
func (s *Screen) prevCell() (int, bool) {
	x := s.x
	if !s.pendingWrap {
		x--
	}
	if x < 0 {
		return 0, false
	}
	if s.g.rows[s.y][x].Width == 0 && x > 0 {
		x--
	}
	return x, true
}

// combine appends a zero width rune to the glyph.
func (s *Screen) combine(r rune) {
	x, ok := s.prevCell()
	if !ok {
		return
	}
	c := &s.g.rows[s.y][x]
	if c.Width == 0 {
		return
	}
	if c.r == 0 {
		c.r = ' '
	}
	if g := s.intern(s.Text(*c) + string(r)); g != 0 {
		c.g = g
	}
}
