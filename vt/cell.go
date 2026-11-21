// cell.go holds colours, styles, cells and the interned grapheme table.
package vt

import "strings"

// Color is a terminal colour. The zero value is the default colour.
type Color uint32

const (
	colorKindShift = 24
	colorIndexed   = 1 << colorKindShift
	colorRGB       = 2 << colorKindShift
)

// Indexed returns a palette colour.
func Indexed(i uint8) Color { return Color(colorIndexed | uint32(i)) }

// RGB returns a true colour.
func RGB(r, g, b uint8) Color {
	return Color(colorRGB | uint32(r)<<16 | uint32(g)<<8 | uint32(b))
}

// IsDefault reports whether c is the default colour.
func (c Color) IsDefault() bool { return c == 0 }

// Index returns the palette index of an indexed colour.
func (c Color) Index() (i uint8, ok bool) {
	if c&^0xff == colorIndexed {
		return uint8(c), true
	}
	return 0, false
}

// RGB returns the components of a true colour.
func (c Color) RGB() (r, g, b uint8, ok bool) {
	if c&^0xffffff == colorRGB {
		return uint8(c >> 16), uint8(c >> 8), uint8(c), true
	}
	return 0, 0, 0, false
}

// Attr is a set of text attributes.
type Attr uint16

// The text attributes.
const (
	Bold Attr = 1 << iota
	Faint
	Italic
	Underline
	Blink
	Reverse
	Invisible
	Strikethrough
)

// Style is the colours and attributes of a cell.
type Style struct {
	Fg, Bg Color
	Attrs  Attr
}

// Cell is one grid position.
type Cell struct {
	r     rune   // the first rune; zero for a blank cell or a wide tail
	g     uint16 // one plus the grapheme table index, or zero for a single rune
	Width uint8
	Style Style
}

// Rune returns the cell's first rune, or a space for a blank cell.
func (c Cell) Rune() rune {
	if c.r == 0 {
		return ' '
	}
	return c.r
}

// maxGraphemes is the size limit of the grapheme table; g is a uint16.
const maxGraphemes = 1<<16 - 1

// asciiText maps each ASCII byte to a one-byte string so Text never allocates for ASCII.
var asciiText = func() (t [128]string) {
	const all = "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f" +
		"\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f" +
		" !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~\x7f"
	for i := range t {
		t[i] = all[i : i+1]
	}
	return t
}()

// Text returns the cell's grapheme: a space for a blank and "" for a wide tail.
func (s *Screen) Text(c Cell) string {
	switch {
	case c.Width == 0:
		return ""
	case c.g != 0 && int(c.g) <= len(s.graphemes):
		return s.graphemes[c.g-1]
	case c.r == 0:
		return " "
	case c.r < 0x80:
		return asciiText[c.r]
	}
	return string(c.r)
}

// intern returns the table reference for str, or zero when the table is full.
func (s *Screen) intern(str string) uint16 {
	if g, ok := s.gindex[str]; ok {
		return g
	}
	if len(s.graphemes) >= maxGraphemes {
		return 0
	}
	if s.gindex == nil {
		s.gindex = make(map[string]uint16)
	}
	s.graphemes = append(s.graphemes, str)
	g := uint16(len(s.graphemes))
	s.gindex[str] = g
	return g
}

// endsWithZWJ reports whether the cell's grapheme ends in a zero width joiner.
func (s *Screen) endsWithZWJ(c Cell) bool {
	return c.g != 0 && strings.HasSuffix(s.Text(c), "‍")
}
