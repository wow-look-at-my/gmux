package vt

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func asciiInput() []byte {
	var b strings.Builder
	for i := range 200 {
		b.WriteString("the quick brown fox jumps over the lazy dog ")
		b.WriteString(strconv.Itoa(i))
		b.WriteString(" lorem ipsum dolor sit amet\r\n")
	}
	return []byte(b.String())
}

func sgrInput() []byte {
	var b strings.Builder
	for i := range 200 {
		b.WriteString("\x1b[01;34mdirectory\x1b[0m  \x1b[01;32mexecutable.sh\x1b[0m  plain.txt  ")
		b.WriteString("\x1b[38;5;208mwarning\x1b[39m: \x1b[1msrc/file.go:")
		b.WriteString(strconv.Itoa(i))
		b.WriteString("\x1b[22m \x1b[38;2;200;100;50munused\x1b[m variable\r\n")
	}
	return []byte(b.String())
}

func cursorInput() []byte {
	var b strings.Builder
	b.WriteString("\x1b[?2026h\x1b[H")
	for y := 1; y <= 24; y++ {
		b.WriteString("\x1b[")
		b.WriteString(strconv.Itoa(y))
		b.WriteString(";1H\x1b[44m ")
		b.WriteString(strconv.Itoa(y))
		b.WriteString(" \x1b[0m\x1b[K\x1b[")
		b.WriteString(strconv.Itoa(y))
		b.WriteString(";10H│ some cell text here \x1b[7mselected\x1b[27m and more text to the edge")
	}
	b.WriteString("\x1b[24;80H\x1b[?2026l")
	return []byte(b.String())
}

func benchWrite(b *testing.B, s *Screen, in []byte) {
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		s.Write(in)
	}
}

func BenchmarkWriteASCII(b *testing.B) {
	benchWrite(b, New(80, 24, Options{Scrollback: -1}), asciiInput())
}

func BenchmarkWriteSGR(b *testing.B) {
	benchWrite(b, New(80, 24, Options{Scrollback: -1}), sgrInput())
}

func BenchmarkWriteCursor(b *testing.B) {
	benchWrite(b, New(80, 24, Options{}), cursorInput())
}

func BenchmarkScrollback(b *testing.B) {
	s := New(80, 24, Options{Scrollback: 1000})
	in := asciiInput()
	for range 10 {
		s.Write(in)
	}
	benchWrite(b, s, in)
}

func TestWriteDoesNotAllocate(t *testing.T) {
	inputs := map[string][]byte{
		"ascii":  asciiInput(),
		"sgr":    sgrInput(),
		"cursor": cursorInput(),
		"edit":   []byte("\x1b[5;5H\x1b[2@\x1b[3P\x1b[2L\x1b[M\x1b[S\x1b[T\x1b[J\x1b[1K\x1bM\x1bD\x1b[3;10r\x1b[r"),
	}
	for name, in := range inputs {
		s := New(80, 24, Options{Scrollback: 100})
		for range 5 {
			s.Write(in)
		}
		allocs := testing.AllocsPerRun(20, func() { s.Write(in) })
		assert.Zero(t, allocs, name)
	}
}
