// csi.go holds the dispatch of control sequences and cursor movement.
package vt

import "strconv"

// dispatchCSI runs a complete control sequence with final byte f.
func (s *Screen) dispatchCSI(f byte) {
	if s.ninter > 0 {
		s.csiInter(f)
		return
	}
	switch s.priv {
	case 0:
	case '?':
		s.csiPrivate(f)
		return
	case '>':
		if f == 'c' && s.param(0, 0) == 0 {
			s.answer("\x1b[>1;10;0c")
		}
		return
	default:
		return
	}
	if f != 'n' && f != 'c' && f != 't' {
		s.dirty = true
	}
	switch f {
	case '@':
		s.insertCells(s.count(0))
	case 'A':
		s.cursorUp(s.count(0))
	case 'B', 'e':
		s.cursorDown(s.count(0))
	case 'C', 'a':
		s.moveTo(s.x+s.count(0), s.y)
	case 'D':
		s.moveTo(s.x-s.count(0), s.y)
	case 'E':
		s.cursorDown(s.count(0))
		s.x = 0
	case 'F':
		s.cursorUp(s.count(0))
		s.x = 0
	case 'G', '`':
		s.moveTo(s.count(0)-1, s.y)
	case 'H', 'f':
		s.cup(s.count(0)-1, s.count(1)-1)
	case 'I':
		s.tabForward(s.count(0))
	case 'J':
		s.eraseDisplay(s.param(0, 0))
	case 'K':
		s.eraseLine(s.param(0, 0))
	case 'L':
		s.insertLines(s.count(0))
	case 'M':
		s.deleteLines(s.count(0))
	case 'P':
		s.deleteCells(s.count(0))
	case 'S':
		s.scrollUp(s.top, s.bottom, s.count(0), true)
	case 'T':
		if s.nparams <= 1 {
			s.scrollDown(s.top, s.bottom, s.count(0))
		}
	case 'X':
		s.eraseChars(s.count(0))
	case 'Z':
		s.tabBack(s.count(0))
	case 'b':
		s.repeat(s.count(0))
	case 'c':
		if s.param(0, 0) == 0 {
			s.answer("\x1b[?62;22c")
		}
	case 'd':
		s.vpa(s.count(0) - 1)
	case 'g':
		s.clearTabs(s.param(0, 0))
	case 'h', 'l':
		s.ansiMode(f == 'h')
	case 'm':
		s.sgr()
	case 'n':
		s.deviceStatus(false)
	case 'r':
		s.setMargins(s.count(0)-1, s.param(1, s.rows)-1)
	case 's':
		if s.nparams == 0 {
			s.saveCursor()
		}
	case 't':
		if s.param(0, 0) == 18 {
			s.reply = append(s.reply[:0], "\x1b[8;"...)
			s.reply = strconv.AppendInt(s.reply, int64(s.rows), 10)
			s.reply = append(s.reply, ';')
			s.reply = strconv.AppendInt(s.reply, int64(s.cols), 10)
			s.reply = append(s.reply, 't')
			s.sendReply()
		}
	case 'u':
		if s.nparams == 0 {
			s.restoreCursor()
		}
	}
}

// csiInter runs a control sequence that has intermediate bytes.
func (s *Screen) csiInter(f byte) {
	if s.ninter != 1 {
		return
	}
	switch {
	case s.inter[0] == '$' && f == 'p':
		s.requestMode(s.priv == '?')
	case s.priv != 0:
	case s.inter[0] == ' ' && f == 'q':
		s.setCursorStyle(s.param(0, 0))
	case s.inter[0] == '!' && f == 'p':
		s.softReset()
	}
}

func (s *Screen) csiPrivate(f byte) {
	switch f {
	case 'h', 'l':
		s.dirty = true
		for i := range s.nparams {
			s.privateMode(s.param(i, 0), f == 'h')
		}
	case 'n':
		s.deviceStatus(true)
	}
}

func (s *Screen) answer(msg string) {
	s.reply = append(s.reply[:0], msg...)
	s.sendReply()
}

// deviceStatus answers DSR; private selects the DEC form of the position report.
func (s *Screen) deviceStatus(private bool) {
	switch s.param(0, 0) {
	case 5:
		if !private {
			s.answer("\x1b[0n")
		}
	case 6:
		y := s.y
		if s.modes.Origin {
			y -= s.top
		}
		s.reply = append(s.reply[:0], "\x1b["...)
		if private {
			s.reply = append(s.reply, '?')
		}
		s.reply = strconv.AppendInt(s.reply, int64(y+1), 10)
		s.reply = append(s.reply, ';')
		s.reply = strconv.AppendInt(s.reply, int64(s.x+1), 10)
		s.reply = append(s.reply, 'R')
		s.sendReply()
	}
}

func (s *Screen) setCursorStyle(n int) {
	switch n {
	case 0:
		s.shape, s.blink = CursorBlock, false
	case 1, 2:
		s.shape, s.blink = CursorBlock, n == 1
	case 3, 4:
		s.shape, s.blink = CursorUnderline, n == 3
	case 5, 6:
		s.shape, s.blink = CursorBar, n == 5
	default:
		return
	}
	s.dirty = true
}

// moveTo moves the cursor and clamps it to the screen.
func (s *Screen) moveTo(x, y int) {
	s.x = min(max(x, 0), s.cols-1)
	s.y = min(max(y, 0), s.rows-1)
	s.pendingWrap = false
}

// cup moves to a row and column, relative to the margins in origin mode.
func (s *Screen) cup(row, col int) {
	if s.modes.Origin {
		row = min(row+s.top, s.bottom)
	}
	s.moveTo(col, row)
}

func (s *Screen) vpa(row int) {
	s.cup(row, s.x)
}

// cursorUp stops at the top margin when the cursor starts inside the margins.
func (s *Screen) cursorUp(n int) {
	lim := 0
	if s.y >= s.top {
		lim = s.top
	}
	s.moveTo(s.x, max(s.y-n, lim))
}

// cursorDown stops at the bottom margin when the cursor starts inside the margins.
func (s *Screen) cursorDown(n int) {
	lim := s.rows - 1
	if s.y <= s.bottom {
		lim = s.bottom
	}
	s.moveTo(s.x, min(s.y+n, lim))
}

// setMargins sets the scroll region (DECSTBM) and homes the cursor.
func (s *Screen) setMargins(top, bottom int) {
	bottom = min(bottom, s.rows-1)
	if top >= bottom {
		return
	}
	s.top, s.bottom = top, bottom
	s.cup(0, 0)
}

func (s *Screen) repeat(n int) {
	if s.lastRune == 0 {
		return
	}
	n = min(n, s.cols*s.rows)
	r, w := s.lastRune, s.lastWidth
	for range n {
		s.put(r, w)
	}
}

func (s *Screen) tabForward(n int) {
	s.dirty = true
	for ; n > 0 && s.x < s.cols-1; n-- {
		s.x++
		for s.x < s.cols-1 && !s.tabs[s.x] {
			s.x++
		}
	}
}

func (s *Screen) tabBack(n int) {
	s.pendingWrap = false
	for ; n > 0 && s.x > 0; n-- {
		s.x--
		for s.x > 0 && !s.tabs[s.x] {
			s.x--
		}
	}
}

func (s *Screen) clearTabs(mode int) {
	switch mode {
	case 0:
		s.tabs[s.x] = false
	case 3:
		clear(s.tabs)
	}
}

func (s *Screen) saveCursor() {
	s.saved[s.screenIndex()] = savedCursor{
		x: s.x, y: s.y, style: s.style, charsets: s.charsets, gl: s.gl,
		origin: s.modes.Origin, pendingWrap: s.pendingWrap,
	}
}

func (s *Screen) restoreCursor() {
	c := s.saved[s.screenIndex()]
	s.moveTo(c.x, c.y)
	s.style, s.charsets, s.gl = c.style, c.charsets, c.gl
	s.modes.Origin = c.origin
	s.pendingWrap = c.pendingWrap && s.modes.AutoWrap
	s.dirty = true
}

func (s *Screen) screenIndex() int {
	if s.modes.AltScreen {
		return 1
	}
	return 0
}

// softReset performs DECSTR.
func (s *Screen) softReset() {
	s.cursorOn = true
	s.modes.Origin, s.modes.AutoWrap, s.modes.Insert = false, true, false
	s.modes.AppCursor, s.modes.AppKeypad = false, false
	s.top, s.bottom = 0, s.rows-1
	s.charsets, s.gl = [2]charset{}, 0
	s.style = Style{}
	s.saved[s.screenIndex()] = savedCursor{}
	s.pendingWrap = false
	s.dirty = true
}

// alignmentTest fills the screen with the letter E (DECALN).
func (s *Screen) alignmentTest() {
	for y, row := range s.g.rows {
		for x := range row {
			row[x] = Cell{r: 'E', Width: 1}
		}
		s.g.wrapped[y] = false
	}
	s.top, s.bottom = 0, s.rows-1
	s.moveTo(0, 0)
}
