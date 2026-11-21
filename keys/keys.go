// Package keys turns the input values the outer terminal delivered back into the bytes a program inside a pane reads.
package keys

import (
	"strconv"

	"github.com/wow-look-at-my/tml/input"
)

// Modes are the terminal modes of the receiving program that change what a key or a pointer event is sent as.
type Modes struct {
	// AppCursor sends the arrows and home/end in their application form.
	AppCursor bool
	// BracketedPaste wraps pasted text so the program can tell it from typing.
	BracketedPaste bool
	// MouseSGR reports the pointer in the extended form, which has no coordinate limit.
	MouseSGR bool
}

const (
	esc = "\x1b"
	csi = "\x1b["
	ss3 = "\x1bO"
)

// finals are the keys a sequence names by its last byte.
var finals = map[rune]byte{
	input.KeyUp: 'A', input.KeyDown: 'B', input.KeyRight: 'C', input.KeyLeft: 'D',
	input.KeyHome: 'H', input.KeyEnd: 'F',
	input.KeyF1: 'P', input.KeyF2: 'Q', input.KeyF3: 'R', input.KeyF4: 'S',
}

// tildes are the keys a sequence names by a number and a closing tilde.
var tildes = map[rune]int{
	input.KeyInsert: 2, input.KeyDelete: 3, input.KeyPgUp: 5, input.KeyPgDown: 6,
	input.KeyF5: 15, input.KeyF6: 17, input.KeyF7: 18, input.KeyF8: 19,
	input.KeyF9: 20, input.KeyF10: 21, input.KeyF11: 23, input.KeyF12: 24,
}

// modifier is the parameter a sequence carries for the keys held, which is one more than a bit set.
func modifier(mod input.Mod) int {
	bits := 0
	if mod&input.Shift != 0 {
		bits |= 1
	}
	if mod&input.Alt != 0 {
		bits |= 2
	}
	if mod&input.Ctrl != 0 {
		bits |= 4
	}
	if mod&input.Meta != 0 {
		bits |= 8
	}
	return bits + 1
}

// Encode is the bytes a key press sends. A key with no encoding sends nothing.
func Encode(key input.Key, modes Modes) []byte {
	if final, ok := finals[key.Code]; ok {
		return named(key, final, modes)
	}
	if number, ok := tildes[key.Code]; ok {
		seq := csi + strconv.Itoa(number)
		if key.Mod != 0 {
			seq += ";" + strconv.Itoa(modifier(key.Mod))
		}
		return []byte(seq + "~")
	}
	if key.Code == input.KeyTab && key.Mod == input.Shift {
		return []byte(csi + "Z")
	}
	body := plain(key)
	if body == "" {
		return nil
	}
	if key.Mod&input.Alt != 0 {
		body = esc + body
	}
	return []byte(body)
}

// named encodes an arrow, home, end or one of the first function keys.
func named(key input.Key, final byte, modes Modes) []byte {
	if key.Mod != 0 {
		return []byte(csi + "1;" + strconv.Itoa(modifier(key.Mod)) + string(final))
	}
	function := key.Code >= input.KeyF1 && key.Code <= input.KeyF4
	if function || modes.AppCursor {
		return []byte(ss3 + string(final))
	}
	return []byte(csi + string(final))
}

// plain is a key that is a character or a control character, before alt is applied.
func plain(key input.Key) string {
	ctrl := key.Mod&input.Ctrl != 0
	switch key.Code {
	case input.KeyEnter:
		return "\r"
	case input.KeyTab:
		return "\t"
	case input.KeyEscape:
		return esc
	case input.KeyBackspace:
		if ctrl {
			return "\b"
		}
		return "\x7f"
	case input.KeySpace:
		if ctrl {
			return "\x00"
		}
		return " "
	}
	if ctrl {
		return control(key.Code)
	}
	if key.Text != "" {
		return key.Text
	}
	if key.Code > 0 && key.Code <= 0x10ffff {
		return string(key.Code)
	}
	return ""
}

// control is the control character a key makes with ctrl held.
func control(code rune) string {
	switch {
	case code >= 'a' && code <= 'z':
		return string(code - 'a' + 1)
	case code >= '[' && code <= '_':
		return string(code - '[' + 0x1b)
	case code == '@' || code == '2':
		return "\x00"
	case code >= '3' && code <= '7':
		return string(code - '3' + 0x1b)
	case code == '8' || code == '?':
		return "\x7f"
	case code == '/':
		return "\x1f"
	}
	return ""
}

// Paste is the bytes pasted text sends.
func Paste(text string, modes Modes) []byte {
	if modes.BracketedPaste {
		return []byte(csi + "200~" + text + csi + "201~")
	}
	return []byte(text)
}

// Focus is the bytes a focus change sends to a program that asked to hear about them.
func Focus(gained bool) []byte {
	if gained {
		return []byte(csi + "I")
	}
	return []byte(csi + "O")
}

// Mouse is the bytes a pointer event sends, with x and y counted from the pane's own corner. An event the program's
// encoding cannot carry sends nothing.
func Mouse(event input.Mouse, x, y int, modes Modes) []byte {
	bits, ok := buttonBits(event)
	if !ok || x < 0 || y < 0 {
		return nil
	}
	if event.Mod&input.Shift != 0 {
		bits |= 4
	}
	if event.Mod&input.Alt != 0 {
		bits |= 8
	}
	if event.Mod&input.Ctrl != 0 {
		bits |= 16
	}
	if modes.MouseSGR {
		final := "M"
		if event.Action == input.Release {
			final = "m"
		}
		return []byte(csi + "<" + strconv.Itoa(bits) + ";" + strconv.Itoa(x+1) + ";" + strconv.Itoa(y+1) + final)
	}
	// The form holds each coordinate in one byte, so a cell past its range has no report.
	const limit = 255 - 32 - 1
	if x > limit || y > limit {
		return nil
	}
	if event.Action == input.Release {
		bits = bits&^3 | 3
	}
	return []byte{0x1b, '[', 'M', byte(32 + bits), byte(32 + x + 1), byte(32 + y + 1)}
}

// buttonBits is the button field of a pointer report.
func buttonBits(event input.Mouse) (int, bool) {
	if event.Action == input.Wheel {
		switch event.Button {
		case input.WheelUp:
			return 64, true
		case input.WheelDown:
			return 65, true
		case input.WheelLeft:
			return 66, true
		case input.WheelRight:
			return 67, true
		}
		return 0, false
	}
	bits := 0
	switch event.Button {
	case input.ButtonLeft:
		bits = 0
	case input.ButtonMiddle:
		bits = 1
	case input.ButtonRight:
		bits = 2
	case input.ButtonNone:
		bits = 3
	default:
		return 0, false
	}
	if event.Action == input.Motion {
		bits |= 32
	}
	return bits, true
}
