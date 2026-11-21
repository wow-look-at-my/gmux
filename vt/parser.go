// parser.go holds the escape sequence state machine and the entry point Write.
package vt

import "unicode/utf8"

type pstate uint8

const (
	stGround pstate = iota
	stEscape
	stEscInter
	stCSIEntry
	stCSIParam
	stCSIInter
	stCSIIgnore
	stOSC
	stOSCEsc
	stString
	stStringEsc
)

const (
	maxParams   = 32
	maxParamVal = 65535
	maxInter    = 2
	maxOSC      = 1<<20 + 64
)

// parser is the state that survives between Write calls.
type parser struct {
	state    pstate
	params   [maxParams]int32 // a missing parameter is negative
	psub     [maxParams]bool  // the parameter follows a colon
	nparams  int
	priv     byte
	inter    [maxInter]byte
	ninter   int
	interBad bool
	osc      []byte
	oscBad   bool
	u8       [utf8.UTFMax]byte
	u8n      int
	u8need   int
}

// Write feeds program output. It always returns len(p), nil.
func (s *Screen) Write(p []byte) (int, error) {
	for i := 0; i < len(p); {
		b := p[i]
		if s.state == stGround && s.u8need == 0 && b >= 0x20 && b < 0x7f {
			if s.fastASCII() {
				j := i + 1
				for j < len(p) && p[j] >= 0x20 && p[j] < 0x7f {
					j++
				}
				s.printASCII(p[i:j])
				i = j
				continue
			}
			s.printRune(rune(b))
			i++
			continue
		}
		s.step(b)
		i++
	}
	if s.dirty {
		s.version++
		s.dirty = false
	}
	return len(p), nil
}

func (s *Screen) step(b byte) {
	switch b {
	case 0x18, 0x1a:
		s.flushUTF8()
		s.state = stGround
		return
	case 0x1b:
		s.flushUTF8()
		switch s.state {
		case stOSC:
			s.state = stOSCEsc
		case stString:
			s.state = stStringEsc
		default:
			s.enterEscape()
		}
		return
	}
	switch s.state {
	case stGround:
		s.ground(b)
	case stEscape:
		s.escape(b)
	case stEscInter:
		s.escInter(b)
	case stCSIEntry, stCSIParam, stCSIInter, stCSIIgnore:
		s.csiByte(b)
	case stOSC:
		s.oscByte(b)
	case stOSCEsc:
		s.dispatchOSC()
		s.enterEscape()
		if b != '\\' {
			s.escape(b)
		} else {
			s.state = stGround
		}
	case stString:
	case stStringEsc:
		s.enterEscape()
		if b != '\\' {
			s.escape(b)
		} else {
			s.state = stGround
		}
	}
}

func (s *Screen) enterEscape() {
	s.state = stEscape
	s.ninter, s.interBad = 0, false
}

// flushUTF8 emits a replacement character for an unfinished UTF-8 sequence.
func (s *Screen) flushUTF8() {
	if s.u8need > 0 {
		s.u8need, s.u8n = 0, 0
		s.printRune(utf8.RuneError)
	}
}

func (s *Screen) ground(b byte) {
	if s.u8need > 0 {
		if b&0xc0 == 0x80 {
			s.u8[s.u8n] = b
			s.u8n++
			if s.u8n == s.u8need {
				r, _ := utf8.DecodeRune(s.u8[:s.u8n])
				s.u8need, s.u8n = 0, 0
				s.printRune(r)
			}
			return
		}
		s.flushUTF8()
	}
	switch {
	case b < 0x20:
		s.execute(b)
	case b < 0x7f:
		s.printRune(rune(b))
	case b == 0x7f:
	case b >= 0xc2 && b <= 0xdf:
		s.startUTF8(b, 2)
	case b >= 0xe0 && b <= 0xef:
		s.startUTF8(b, 3)
	case b >= 0xf0 && b <= 0xf4:
		s.startUTF8(b, 4)
	default:
		s.printRune(utf8.RuneError)
	}
}

func (s *Screen) startUTF8(b byte, n int) {
	s.u8[0], s.u8n, s.u8need = b, 1, n
}

// execute runs a C0 control.
func (s *Screen) execute(b byte) {
	switch b {
	case 0x07:
		if s.onBell != nil {
			s.onBell()
		}
	case 0x08:
		s.dirty = true
		if s.pendingWrap {
			s.pendingWrap = false
		}
		if s.x > 0 {
			s.x--
		}
	case 0x09:
		s.tabForward(1)
	case 0x0a, 0x0b, 0x0c:
		s.index()
		if s.modes.NewLine {
			s.x = 0
		}
		s.dirty = true
	case 0x0d:
		s.x, s.pendingWrap, s.dirty = 0, false, true
	case 0x0e:
		s.gl = 1
	case 0x0f:
		s.gl = 0
	}
}

func (s *Screen) escape(b byte) {
	switch {
	case b < 0x20:
		s.execute(b)
		return
	case b < 0x30:
		s.state = stEscInter
		s.collectInter(b)
		return
	}
	s.state = stGround
	switch b {
	case '[':
		s.state, s.nparams, s.priv = stCSIEntry, 0, 0
		s.ninter, s.interBad = 0, false
	case ']':
		s.state, s.osc, s.oscBad = stOSC, s.osc[:0], false
	case 'P', 'X', '^', '_':
		s.state = stString
	default:
		s.dirty = true
		s.escDispatch(b)
	}
}

func (s *Screen) escDispatch(b byte) {
	switch b {
	case '7':
		s.saveCursor()
	case '8':
		s.restoreCursor()
	case 'D':
		s.index()
	case 'E':
		s.index()
		s.x = 0
	case 'M':
		s.reverseIndex()
	case 'H':
		s.tabs[s.x] = true
	case 'c':
		s.reset()
	case '=':
		s.modes.AppKeypad = true
	case '>':
		s.modes.AppKeypad = false
	}
}

func (s *Screen) collectInter(b byte) {
	if s.ninter < maxInter {
		s.inter[s.ninter] = b
		s.ninter++
	} else {
		s.interBad = true
	}
}

func (s *Screen) escInter(b byte) {
	switch {
	case b < 0x20:
		s.execute(b)
		return
	case b < 0x30:
		s.collectInter(b)
		return
	case b == 0x7f:
		return
	}
	s.state = stGround
	if s.interBad || s.ninter != 1 {
		return
	}
	s.dirty = true
	switch s.inter[0] {
	case '(', ')':
		cs := charsetASCII
		if b == '0' {
			cs = charsetGraphics
		}
		s.charsets[s.inter[0]-'('] = cs
	case '#':
		if b == '8' {
			s.alignmentTest()
		}
	}
}

// csiByte advances the CSI states.
func (s *Screen) csiByte(b byte) {
	switch {
	case b < 0x20:
		s.execute(b)
	case b == 0x7f:
	case b >= 0x40:
		if s.state != stCSIIgnore && !s.interBad {
			s.dispatchCSI(b)
		}
		s.state = stGround
	case s.state == stCSIIgnore:
	case b < 0x30:
		s.state = stCSIInter
		s.collectInter(b)
	case s.state == stCSIInter:
		s.state = stCSIIgnore
	case b <= '9':
		s.state = stCSIParam
		if s.nparams == 0 {
			s.newParam(false)
		}
		i := s.nparams - 1
		v := max(s.params[i], 0)*10 + int32(b-'0')
		s.params[i] = min(v, maxParamVal)
	case b == ';' || b == ':':
		if s.nparams == 0 {
			s.newParam(false)
		}
		s.state = stCSIParam
		s.newParam(b == ':')
	case s.state == stCSIEntry:
		s.priv = b
		s.state = stCSIParam
	default:
		s.state = stCSIIgnore
	}
}

// newParam starts a parameter; past the cap the last slot is reused.
func (s *Screen) newParam(sub bool) {
	if s.nparams == maxParams {
		s.params[maxParams-1] = -1
		return
	}
	s.params[s.nparams] = -1
	s.psub[s.nparams] = sub
	s.nparams++
}

// param returns parameter i, or def when it is missing.
func (s *Screen) param(i int, def int) int {
	if i >= s.nparams || s.params[i] < 0 {
		return def
	}
	return int(s.params[i])
}

// count returns parameter i for a repeat count: missing or zero means one.
func (s *Screen) count(i int) int {
	return max(s.param(i, 1), 1)
}
