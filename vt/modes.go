// modes.go holds terminal modes: SM, RM, DECSET, DECRST, DECRQM and the alternate screen.
package vt

import "strconv"

// MouseMode is the mouse tracking mode a program asked for.
type MouseMode uint8

// The mouse tracking modes.
const (
	MouseOff    MouseMode = iota
	MouseX10              // press only
	MouseNormal           // press and release
	MouseButton           // plus motion while a button is held
	MouseAny              // plus all motion
)

// Modes is the set of terminal modes.
type Modes struct {
	AltScreen      bool
	AppCursor      bool // DECCKM
	AppKeypad      bool // DECKPAM
	BracketedPaste bool
	FocusEvents    bool
	Mouse          MouseMode
	MouseSGR       bool
	Sync           bool // synchronized output is open
	Origin         bool
	AutoWrap       bool // on by default
	Insert         bool // IRM
	NewLine        bool // LNM
}

// The private mode numbers DECSET and DECRST take.
const (
	modeAppCursor      = 1
	modeOrigin         = 6
	modeAutoWrap       = 7
	modeMouseX10       = 9
	modeCursorBlink    = 12
	modeCursorVisible  = 25
	modeAltPlain       = 47
	modeMouseNormal    = 1000
	modeMouseButton    = 1002
	modeMouseAny       = 1003
	modeFocus          = 1004
	modeMouseSGR       = 1006
	modeAltClear       = 1047
	modeSaveCursor     = 1048
	modeAltSave        = 1049
	modeBracketedPaste = 2004
	modeSync           = 2026
)

// The ANSI mode numbers SM and RM take.
const (
	modeInsert  = 4
	modeNewLine = 20
)

func (s *Screen) privateMode(n int, on bool) {
	switch n {
	case modeAppCursor:
		s.modes.AppCursor = on
	case modeOrigin:
		s.modes.Origin = on
		s.cup(0, 0)
	case modeAutoWrap:
		s.modes.AutoWrap = on
		if !on {
			s.pendingWrap = false
		}
	case modeCursorBlink:
		s.blink = on
	case modeCursorVisible:
		s.cursorOn = on
	case modeAltPlain:
		s.switchScreen(on, false)
	case modeAltClear:
		if !on && s.modes.AltScreen {
			clearGrid(&s.alt, s.style)
		}
		s.switchScreen(on, false)
	case modeSaveCursor:
		if on {
			s.saveCursor()
		} else {
			s.restoreCursor()
		}
	case modeAltSave:
		s.altSave(on)
	case modeMouseX10, modeMouseNormal, modeMouseButton, modeMouseAny:
		s.modes.Mouse = MouseOff
		if on {
			s.modes.Mouse = mouseFor(n)
		}
	case modeMouseSGR:
		s.modes.MouseSGR = on
	case modeFocus:
		s.modes.FocusEvents = on
	case modeBracketedPaste:
		s.modes.BracketedPaste = on
	case modeSync:
		s.modes.Sync = on
	}
}

func mouseFor(n int) MouseMode {
	switch n {
	case modeMouseX10:
		return MouseX10
	case modeMouseNormal:
		return MouseNormal
	case modeMouseButton:
		return MouseButton
	}
	return MouseAny
}

// altSave is the xterm alternate screen mode that saves the cursor and clears the alternate grid.
func (s *Screen) altSave(on bool) {
	if on == s.modes.AltScreen {
		return
	}
	if on {
		s.saveCursor()
		s.switchScreen(true, true)
		return
	}
	s.switchScreen(false, false)
	s.restoreCursor()
}

// switchScreen shows the alternate grid when alt is set, else the primary grid.
func (s *Screen) switchScreen(alt, clearAlt bool) {
	if alt == s.modes.AltScreen {
		return
	}
	s.modes.AltScreen = alt
	s.g = &s.pri
	if alt {
		s.g = &s.alt
		if clearAlt {
			clearGrid(&s.alt, s.style)
		}
	}
	s.pendingWrap = false
	s.dirty = true
}

func (s *Screen) ansiMode(on bool) {
	for i := range s.nparams {
		switch s.param(i, 0) {
		case modeInsert:
			s.modes.Insert = on
		case modeNewLine:
			s.modes.NewLine = on
		}
	}
}

// modeState returns the DECRQM answer for a mode: set, reset, or not recognised.
func (s *Screen) modeState(n int, private bool) int {
	const (
		unknown = 0
		set     = 1
		reset   = 2
	)
	var v bool
	switch {
	case !private && n == modeInsert:
		v = s.modes.Insert
	case !private && n == modeNewLine:
		v = s.modes.NewLine
	case !private:
		return unknown
	case n == modeAppCursor:
		v = s.modes.AppCursor
	case n == modeOrigin:
		v = s.modes.Origin
	case n == modeAutoWrap:
		v = s.modes.AutoWrap
	case n == modeCursorBlink:
		v = s.blink
	case n == modeCursorVisible:
		v = s.cursorOn
	case n == modeAltPlain || n == modeAltClear || n == modeAltSave:
		v = s.modes.AltScreen
	case n == modeSaveCursor:
		v = false
	case n == modeMouseX10 || n == modeMouseNormal || n == modeMouseButton || n == modeMouseAny:
		v = s.modes.Mouse == mouseFor(n)
	case n == modeMouseSGR:
		v = s.modes.MouseSGR
	case n == modeFocus:
		v = s.modes.FocusEvents
	case n == modeBracketedPaste:
		v = s.modes.BracketedPaste
	case n == modeSync:
		v = s.modes.Sync
	default:
		return unknown
	}
	if v {
		return set
	}
	return reset
}

// requestMode answers DECRQM.
func (s *Screen) requestMode(private bool) {
	n := s.param(0, 0)
	s.reply = append(s.reply[:0], "\x1b["...)
	if private {
		s.reply = append(s.reply, '?')
	}
	s.reply = strconv.AppendInt(s.reply, int64(n), 10)
	s.reply = append(s.reply, ';')
	s.reply = strconv.AppendInt(s.reply, int64(s.modeState(n, private)), 10)
	s.reply = append(s.reply, "$y"...)
	s.sendReply()
}
