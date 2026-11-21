package vt

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// script mixes text, wide glyphs, combining marks, SGR, cursor movement,
// scrolling, OSC and DCS so that a split at any byte crosses every state.
const script = "hello \x1b[1;31mred\x1b[0m world\r\n" +
	"世界 é \x1b[38;2;10;20;30mrgb\x1b[38:5:200mx\x1b[m\r\n" +
	"\x1b]0;title\x07\x1b]2;other\x1b\\\x1bP1$qm\x1b\\\x1b[2;3Hmid\x1b[K" +
	"\x1b[5;20r\x1b[20;1H\n\n\nscroll\x1bM\x1b[?1049hALT\x1b[?1049l" +
	"\x1b(0lqqk\x1b(B\x1b[3Ltab\there\x1b[?25l\xff\xe4\xb8bad\r\n" +
	"\x1b[48;5;4m\x1b[2Jclear\x1b[10;10Hend\x1b7\x1b[1;1H\x1b8!"

func TestSplitAtEveryByte(t *testing.T) {
	whole := New(40, 24, Options{})
	whole.Write([]byte(script))
	want := whole.String()

	byOne := New(40, 24, Options{})
	for i := range len(script) {
		byOne.Write([]byte{script[i]})
	}
	assert.Equal(t, want, byOne.String())
	assert.Equal(t, whole.Cursor(), byOne.Cursor())
	assert.Equal(t, whole.Title(), byOne.Title())
	for y := range 24 {
		require.Equal(t, whole.Row(y), byOne.Row(y), "row %d", y)
	}

	for split := 1; split < len(script); split++ {
		s := New(40, 24, Options{})
		s.Write([]byte(script[:split]))
		s.Write([]byte(script[split:]))
		require.Equal(t, want, s.String(), "split at %d", split)
	}
}

func TestInvalidUTF8(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"a\xffb", "a�b"},
		{"a\xe4\xb8b", "a�b"},
		{"a\xe4\x1b[Cb", "a� b"},
		{"\xc0\xaf", "��"},
		{"\xed\xa0\x80", "�"},
		{"\xe4\xb8\x96", "世"},
		{"\xf0\x9f\x98\x80", "😀"},
		{"a\xe4\x18b", "a�b"},
		{"a\xe4\nb", "a�\n  b"},
	}
	for _, tt := range tests {
		s := feed(10, 2, tt.in)
		assert.Equal(t, tt.want, strings.TrimRight(s.String(), "\n"), "%q", tt.in)
	}
}

func TestAbortAndUnknown(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"CAN aborts CSI", "a\x1b[3\x18m", "am"},
		{"SUB aborts escape", "a\x1b\x1ab", "ab"},
		{"unknown CSI ignored", "a\x1b[5yb", "ab"},
		{"unknown private CSI", "a\x1b[>4;1mb", "ab"},
		{"unknown escape", "a\x1bZb", "ab"},
		{"control inside CSI", "a\x1b[\r2Cb", "a b"},
		{"control inside escape", "ab\x1b\r=X", "Xb"},
		{"too many intermediates", "a\x1b[1$$$pb\x1b(%%%0q", "abq"},
		{"CSI ignore state", "a\x1b[1?2mb", "ab"},
		{"intermediate then digit", "a\x1b[ 1qb", "ab"},
		{"DEL in CSI", "a\x1b[\x7f1Cb", "a b"},
		{"DCS ignored", "a\x1bPq#0;2;0;0;0#0~~\x1b\\b", "ab"},
		{"APC ignored", "a\x1b_payload\x07still\x1b\\b", "ab"},
		{"string ESC other", "a\x1b^pm\x1bcb", "b"},
		{"lone ST", "a\x1b\\b", "ab"},
		{"OSC aborted by CAN", "a\x1b]0;t\x18b", "ab"},
		{"escape intermediate DEL", "a\x1b(\x7f0q\x1b(B", "a─"},
		{"escape intermediate control", "a\x1b(\r0q\x1b(B", "─"},
		{"escape intermediate unknown", "a\x1b#3b", "ab"},
		{"private tertiary DA", "a\x1b[=cb", "ab"},
		{"intermediate with private", "a\x1b[? qb", "ab"},
		{"intermediate unknown", "a\x1b[1'zb", "ab"},
		{"CSI T with many params", "a\x1b[1;2;3;4;5Tb", "ab"},
		{"DECSCUSR unknown", "a\x1b[9 qb", "ab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := feed(10, 1, tt.in)
			assert.Equal(t, tt.want, s.String())
		})
	}
}

func TestHostileInput(t *testing.T) {
	s := New(80, 24, Options{Scrollback: 100})
	s.Write([]byte("\x1b[99999999999;99999999999H\x1b[99999999999@\x1b[99999999999P\x1b[99999999999L"))
	s.Write([]byte("\x1b[99999999999M\x1b[99999999999S\x1b[99999999999T\x1b[99999999999X\x1b[99999999999b"))
	s.Write([]byte("\x1b[" + strings.Repeat("1;", 10000) + "m"))
	s.Write([]byte("\x1b]0;" + strings.Repeat("x", 2<<20) + "\x07"))
	assert.Empty(t, s.Title())
	assert.LessOrEqual(t, cap(s.osc), 4<<20)
	s.Write([]byte("\x1bP" + strings.Repeat("y", 1<<20) + "\x1b\\"))

	rng := rand.New(rand.NewSource(1))
	garbage := make([]byte, 1<<20)
	rng.Read(garbage)
	for i := range garbage {
		if rng.Intn(8) == 0 {
			garbage[i] = 0x1b
		}
	}
	require.NotPanics(t, func() {
		s.Write(garbage)
		s.Resize(3, 2)
		s.Write(garbage[:1<<16])
		s.Resize(100, 50)
		s.Write(garbage[:1<<16])
	})
	assert.LessOrEqual(t, s.HistoryLen(), 100)
	assert.LessOrEqual(t, len(s.graphemes), maxGraphemes)
}

func TestWriteReturnsLength(t *testing.T) {
	s := New(10, 2, Options{})
	n, err := s.Write([]byte("abc\x1b[1m"))
	require.NoError(t, err)
	assert.Equal(t, 7, n)
}

func TestVersion(t *testing.T) {
	s := New(10, 2, Options{})
	v := s.Version()
	s.Write([]byte("a"))
	assert.Greater(t, s.Version(), v)
	v = s.Version()
	s.Write([]byte("\x1b[c\x1b[6n\x1b[?25$p"))
	assert.Equal(t, v, s.Version())
	s.Write(nil)
	assert.Equal(t, v, s.Version())
	s.Resize(10, 2)
	assert.Equal(t, v, s.Version())
	s.Resize(11, 2)
	assert.Greater(t, s.Version(), v)
	v = s.Version()
	s.Reset()
	assert.Greater(t, s.Version(), v)
}
