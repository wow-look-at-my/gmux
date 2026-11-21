package vt

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivateModes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Modes
	}{
		{"defaults", "", Modes{AutoWrap: true}},
		{"app cursor", "\x1b[?1h", Modes{AutoWrap: true, AppCursor: true}},
		{"app keypad", "\x1b=", Modes{AutoWrap: true, AppKeypad: true}},
		{"numeric keypad", "\x1b=\x1b>", Modes{AutoWrap: true}},
		{"origin", "\x1b[?6h", Modes{AutoWrap: true, Origin: true}},
		{"autowrap off", "\x1b[?7l", Modes{}},
		{"mouse x10", "\x1b[?9h", Modes{AutoWrap: true, Mouse: MouseX10}},
		{"mouse normal", "\x1b[?1000h", Modes{AutoWrap: true, Mouse: MouseNormal}},
		{"mouse button", "\x1b[?1002h", Modes{AutoWrap: true, Mouse: MouseButton}},
		{"mouse any", "\x1b[?1003h", Modes{AutoWrap: true, Mouse: MouseAny}},
		{"mouse off", "\x1b[?1003h\x1b[?1000l", Modes{AutoWrap: true}},
		{"mouse sgr", "\x1b[?1006h", Modes{AutoWrap: true, MouseSGR: true}},
		{"focus", "\x1b[?1004h", Modes{AutoWrap: true, FocusEvents: true}},
		{"paste", "\x1b[?2004h", Modes{AutoWrap: true, BracketedPaste: true}},
		{"sync", "\x1b[?2026h", Modes{AutoWrap: true, Sync: true}},
		{"sync closed", "\x1b[?2026h\x1b[?2026l", Modes{AutoWrap: true}},
		{"several at once", "\x1b[?1000;1006;2004h", Modes{AutoWrap: true, Mouse: MouseNormal, MouseSGR: true, BracketedPaste: true}},
		{"insert", "\x1b[4h", Modes{AutoWrap: true, Insert: true}},
		{"newline", "\x1b[20h", Modes{AutoWrap: true, NewLine: true}},
		{"insert off", "\x1b[4h\x1b[4l", Modes{AutoWrap: true}},
		{"unknown private", "\x1b[?31337h", Modes{AutoWrap: true}},
		{"alt plain", "\x1b[?47h", Modes{AutoWrap: true, AltScreen: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, feed(10, 3, tt.in).Modes())
		})
	}
}

func TestDECRQMReportsState(t *testing.T) {
	modes := []int{1, 6, 7, 9, 12, 25, 47, 1000, 1002, 1003, 1004, 1006, 1047, 1049, 2004, 2026}
	for _, m := range modes {
		s := New(10, 3, Options{})
		var got string
		s.OnReply(func(b []byte) { got = string(b) })
		on := "\x1b[?" + itoa(m) + "h"
		s.Write([]byte(on + "\x1b[?" + itoa(m) + "$p"))
		assert.Equal(t, "\x1b[?"+itoa(m)+";1$y", got, "mode %d set", m)
		s.Write([]byte("\x1b[?" + itoa(m) + "l\x1b[?" + itoa(m) + "$p"))
		assert.Equal(t, "\x1b[?"+itoa(m)+";2$y", got, "mode %d reset", m)
	}
	s := New(10, 3, Options{})
	var got string
	s.OnReply(func(b []byte) { got = string(b) })
	s.Write([]byte("\x1b[?1048h\x1b[?1048$p"))
	assert.Equal(t, "\x1b[?1048;2$y", got)
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestAltScreen(t *testing.T) {
	for _, mode := range []string{"47", "1047", "1049"} {
		t.Run(mode, func(t *testing.T) {
			s := New(10, 3, Options{})
			s.Write([]byte("primary\r\nline2\x1b[2;3H"))
			s.Write([]byte("\x1b[?" + mode + "h"))
			assert.True(t, s.Modes().AltScreen)
			if mode == "1049" {
				assert.Equal(t, "\n\n", s.String())
			}
			s.Write([]byte("\x1b[2J\x1b[Halt\r\n\r\n\r\nscrolls"))
			assert.Zero(t, s.HistoryLen())
			s.Write([]byte("\x1b[?" + mode + "l"))
			assert.False(t, s.Modes().AltScreen)
			assert.Equal(t, "primary\nline2\n", s.String())
			if mode == "1049" {
				assert.Equal(t, Cursor{X: 2, Y: 1, Visible: true}, s.Cursor())
			}
		})
	}
}

func TestAltScreenContentKept(t *testing.T) {
	// The plain switch keeps the alternate grid between visits.
	s := feed(10, 2, "\x1b[?47hkeep\x1b[?47l\x1b[?47h")
	assert.Equal(t, "keep\n", s.String())

	// The clearing switch clears it on the way out.
	s = feed(10, 2, "\x1b[?1047hgone\x1b[?1047l\x1b[?47h")
	assert.Equal(t, "\n", s.String())

	// Setting a mode that is already set changes nothing.
	s = feed(10, 2, "\x1b[?1049hx\x1b[?1049h")
	assert.Equal(t, "x\n", s.String())
	s = feed(10, 2, "a\x1b[?1049l")
	assert.Equal(t, "a\n", s.String())
	s = feed(10, 2, "\x1b[?47hx\x1b[?47h")
	assert.Equal(t, "x\n", s.String())
}

func TestSaveCursorMode(t *testing.T) {
	s := feed(10, 3, "\x1b[2;5H\x1b[?1048h\x1b[H\x1b[?1048l")
	assert.Equal(t, 4, s.Cursor().X)
	assert.Equal(t, 1, s.Cursor().Y)

	// Each screen has its own saved cursor.
	s = feed(10, 3, "\x1b[2;5H\x1b7\x1b[?47h\x1b[3;3H\x1b7\x1b[?47l\x1b8")
	assert.Equal(t, 4, s.Cursor().X)
	s.Write([]byte("\x1b[?47h\x1b8"))
	assert.Equal(t, 2, s.Cursor().X)
	require.Equal(t, 2, s.Cursor().Y)
}
