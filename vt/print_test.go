package vt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func feed(cols, rows int, in string) *Screen {
	s := New(cols, rows, Options{})
	s.Write([]byte(in))
	return s
}

func TestPrintAndWrap(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		want  string
		x, y  int
		wrap0 bool
	}{
		{"plain", "abc", "abc\n\n", 3, 0, false},
		{"fill last column keeps cursor", "abcde", "abcde\n\n", 4, 0, false},
		{"pending wrap then print", "abcdef", "abcde\nf\n", 1, 1, true},
		{"CR after last column", "abcde\rX", "Xbcde\n\n", 1, 0, false},
		{"LF after last column", "abcde\nX", "abcde\n    X\n", 4, 1, false},
		{"CRLF after last column", "abcde\r\nX", "abcde\nX\n", 1, 1, false},
		{"BS after last column", "abcde\bX", "abcXe\n\n", 4, 0, false},
		{"CUP after last column", "abcde\x1b[1;1HX", "Xbcde\n\n", 1, 0, false},
		{"wrap scrolls", "abcdefghijklmnop", "fghij\nklmno\np", 1, 2, true},
		{"no autowrap", "\x1b[?7labcdefgh", "abcdh\n\n", 4, 0, false},
		{"no autowrap slow path", "\x1b[?7l\x1b[4habcdefgh", "abcdh\n\n", 4, 0, false},
		{"DEC graphics", "\x1b(0lqk\x1b(Bq", "┌─┐q\n\n", 4, 0, false},
		{"shift out G1", "\x1b)0\x0eq\x0fq", "─q\n\n", 2, 0, false},
		{"insert mode", "abc\r\x1b[4hXY", "XYabc\n\n", 2, 0, false},
		{"insert mode push off", "abcde\r\x1b[4hX", "Xabcd\n\n", 1, 0, false},
		{"REP", "a\x1b[3b", "aaaa\n\n", 4, 0, false},
		{"REP nothing printed", "\x1b[3b", "\n\n", 0, 0, false},
		{"BEL ignored in text", "a\x07b", "ab\n\n", 2, 0, false},
		{"LNM newline", "\x1b[20ha\nb", "a\nb\n", 1, 1, false},
		{"DEL ignored", "a\x7fb", "ab\n\n", 2, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := feed(5, 3, tt.in)
			assert.Equal(t, tt.want, s.String())
			c := s.Cursor()
			assert.Equal(t, tt.x, c.X, "x")
			assert.Equal(t, tt.y, c.Y, "y")
			assert.Equal(t, tt.wrap0, s.RowWrapped(0), "wrapped")
		})
	}
}

func TestWideCharacters(t *testing.T) {
	s := feed(5, 2, "a世b")
	row := s.Row(0)
	assert.Equal(t, uint8(2), row[1].Width)
	assert.Equal(t, uint8(0), row[2].Width)
	assert.Equal(t, "世", s.Text(row[1]))
	assert.Equal(t, "", s.Text(row[2]))
	assert.Equal(t, ' ', row[2].Rune())
	assert.Equal(t, "a世b", s.String()[:5])
	assert.Equal(t, 4, s.Cursor().X)

	// A wide char that does not fit in the last column wraps first.
	s = feed(5, 2, "abcd世")
	assert.Equal(t, "abcd\n世", s.String())
	assert.True(t, s.RowWrapped(0))
	assert.Equal(t, 2, s.Cursor().X)

	// Without autowrap it lands one column to the left.
	s = feed(5, 2, "\x1b[?7labcd世")
	assert.Equal(t, "abc世\n", s.String())

	// Overwriting the tail blanks the head, and the reverse.
	s = feed(5, 1, "世\x1b[1;2HX")
	assert.Equal(t, " X", s.String())
	s = feed(5, 1, "世\rX")
	assert.Equal(t, "X", s.String())
	assert.Equal(t, uint8(1), s.Row(0)[1].Width)
	s = feed(5, 1, "a世\x1b[1;1HXY")
	assert.Equal(t, "XY", s.String())

	// A wide char over the head of another blanks its tail.
	s = feed(5, 1, "a世\x1b[1;3H界")
	assert.Equal(t, "a 界", s.String())

	// One column screen.
	s = feed(1, 2, "世")
	assert.Equal(t, uint8(1), s.Row(0)[0].Width)
}

func TestCombining(t *testing.T) {
	s := feed(5, 1, "éx")
	row := s.Row(0)
	assert.Equal(t, "é", s.Text(row[0]))
	assert.Equal(t, 'e', row[0].Rune())
	assert.Equal(t, "x", s.Text(row[1]))
	assert.Equal(t, 2, s.Cursor().X)

	// Identical graphemes share one table entry.
	s = feed(10, 1, "éé")
	assert.Equal(t, s.Row(0)[0], s.Row(0)[1])
	assert.Len(t, s.graphemes, 1)

	// A mark after a pending wrap joins the last column.
	s = feed(3, 2, "abć")
	assert.Equal(t, "ć", s.Text(s.Row(0)[2]))

	// A mark after a wide glyph joins its head.
	s = feed(5, 1, "世️")
	assert.Equal(t, "世️", s.Text(s.Row(0)[0]))

	// A mark at column zero has nothing to join.
	s = feed(5, 1, "́")
	assert.Equal(t, "", s.String())

	// A mark on a blank cell.
	s = feed(5, 1, " ́")
	assert.Equal(t, " ́", s.Text(s.Row(0)[0]))
	s = feed(5, 1, "\x1b[Ć")
	assert.Equal(t, " ́", s.Text(s.Row(0)[0]))

	// A ZWJ sequence stays in one cell.
	s = feed(6, 1, "👨‍👩x")
	assert.Equal(t, "👨‍👩", s.Text(s.Row(0)[0]))
	assert.Equal(t, "x", s.Text(s.Row(0)[2]))
}

func TestGraphemeTableFull(t *testing.T) {
	s := New(5, 1, Options{})
	for i := range maxGraphemes {
		s.graphemes = append(s.graphemes, string(rune(0x10000+i)))
	}
	s.Write([]byte("é"))
	assert.Equal(t, "e", s.Text(s.Row(0)[0]))
}

func TestTextASCIIDoesNotAllocate(t *testing.T) {
	s := feed(5, 1, "a")
	c := s.Row(0)[0]
	allocs := testing.AllocsPerRun(100, func() { _ = s.Text(c) })
	require.Zero(t, allocs)
	assert.Equal(t, "é", s.Text(Cell{r: 'é', Width: 1}))
	assert.Equal(t, ' ', Cell{}.Rune())
}

func TestC1RunesIgnored(t *testing.T) {
	s := feed(5, 1, "a\u0085b")
	assert.Equal(t, "ab", s.String())
}
