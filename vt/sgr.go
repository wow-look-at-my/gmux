// sgr.go holds Select Graphic Rendition: attributes and colours.
package vt

// sgr applies the parameters of an SGR sequence to the current style.
func (s *Screen) sgr() {
	if s.nparams == 0 {
		s.style = Style{}
		return
	}
	st := &s.style
	for i := 0; i < s.nparams; {
		p := s.params[i]
		j := i + 1
		for j < s.nparams && s.psub[j] {
			j++
		}
		subs := s.params[i+1 : j]
		switch {
		case p <= 0:
			*st = Style{}
		case p == 1:
			st.Attrs |= Bold
		case p == 2:
			st.Attrs |= Faint
		case p == 3:
			st.Attrs |= Italic
		case p == 4:
			if len(subs) > 0 && subs[0] == 0 {
				st.Attrs &^= Underline
			} else {
				st.Attrs |= Underline
			}
		case p == 5 || p == 6:
			st.Attrs |= Blink
		case p == 7:
			st.Attrs |= Reverse
		case p == 8:
			st.Attrs |= Invisible
		case p == 9:
			st.Attrs |= Strikethrough
		case p == 21:
			st.Attrs |= Underline
		case p == 22:
			st.Attrs &^= Bold | Faint
		case p == 23:
			st.Attrs &^= Italic
		case p == 24:
			st.Attrs &^= Underline
		case p == 25:
			st.Attrs &^= Blink
		case p == 27:
			st.Attrs &^= Reverse
		case p == 28:
			st.Attrs &^= Invisible
		case p == 29:
			st.Attrs &^= Strikethrough
		case p >= 30 && p <= 37:
			st.Fg = Indexed(uint8(p - 30))
		case p == 39:
			st.Fg = 0
		case p >= 40 && p <= 47:
			st.Bg = Indexed(uint8(p - 40))
		case p == 49:
			st.Bg = 0
		case p >= 90 && p <= 97:
			st.Fg = Indexed(uint8(p - 90 + 8))
		case p >= 100 && p <= 107:
			st.Bg = Indexed(uint8(p - 100 + 8))
		case p == 38 || p == 48 || p == 58:
			var c Color
			var ok bool
			if len(subs) > 0 {
				c, ok = colonColor(subs)
			} else {
				c, ok, j = s.semicolonColor(j)
			}
			if ok && p == 38 {
				st.Fg = c
			} else if ok && p == 48 {
				st.Bg = c
			}
		}
		i = j
	}
}

// colonColor reads an extended colour from sub-parameters: the colon form,
// with or without the colourspace field.
func colonColor(subs []int32) (Color, bool) {
	switch subs[0] {
	case 5:
		if len(subs) >= 2 {
			return Indexed(clampByte(subs[1])), true
		}
	case 2:
		rgb := subs[1:]
		if len(rgb) >= 4 {
			rgb = rgb[1:]
		}
		if len(rgb) >= 3 {
			return RGB(clampByte(rgb[0]), clampByte(rgb[1]), clampByte(rgb[2])), true
		}
	}
	return 0, false
}

// semicolonColor reads an extended colour from the parameters after i and
// returns the index of the next unread parameter.
func (s *Screen) semicolonColor(i int) (Color, bool, int) {
	switch s.param(i, -1) {
	case 5:
		return Indexed(clampByte(int32(s.param(i+1, 0)))), true, min(i+2, s.nparams)
	case 2:
		c := RGB(clampByte(int32(s.param(i+1, 0))), clampByte(int32(s.param(i+2, 0))), clampByte(int32(s.param(i+3, 0))))
		return c, true, min(i+4, s.nparams)
	}
	return 0, false, i
}

func clampByte(v int32) uint8 {
	return uint8(min(max(v, 0), 255))
}
