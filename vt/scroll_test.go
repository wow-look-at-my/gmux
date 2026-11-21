package vt

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func historyText(s *Screen) []string {
	var out []string
	for i := range s.HistoryLen() {
		out = append(out, strings.TrimRight(s.rowText(s.HistoryRow(i)), " "))
	}
	return out
}

func lines(n int) string {
	var b strings.Builder
	for i := range n {
		if i > 0 {
			b.WriteString("\r\n")
		}
		b.WriteString(strconv.Itoa(i))
	}
	return b.String()
}

func TestScrollback(t *testing.T) {
	s := New(5, 3, Options{})
	s.Write([]byte(lines(6)))
	assert.Equal(t, []string{"0", "1", "2"}, historyText(s))
	assert.Equal(t, "3\n4\n5", s.String())
	assert.Nil(t, s.HistoryRow(-1))
	assert.Nil(t, s.HistoryRow(3))
	assert.False(t, s.HistoryWrapped(3))
	assert.Nil(t, s.Row(3))
	assert.Nil(t, s.Row(-1))
	assert.False(t, s.RowWrapped(9))

	// Soft wraps travel into history.
	s = New(3, 2, Options{})
	s.Write([]byte("abcdefghi"))
	assert.Equal(t, []string{"abc"}, historyText(s))
	assert.True(t, s.HistoryWrapped(0))
	assert.True(t, s.RowWrapped(0))
	assert.False(t, s.RowWrapped(1))
}

func TestScrollbackCap(t *testing.T) {
	s := New(5, 2, Options{Scrollback: 3})
	s.Write([]byte(lines(10)))
	assert.Equal(t, []string{"5", "6", "7"}, historyText(s))
	assert.Equal(t, "8\n9", s.String())

	s = New(5, 2, Options{Scrollback: -1})
	s.Write([]byte(lines(10)))
	assert.Zero(t, s.HistoryLen())
	assert.Equal(t, "8\n9", s.String())

	s = New(5, 2, Options{})
	assert.Equal(t, defaultScrollback, s.hist.max)
}

func TestScrollbackClear(t *testing.T) {
	s := New(5, 2, Options{Scrollback: 3})
	s.Write([]byte(lines(10) + "\x1b[3J"))
	assert.Zero(t, s.HistoryLen())
	s.Write([]byte("\r\na\r\nb\r\nc"))
	assert.Equal(t, []string{"8", "9", "a"}, historyText(s))
}

func TestScrollbackOnlyFromTop(t *testing.T) {
	s := New(5, 4, Options{})
	s.Write([]byte("\x1b[2;4r\x1b[4;1H" + lines(6)))
	assert.Zero(t, s.HistoryLen())
	s.Write([]byte("\x1b[1;3r\x1b[3;1H\nx"))
	assert.Equal(t, 1, s.HistoryLen())
}

func TestResizeShorterAndTaller(t *testing.T) {
	s := New(5, 5, Options{})
	s.Write([]byte(lines(3)))
	// Blank lines below the cursor go first.
	s.Resize(5, 3)
	assert.Equal(t, "0\n1\n2", s.String())
	assert.Zero(t, s.HistoryLen())

	s.Resize(5, 2)
	assert.Equal(t, "1\n2", s.String())
	assert.Equal(t, []string{"0"}, historyText(s))
	assert.Equal(t, 1, s.Cursor().Y)

	// Lines come back from history and the cursor moves down with them.
	s.Resize(5, 4)
	assert.Equal(t, "0\n1\n2\n", s.String())
	assert.Zero(t, s.HistoryLen())
	assert.Equal(t, 2, s.Cursor().Y)
	s.Write([]byte("x"))
	assert.Equal(t, "0\n1\n2x\n", s.String())
}

func TestResizeKeepsLinesBelowCursor(t *testing.T) {
	s := New(5, 4, Options{})
	s.Write([]byte("a\r\nb\r\nc\r\nd\x1b[1;1H"))
	s.Resize(5, 2)
	assert.Equal(t, "a\nb", s.String())
	assert.Zero(t, s.HistoryLen())
}

func TestResizeWidth(t *testing.T) {
	s := New(6, 2, Options{})
	s.Write([]byte("abcd世\x1b[2;1H\x1b[44mx"))
	// The wide glyph has no room at the end of the narrower row, and goes to a row of its own.
	s.Resize(5, 2)
	assert.Equal(t, "世\nx", s.String())
	assert.Equal(t, []string{"abcd"}, historyText(s))
	assert.Equal(t, uint8(1), s.HistoryRow(0)[4].Width)
	assert.True(t, s.HistoryWrapped(0))
	assert.Len(t, s.Row(0), 5)

	s.Resize(8, 2)
	assert.Equal(t, "abcd世\nx", s.String())
	assert.Zero(t, s.HistoryLen())
	assert.Len(t, s.Row(1), 8)
	assert.True(t, s.Row(1)[7].Style.Bg.IsDefault())

	// Clamps the cursor.
	s.Write([]byte("\x1b[2;8H"))
	s.Resize(3, 2)
	assert.Equal(t, 2, s.Cursor().X)

	// Below one clamps to one.
	s.Resize(0, -5)
	cols, rows := s.Size()
	assert.Equal(t, [2]int{1, 1}, [2]int{cols, rows})
	s.Write([]byte("zz"))
	assert.Equal(t, "z", s.String())
}

func TestResizeTabsAndRegion(t *testing.T) {
	s := New(10, 5, Options{})
	s.Write([]byte("\x1b[3g\x1b[3G\x1bH\x1b[2;3r"))
	s.Resize(20, 6)
	s.Write([]byte("\r\t"))
	assert.Equal(t, 2, s.Cursor().X)
	s.Write([]byte("\t"))
	assert.Equal(t, 16, s.Cursor().X)
	assert.Equal(t, 0, s.top)
	assert.Equal(t, 5, s.bottom)
}

func TestResizeHistoryWidths(t *testing.T) {
	s := New(4, 2, Options{})
	s.Write([]byte(lines(4)))
	s.Resize(8, 2)
	assert.Len(t, s.HistoryRow(0), 8)
	s.Write([]byte("\r\nabcdefgh"))
	assert.Len(t, s.HistoryRow(2), 8)
	s.Resize(8, 6)
	assert.Equal(t, "0\n1\n2\n3\nabcdefgh\n", s.String())
	for y := range 6 {
		require.Len(t, s.Row(y), 8)
	}
}

func TestResizeAltScreen(t *testing.T) {
	s := New(5, 4, Options{})
	s.Write([]byte(lines(4) + "\x1b[?1049h\x1b[4;1Halt"))
	s.Resize(4, 2)
	assert.True(t, s.Modes().AltScreen)
	assert.Equal(t, "\n", s.String())
	assert.Equal(t, 1, s.Cursor().Y)
	s.Write([]byte("\x1b[?1049l"))
	assert.Equal(t, "2\n3", s.String())
	assert.Equal(t, []string{"0", "1"}, historyText(s))
	assert.Equal(t, 1, s.Cursor().Y)

	s.Write([]byte("\x1b[?1049h"))
	s.Resize(4, 4)
	assert.Len(t, s.Row(3), 4)
	s.Write([]byte("\x1b[?1049l"))
	assert.Equal(t, "0\n1\n2\n3", s.String())
}
