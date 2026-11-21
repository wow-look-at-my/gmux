package keys

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wow-look-at-my/tml/input"
	"github.com/wow-look-at-my/tml/term"
)

func TestEncodeSendsWhatATerminalSends(t *testing.T) {
	cases := []struct {
		key   string
		modes Modes
		want  string
	}{
		{"a", Modes{}, "a"},
		{"shift+a", Modes{}, "A"},
		{"enter", Modes{}, "\r"},
		{"tab", Modes{}, "\t"},
		{"shift+tab", Modes{}, "\x1b[Z"},
		{"esc", Modes{}, "\x1b"},
		{"backspace", Modes{}, "\x7f"},
		{"ctrl+backspace", Modes{}, "\b"},
		{"alt+backspace", Modes{}, "\x1b\x7f"},
		{"space", Modes{}, " "},
		{"ctrl+space", Modes{}, "\x00"},
		{"ctrl+c", Modes{}, "\x03"},
		{"ctrl+z", Modes{}, "\x1a"},
		{"ctrl+[", Modes{}, "\x1b"},
		{"ctrl+\\", Modes{}, "\x1c"},
		{"ctrl+_", Modes{}, "\x1f"},
		{"ctrl+/", Modes{}, "\x1f"},
		{"ctrl+@", Modes{}, "\x00"},
		{"ctrl+2", Modes{}, "\x00"},
		{"ctrl+3", Modes{}, "\x1b"},
		{"ctrl+8", Modes{}, "\x7f"},
		{"ctrl+?", Modes{}, "\x7f"},
		{"ctrl+1", Modes{}, ""},
		{"alt+x", Modes{}, "\x1bx"},
		{"alt+enter", Modes{}, "\x1b\r"},
		{"ctrl+alt+c", Modes{}, "\x1b\x03"},
		{"up", Modes{}, "\x1b[A"},
		{"down", Modes{AppCursor: true}, "\x1bOB"},
		{"ctrl+right", Modes{AppCursor: true}, "\x1b[1;5C"},
		{"shift+left", Modes{}, "\x1b[1;2D"},
		{"home", Modes{}, "\x1b[H"},
		{"end", Modes{AppCursor: true}, "\x1bOF"},
		{"f1", Modes{}, "\x1bOP"},
		{"shift+f4", Modes{}, "\x1b[1;2S"},
		{"f5", Modes{}, "\x1b[15~"},
		{"ctrl+f12", Modes{}, "\x1b[24;5~"},
		{"insert", Modes{}, "\x1b[2~"},
		{"delete", Modes{}, "\x1b[3~"},
		{"alt+delete", Modes{}, "\x1b[3;3~"},
		{"pgup", Modes{}, "\x1b[5~"},
		{"pgdown", Modes{}, "\x1b[6~"},
		{"é", Modes{}, "é"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, string(Encode(input.ParseKey(c.key), c.modes)), c.key)
	}
}

func TestEncodeSendsACodeWithNoText(t *testing.T) {
	assert.Equal(t, "q", string(Encode(input.Key{Code: 'q'}, Modes{})))
	assert.Empty(t, Encode(input.Key{}, Modes{}))
	assert.Empty(t, Encode(input.Key{Code: 0x7fffffff}, Modes{}))
}

// A key that went out and came back through the decoder is the key that went out.
func TestEncodeRoundTripsThroughTheDecoder(t *testing.T) {
	names := []string{
		"a", "shift+a", "enter", "tab", "shift+tab", "backspace", "space", "ctrl+space", "ctrl+c", "ctrl+\\",
		"alt+x", "up", "down", "left", "right", "ctrl+right", "shift+alt+up", "home", "end", "f1", "f4", "f5",
		"ctrl+f12", "insert", "delete", "pgup", "pgdown", "alt+delete", "é",
	}
	for _, name := range names {
		key := input.ParseKey(name)
		var decoder term.Decoder
		values := decoder.Feed(Encode(key, Modes{}))
		values = append(values, decoder.Flush()...)
		require.Len(t, values, 1, name)
		got, ok := values[0].(input.Key)
		require.True(t, ok, name)
		assert.Equal(t, key.Keystroke(), got.Keystroke(), name)
	}
}

func TestPasteWrapsTextOnlyForAProgramThatAsked(t *testing.T) {
	assert.Equal(t, "hi", string(Paste("hi", Modes{})))
	assert.Equal(t, "\x1b[200~hi\x1b[201~", string(Paste("hi", Modes{BracketedPaste: true})))
}

func TestFocus(t *testing.T) {
	assert.Equal(t, "\x1b[I", string(Focus(true)))
	assert.Equal(t, "\x1b[O", string(Focus(false)))
}

func TestMouseInTheExtendedForm(t *testing.T) {
	sgr := Modes{MouseSGR: true}
	press := input.Mouse{Button: input.ButtonLeft, Action: input.Press}
	assert.Equal(t, "\x1b[<0;4;6M", string(Mouse(press, 3, 5, sgr)))

	release := input.Mouse{Button: input.ButtonRight, Action: input.Release, Mod: input.Ctrl}
	assert.Equal(t, "\x1b[<18;1;1m", string(Mouse(release, 0, 0, sgr)))

	drag := input.Mouse{Button: input.ButtonMiddle, Action: input.Motion, Mod: input.Shift | input.Alt}
	assert.Equal(t, "\x1b[<45;300;2M", string(Mouse(drag, 299, 1, sgr)))

	hover := input.Mouse{Button: input.ButtonNone, Action: input.Motion}
	assert.Equal(t, "\x1b[<35;2;2M", string(Mouse(hover, 1, 1, sgr)))

	for button, bits := range map[input.Button]string{
		input.WheelUp: "64", input.WheelDown: "65", input.WheelLeft: "66", input.WheelRight: "67",
	} {
		wheel := input.Mouse{Button: button, Action: input.Wheel}
		assert.Equal(t, "\x1b[<"+bits+";1;1M", string(Mouse(wheel, 0, 0, sgr)))
	}
}

func TestMouseInTheLegacyForm(t *testing.T) {
	press := input.Mouse{Button: input.ButtonLeft, Action: input.Press}
	assert.Equal(t, "\x1b[M !!", string(Mouse(press, 0, 0, Modes{})))

	release := input.Mouse{Button: input.ButtonLeft, Action: input.Release}
	assert.Equal(t, "\x1b[M#\"#", string(Mouse(release, 1, 2, Modes{})))

	assert.Empty(t, Mouse(press, 400, 0, Modes{}), "a cell past the form's range has no report")
}

func TestMouseSendsNothingForAnEventWithNoReport(t *testing.T) {
	assert.Empty(t, Mouse(input.Mouse{Button: input.ButtonLeft}, -1, 0, Modes{MouseSGR: true}))
	assert.Empty(t, Mouse(input.Mouse{Button: input.Button(99)}, 0, 0, Modes{MouseSGR: true}))
	assert.Empty(t, Mouse(input.Mouse{Button: input.ButtonLeft, Action: input.Wheel}, 0, 0, Modes{MouseSGR: true}))
}
