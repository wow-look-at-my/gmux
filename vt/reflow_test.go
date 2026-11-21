package vt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// all is the scrollback and the screen as text, a line for each row.
func all(s *Screen) string {
	return strings.Join(append(historyText(s), strings.Split(s.String(), "\n")...), "\n")
}

func TestANarrowerScreenCutsLinesAndAWiderOneJoinsThem(t *testing.T) {
	s := New(10, 4, Options{})
	s.Write([]byte("0123456789abcdefghij\r\nxyz"))
	require.Equal(t, "0123456789\nabcdefghij\nxyz\n", s.String())

	s.Resize(5, 4)
	assert.Equal(t, []string{"01234"}, historyText(s), "the rows that no longer fit go up into the scrollback")
	assert.Equal(t, "56789\nabcde\nfghij\nxyz", s.String())
	assert.Equal(t, Cursor{X: 3, Y: 3, Visible: true}, s.Cursor())
	assert.True(t, s.HistoryWrapped(0))
	assert.Equal(t, []bool{true, true, false, false},
		[]bool{s.RowWrapped(0), s.RowWrapped(1), s.RowWrapped(2), s.RowWrapped(3)}, "the long line ends on its fourth row")

	s.Resize(20, 4)
	assert.Zero(t, s.HistoryLen(), "nothing is lost: the rows come back")
	assert.Equal(t, "0123456789abcdefghij\nxyz\n\n", s.String())
	assert.Equal(t, Cursor{X: 3, Y: 1, Visible: true}, s.Cursor())

	s.Resize(7, 4)
	assert.Equal(t, "0123456\n789abcd\nefghij\nxyz", s.String())
	s.Write([]byte("!"))
	assert.Equal(t, "0123456\n789abcd\nefghij\nxyz!", s.String(), "the cursor is still after the text")
}

func TestReflowReachesTheScrollback(t *testing.T) {
	s := New(8, 2, Options{})
	s.Write([]byte("aaaaaaaabbbb\r\ncccc\r\ndddd\r\n$ "))
	require.Equal(t, []string{"aaaaaaaa", "bbbb", "cccc"}, historyText(s))

	s.Resize(4, 2)
	assert.Equal(t, "aaaa\naaaa\nbbbb\ncccc\ndddd\n$", all(s))
	s.Resize(12, 2)
	assert.Equal(t, "aaaaaaaabbbb\ncccc\ndddd\n$", all(s))
	assert.Equal(t, Cursor{X: 2, Y: 1, Visible: true}, s.Cursor(), "the cursor is still after the prompt's space")
}

func TestReflowKeepsStylesAndBlankLines(t *testing.T) {
	s := New(6, 6, Options{})
	s.Write([]byte("\x1b[31mredred\x1b[0mplain\r\n\r\nend"))
	s.Resize(3, 6)
	assert.Equal(t, "red\nred\npla\nin\n\nend", s.String())
	red, _ := s.Row(1)[2].Style.Fg.Index()
	assert.Equal(t, uint8(1), red)
	assert.True(t, s.Row(2)[0].Style.Fg.IsDefault())
	s.Resize(11, 6)
	assert.Equal(t, "redredplain\n\nend\n\n\n", s.String(), "a screen that is not full keeps its text at the top")
}

func TestReflowKeepsACursorThatWaitsToWrap(t *testing.T) {
	s := New(6, 3, Options{})
	s.Write([]byte("abcdef"))
	s.Resize(3, 3)
	assert.Equal(t, "abc\ndef\n", s.String())
	s.Write([]byte("g"))
	assert.Equal(t, "abc\ndef\ng", s.String(), "the next glyph goes to a new row, not over the last one")

	s.Resize(10, 3)
	assert.Equal(t, "abcdefg\n\n", s.String())
	assert.Equal(t, 7, s.Cursor().X)
}

func TestReflowKeepsTheCursorOnItsCellInsideALine(t *testing.T) {
	s := New(10, 4, Options{})
	s.Write([]byte("0123456789abcd\x1b[1;4H"))
	s.Resize(4, 4)
	assert.Equal(t, "0123\n4567\n89ab\ncd", all(s))
	assert.Equal(t, Cursor{X: 3, Y: 0, Visible: true}, s.Cursor())
	s.Write([]byte("#"))
	assert.Equal(t, "012#\n4567\n89ab\ncd", all(s))

	// The cursor row stays on screen. Text below it that no longer fits is lost from the bottom.
	s.Resize(2, 3)
	assert.Equal(t, "01\n2#\n45\n67", all(s))
	assert.Equal(t, Cursor{X: 1, Y: 0, Visible: true}, s.Cursor())
	assert.Equal(t, []string{"01"}, historyText(s))
}

func TestReflowRespectsTheScrollbackLimit(t *testing.T) {
	s := New(8, 1, Options{Scrollback: 2})
	s.Write([]byte("aaaaaaaa\r\nbbbbbbbb\r\ncccc"))
	s.Resize(4, 1)
	assert.Equal(t, []string{"bbbb", "bbbb"}, historyText(s), "the oldest rows leave first")
	assert.Equal(t, "cccc", s.String())
}

func TestReflowLeavesTheAlternateScreenAlone(t *testing.T) {
	s := New(8, 2, Options{})
	s.Write([]byte("0123456789\x1b[?1049h\x1b[HALTERNATE"))
	s.Resize(4, 2)
	assert.Equal(t, "ALTE\nE", s.String(), "a full-screen program redraws itself: its rows are only cut")
	s.Write([]byte("\x1b[?1049l"))
	assert.Equal(t, "0123\n4567\n89", all(s))
}
