// osc.go holds operating system commands: title, working directory and clipboard.
package vt

import (
	"bytes"
	"encoding/base64"
	"net/url"
	"strconv"
)

// oscByte collects one byte of an OSC payload; BEL ends the string.
func (s *Screen) oscByte(b byte) {
	switch {
	case b == 0x07:
		s.dispatchOSC()
		s.state = stGround
	case b < 0x20:
	case len(s.osc) >= maxOSC:
		s.oscBad = true
	default:
		s.osc = append(s.osc, b)
	}
}

// dispatchOSC runs a complete OSC. The emulator never answers an OSC.
func (s *Screen) dispatchOSC() {
	if s.oscBad {
		return
	}
	code, data, _ := bytes.Cut(s.osc, []byte{';'})
	n, err := strconv.Atoi(string(code))
	if err != nil {
		return
	}
	switch n {
	case 0, 2:
		s.setTitle(string(data))
	case 7:
		s.setCwd(string(data))
	case 52:
		s.clipboard(data)
	}
}

func (s *Screen) setTitle(t string) {
	s.title = t
	s.dirty = true
	if s.onTitle != nil {
		s.onTitle(t)
	}
}

// setCwd reads a file URL, or a bare path, and keeps the decoded path.
func (s *Screen) setCwd(v string) {
	u, err := url.Parse(v)
	if err != nil {
		return
	}
	if u.Scheme != "" && u.Scheme != "file" {
		return
	}
	s.cwd = u.Path
}

// clipboard decodes a set request of the form selection;base64.
func (s *Screen) clipboard(data []byte) {
	_, payload, ok := bytes.Cut(data, []byte{';'})
	if !ok || s.onClip == nil || string(payload) == "?" {
		return
	}
	dec, err := base64.StdEncoding.DecodeString(string(payload))
	if err != nil {
		dec, err = base64.RawStdEncoding.DecodeString(string(payload))
		if err != nil {
			return
		}
	}
	s.onClip(dec)
}
