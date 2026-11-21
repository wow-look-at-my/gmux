package vt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSGR(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Style
	}{
		{"reset empty", "\x1b[1;31m\x1b[m", Style{}},
		{"reset zero", "\x1b[1;31m\x1b[0m", Style{}},
		{"bold", "\x1b[1m", Style{Attrs: Bold}},
		{"faint", "\x1b[2m", Style{Attrs: Faint}},
		{"italic", "\x1b[3m", Style{Attrs: Italic}},
		{"underline", "\x1b[4m", Style{Attrs: Underline}},
		{"curly underline", "\x1b[4:3m", Style{Attrs: Underline}},
		{"no underline sub-style", "\x1b[4m\x1b[4:0m", Style{}},
		{"double underline", "\x1b[21m", Style{Attrs: Underline}},
		{"blink", "\x1b[5m", Style{Attrs: Blink}},
		{"rapid blink", "\x1b[6m", Style{Attrs: Blink}},
		{"reverse", "\x1b[7m", Style{Attrs: Reverse}},
		{"invisible", "\x1b[8m", Style{Attrs: Invisible}},
		{"strike", "\x1b[9m", Style{Attrs: Strikethrough}},
		{"all attrs off", "\x1b[1;2;3;4;5;7;8;9m\x1b[22;23;24;25;27;28;29m", Style{}},
		{"fg", "\x1b[31m", Style{Fg: Indexed(1)}},
		{"bg", "\x1b[42m", Style{Bg: Indexed(2)}},
		{"bright fg", "\x1b[91m", Style{Fg: Indexed(9)}},
		{"bright bg", "\x1b[107m", Style{Bg: Indexed(15)}},
		{"default colours", "\x1b[31;42m\x1b[39;49m", Style{}},
		{"256 semicolon", "\x1b[38;5;200;48;5;17m", Style{Fg: Indexed(200), Bg: Indexed(17)}},
		{"256 colon", "\x1b[38:5:200;48:5:17m", Style{Fg: Indexed(200), Bg: Indexed(17)}},
		{"rgb semicolon", "\x1b[38;2;1;2;3;48;2;4;5;6m", Style{Fg: RGB(1, 2, 3), Bg: RGB(4, 5, 6)}},
		{"rgb colon", "\x1b[38:2:1:2:3m", Style{Fg: RGB(1, 2, 3)}},
		{"rgb colon colourspace", "\x1b[48:2::4:5:6m", Style{Bg: RGB(4, 5, 6)}},
		{"rgb colon colourspace id", "\x1b[38:2:0:7:8:9m", Style{Fg: RGB(7, 8, 9)}},
		{"rgb clamps", "\x1b[38;2;300;2;3m", Style{Fg: RGB(255, 2, 3)}},
		{"rgb then bold", "\x1b[38;2;1;2;3;1m", Style{Fg: RGB(1, 2, 3), Attrs: Bold}},
		{"underline colour semicolon", "\x1b[58;2;1;2;3;1m", Style{Attrs: Bold}},
		{"underline colour 256", "\x1b[58;5;9;3m", Style{Attrs: Italic}},
		{"underline colour colon", "\x1b[58:2::1:2:3;1m", Style{Attrs: Bold}},
		{"truncated rgb", "\x1b[38;2;1m", Style{Fg: RGB(1, 0, 0)}},
		{"bad colour kind", "\x1b[38;7;1m", Style{Attrs: Reverse | Bold}},
		{"bad colon kind", "\x1b[38:7:1;3m", Style{Attrs: Italic}},
		{"short colon", "\x1b[38:5;3m", Style{Attrs: Italic}},
		{"short colon rgb", "\x1b[38:2:1:2;3m", Style{Attrs: Italic}},
		{"unknown param", "\x1b[1;77;3m", Style{Attrs: Bold | Italic}},
		{"missing param resets", "\x1b[1m\x1b[;3m", Style{Attrs: Italic}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := feed(5, 1, tt.in+"x")
			assert.Equal(t, tt.want, s.Row(0)[0].Style)
		})
	}
}

func TestColor(t *testing.T) {
	var c Color
	assert.True(t, c.IsDefault())
	_, ok := c.Index()
	assert.False(t, ok)
	_, _, _, ok = c.RGB()
	assert.False(t, ok)

	i, ok := Indexed(42).Index()
	assert.True(t, ok)
	assert.Equal(t, uint8(42), i)
	_, _, _, ok = Indexed(42).RGB()
	assert.False(t, ok)
	assert.False(t, Indexed(0).IsDefault())

	r, g, b, ok := RGB(1, 2, 3).RGB()
	assert.True(t, ok)
	assert.Equal(t, [3]uint8{1, 2, 3}, [3]uint8{r, g, b})
	_, ok = RGB(0, 0, 0).Index()
	assert.False(t, ok)
	assert.False(t, RGB(0, 0, 0).IsDefault())
}
