package vt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCursorMovement(t *testing.T) {
	tests := []struct {
		name string
		in   string
		x, y int
	}{
		{"CUU", "\x1b[5;5H\x1b[2A", 4, 2},
		{"CUU default", "\x1b[5;5H\x1b[A", 4, 3},
		{"CUU clamps", "\x1b[5;5H\x1b[99A", 4, 0},
		{"CUD", "\x1b[2B", 0, 2},
		{"CUD clamps", "\x1b[99B", 0, 9},
		{"VPR", "\x1b[2e", 0, 2},
		{"CUF", "\x1b[3C", 3, 0},
		{"HPR", "\x1b[3a", 3, 0},
		{"CUF clamps", "\x1b[99C", 9, 0},
		{"CUB", "\x1b[5C\x1b[2D", 3, 0},
		{"CUB clamps", "\x1b[99D", 0, 0},
		{"CNL", "\x1b[3C\x1b[2E", 0, 2},
		{"CPL", "\x1b[5;5H\x1b[2F", 0, 2},
		{"CHA", "\x1b[3;3H\x1b[7G", 6, 2},
		{"HPA", "\x1b[7`", 6, 0},
		{"VPA", "\x1b[3C\x1b[4d", 3, 3},
		{"CUP", "\x1b[3;4H", 3, 2},
		{"CUP zero", "\x1b[0;0H", 0, 0},
		{"CUP clamps", "\x1b[99;99H", 9, 9},
		{"HVP", "\x1b[3;4f", 3, 2},
		{"CUU stops at margin", "\x1b[3;8r\x1b[6;1H\x1b[9A", 0, 2},
		{"CUU above margin", "\x1b[3;8r\x1b[2;1H\x1b[9A", 0, 0},
		{"CUD stops at margin", "\x1b[3;8r\x1b[4;1H\x1b[9B", 0, 7},
		{"CUD below margin", "\x1b[3;8r\x1b[9;1H\x1b[9B", 0, 9},
		{"origin CUP", "\x1b[3;8r\x1b[?6h\x1b[2;2H", 1, 3},
		{"origin CUP clamps", "\x1b[3;8r\x1b[?6h\x1b[99;2H", 1, 7},
		{"origin VPA", "\x1b[3;8r\x1b[?6h\x1b[2d", 0, 3},
		{"origin home", "\x1b[3;8r\x1b[5;5H\x1b[?6h", 0, 2},
		{"DECSTBM homes", "\x1b[5;5H\x1b[3;8r", 0, 0},
		{"DECSTBM invalid", "\x1b[5;5H\x1b[8;3r", 4, 4},
		{"SCOSC SCORC", "\x1b[3;4H\x1b[s\x1b[H\x1b[u", 3, 2},
		{"CSI s with params ignored", "\x1b[3;4H\x1b[1;5s\x1b[H\x1b[u", 0, 0},
		{"DECSC DECRC", "\x1b[3;4H\x1b7\x1b[H\x1b8", 3, 2},
		{"IND", "\x1b[3;4H\x1bD", 3, 3},
		{"NEL", "\x1b[3;4H\x1bE", 0, 3},
		{"RI", "\x1b[3;4H\x1bM", 3, 1},
		{"BS at zero", "\b", 0, 0},
		{"VT and FF", "\v\f", 0, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := feed(10, 10, tt.in)
			c := s.Cursor()
			assert.Equal(t, tt.x, c.X, "x")
			assert.Equal(t, tt.y, c.Y, "y")
		})
	}
}

func TestEditing(t *testing.T) {
	const base = "abcdefgh\r\nijklmnop\r\nqrstuvwx"
	tests := []struct {
		name, in, want string
	}{
		{"ED below", "\x1b[2;3H\x1b[J", "abcdefgh\nij\n"},
		{"ED above", "\x1b[2;3H\x1b[1J", "\n   lmnop\nqrstuvwx"},
		{"ED all", "\x1b[2J", "\n\n"},
		{"EL right", "\x1b[2;3H\x1b[K", "abcdefgh\nij\nqrstuvwx"},
		{"EL left", "\x1b[2;3H\x1b[1K", "abcdefgh\n   lmnop\nqrstuvwx"},
		{"EL all", "\x1b[2;3H\x1b[2K", "abcdefgh\n\nqrstuvwx"},
		{"ECH", "\x1b[2;3H\x1b[2X", "abcdefgh\nij  mnop\nqrstuvwx"},
		{"ECH clamps", "\x1b[2;3H\x1b[99X", "abcdefgh\nij\nqrstuvwx"},
		{"ICH", "\x1b[2;3H\x1b[2@", "abcdefgh\nij  klmn\nqrstuvwx"},
		{"DCH", "\x1b[2;3H\x1b[2P", "abcdefgh\nijmnop\nqrstuvwx"},
		{"DCH clamps", "\x1b[2;3H\x1b[99P", "abcdefgh\nij\nqrstuvwx"},
		{"IL", "\x1b[2;3H\x1b[L", "abcdefgh\n\nijklmnop"},
		{"DL", "\x1b[1;3H\x1b[M", "ijklmnop\nqrstuvwx\n"},
		{"SU", "\x1b[S", "ijklmnop\nqrstuvwx\n"},
		{"SD", "\x1b[T", "\nabcdefgh\nijklmnop"},
		{"DECALN", "\x1b#8", "EEEEEEEE\nEEEEEEEE\nEEEEEEEE"},
		{"ED scrollback keeps screen", "\x1b[3J", "abcdefgh\nijklmnop\nqrstuvwx"},
		{"ED unknown", "\x1b[7J", "abcdefgh\nijklmnop\nqrstuvwx"},
		{"ICH over wide", "\x1b[1;1H世\x1b[1;2H\x1b[@", "   cdefg\nijklmnop\nqrstuvwx"},
		{"DCH splits wide", "\x1b[1;3H世\x1b[1;4H\x1b[P", "ab efgh\nijklmnop\nqrstuvwx"},
		{"ICH pushes wide off", "\x1b[1;7H世\x1b[1;1H\x1b[@", " abcdef\nijklmnop\nqrstuvwx"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := feed(8, 3, base+tt.in)
			assert.Equal(t, tt.want, s.String())
		})
	}
}

func TestEraseUsesBackground(t *testing.T) {
	s := feed(4, 2, "abcd\x1b[1;44m\x1b[1;2H\x1b[K")
	row := s.Row(0)
	assert.Equal(t, Indexed(4), row[3].Style.Bg)
	assert.Zero(t, row[3].Style.Attrs)
	assert.True(t, row[0].Style.Bg.IsDefault())

	s = feed(4, 2, "\x1b[41m\x1b[2J")
	assert.Equal(t, Indexed(1), s.Row(1)[0].Style.Bg)

	s = feed(4, 2, "\x1b[42m\n\n")
	assert.Equal(t, Indexed(2), s.Row(1)[2].Style.Bg)
}

func TestScrollRegion(t *testing.T) {
	const base = "1\r\n2\r\n3\r\n4\r\n5"
	tests := []struct {
		name, in, want string
	}{
		{"LF at bottom margin", "\x1b[2;4r\x1b[4;1H\nX", "1\n3\n4\nX\n5"},
		{"LF below region", "\x1b[2;4r\x1b[5;1H\nX", "1\n2\n3\n4\nX"},
		{"RI at top margin", "\x1b[2;4r\x1b[2;1H\x1bMX", "1\nX\n2\n3\n5"},
		{"RI at screen top", "\x1b[1;1H\x1bMX", "X\n1\n2\n3\n4"},
		{"IL in region", "\x1b[2;4r\x1b[3;1H\x1b[LX", "1\n2\nX\n3\n5"},
		{"DL in region", "\x1b[2;4r\x1b[2;1H\x1b[2M", "1\n4\n\n\n5"},
		{"IL outside region", "\x1b[2;4r\x1b[5;1H\x1b[L", "1\n2\n3\n4\n5"},
		{"DL outside region", "\x1b[2;4r\x1b[1;1H\x1b[M", "1\n2\n3\n4\n5"},
		{"SU in region", "\x1b[2;4r\x1b[S", "1\n3\n4\n\n5"},
		{"SD in region", "\x1b[2;4r\x1b[2T", "1\n\n\n2\n5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := feed(4, 5, base+tt.in)
			assert.Equal(t, tt.want, s.String())
			assert.Zero(t, s.HistoryLen())
		})
	}
}

func TestTabs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		x    int
	}{
		{"HT", "\t", 8},
		{"HT twice", "\t\t", 16},
		{"HT at end", "\t\t\t\t", 19},
		{"CHT", "\x1b[2I", 16},
		{"CBT", "\x1b[19G\x1b[2Z", 8},
		{"CBT at zero", "\x1b[Z", 0},
		{"HTS", "\x1b[4G\x1bH\r\t", 3},
		{"TBC current", "\x1b[9G\x1b[g\r\t", 16},
		{"TBC all", "\x1b[3g\r\t", 19},
		{"TBC unknown", "\x1b[5g\r\t", 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := feed(20, 2, tt.in)
			assert.Equal(t, tt.x, s.Cursor().X)
		})
	}
}

func TestReplies(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"DA primary", "\x1b[c", "\x1b[?62;22c"},
		{"DA primary zero", "\x1b[0c", "\x1b[?62;22c"},
		{"DA secondary", "\x1b[>c", "\x1b[>1;10;0c"},
		{"DSR status", "\x1b[5n", "\x1b[0n"},
		{"DSR position", "\x1b[3;4H\x1b[6n", "\x1b[3;4R"},
		{"DSR position origin", "\x1b[2;5r\x1b[?6h\x1b[2;4H\x1b[6n", "\x1b[2;4R"},
		{"DECXCPR", "\x1b[3;4H\x1b[?6n", "\x1b[?3;4R"},
		{"text area size", "\x1b[18t", "\x1b[8;6;10t"},
		{"other window op", "\x1b[22;0t", ""},
		{"DECRQM set", "\x1b[?25$p", "\x1b[?25;1$y"},
		{"DECRQM reset", "\x1b[?1$p", "\x1b[?1;2$y"},
		{"DECRQM unknown", "\x1b[?9999$p", "\x1b[?9999;0$y"},
		{"DECRQM ANSI insert", "\x1b[4h\x1b[4$p", "\x1b[4;1$y"},
		{"DECRQM ANSI newline", "\x1b[20$p", "\x1b[20;2$y"},
		{"DECRQM ANSI unknown", "\x1b[3$p", "\x1b[3;0$y"},
		{"DSR private status", "\x1b[?5n", ""},
		{"DA unknown param", "\x1b[1c", ""},
		{"private n unknown", "\x1b[?7n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(10, 6, Options{})
			var got []byte
			s.OnReply(func(b []byte) { got = append(got, b...) })
			s.Write([]byte(tt.in))
			assert.Equal(t, tt.want, string(got))
		})
	}
	s := New(10, 6, Options{})
	s.OnReply(nil)
	s.Write([]byte("\x1b[c"))
}

func TestCursorStyle(t *testing.T) {
	tests := []struct {
		in    string
		shape CursorShape
		blink bool
	}{
		{"\x1b[1 q", CursorBlock, true},
		{"\x1b[2 q", CursorBlock, false},
		{"\x1b[3 q", CursorUnderline, true},
		{"\x1b[4 q", CursorUnderline, false},
		{"\x1b[5 q", CursorBar, true},
		{"\x1b[6 q", CursorBar, false},
		{"\x1b[5 q\x1b[0 q", CursorBlock, false},
		{"\x1b[?12h", CursorBlock, true},
	}
	for _, tt := range tests {
		c := feed(5, 1, tt.in).Cursor()
		assert.Equal(t, tt.shape, c.Shape, "%q", tt.in)
		assert.Equal(t, tt.blink, c.Blink, "%q", tt.in)
	}
	assert.False(t, feed(5, 1, "\x1b[?25l").Cursor().Visible)
	assert.True(t, feed(5, 1, "\x1b[?25l\x1b[?25h").Cursor().Visible)
}

func TestSaveRestoreState(t *testing.T) {
	s := feed(10, 3, "\x1b[1;31m\x1b(0\x1b[2;3H\x1b7\x1b[0m\x1b(B\x1b[H\x1b8q")
	row := s.Row(1)
	assert.Equal(t, '─', row[2].Rune())
	assert.Equal(t, Indexed(1), row[2].Style.Fg)
	assert.Equal(t, Bold, row[2].Style.Attrs)

	// The pending wrap state survives DECSC and DECRC.
	s = feed(5, 3, "abcde\x1b7\x1b[3;1H\x1b8X")
	assert.Equal(t, "abcde\nX\n", s.String())

	// Restoring without a save homes the cursor.
	s = feed(5, 3, "\x1b[2;2H\x1b8")
	assert.Equal(t, 0, s.Cursor().X)
}

func TestSoftAndHardReset(t *testing.T) {
	s := feed(10, 4, "abc\x1b[?25l\x1b[?6h\x1b[?7l\x1b[4h\x1b[?1h\x1b=\x1b[2;3r\x1b[1;33m\x1b(0\x1b[!p")
	m := s.Modes()
	assert.True(t, s.Cursor().Visible)
	assert.False(t, m.Origin)
	assert.True(t, m.AutoWrap)
	assert.False(t, m.Insert)
	assert.False(t, m.AppCursor)
	assert.False(t, m.AppKeypad)
	assert.Equal(t, 0, s.top)
	assert.Equal(t, 3, s.bottom)
	s.Write([]byte("\x1b[4;1Hq"))
	assert.Equal(t, "abc\n\n\nq", s.String())
	assert.Equal(t, Style{}, s.Row(3)[0].Style)

	s = feed(10, 4, "abc\x1b[?1049h\x1b[?2004h\x1b[31mx\x1bc")
	assert.Equal(t, "\n\n\n", s.String())
	assert.False(t, s.Modes().AltScreen)
	assert.False(t, s.Modes().BracketedPaste)
	assert.Equal(t, Cursor{Visible: true}, s.Cursor())
	s.Write([]byte("y"))
	assert.Equal(t, Style{}, s.Row(0)[0].Style)
	s.Write([]byte("\x1b[?1049h"))
	assert.Equal(t, "\n\n\n", s.String())
}
