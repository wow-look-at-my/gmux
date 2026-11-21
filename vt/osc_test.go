package vt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOSCTitle(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"icon and title BEL", "\x1b]0;one\x07", "one"},
		{"title ST", "\x1b]2;two\x1b\\", "two"},
		{"icon only", "\x1b]1;icon\x07", ""},
		{"utf8 title", "\x1b]2;héllo 世界\x07", "héllo 世界"},
		{"empty title", "\x1b]2;x\x07\x1b]2;\x07", ""},
		{"no code", "\x1b];x\x07", ""},
		{"bad code", "\x1b]a;x\x07", ""},
		{"ESC other ends OSC", "\x1b]2;cut\x1b[1mx", "cut"},
		{"control ignored", "\x1b]2;a\rb\x07", "ab"},
		{"colour query", "\x1b]10;?\x07\x1b]4;1;?\x07", ""},
		{"hyperlink", "\x1b]8;;http://example.com\x07link\x1b]8;;\x07", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(10, 1, Options{})
			var titles []string
			var replies int
			s.OnTitle(func(v string) { titles = append(titles, v) })
			s.OnReply(func([]byte) { replies++ })
			s.Write([]byte(tt.in))
			assert.Equal(t, tt.want, s.Title())
			if tt.want != "" {
				assert.Equal(t, tt.want, titles[len(titles)-1])
			}
			assert.Zero(t, replies)
		})
	}
	assert.Equal(t, "link", feed(10, 1, "\x1b]8;;http://e\x07link\x1b]8;;\x07").String())
	s := New(10, 1, Options{})
	s.OnTitle(nil)
	s.Write([]byte("\x1b]2;t\x07"))
	assert.Equal(t, "t", s.Title())
}

func TestOSCCwd(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"\x1b]7;file://host/home/user/my%20dir\x07", "/home/user/my dir"},
		{"\x1b]7;file:///tmp\x1b\\", "/tmp"},
		{"\x1b]7;/bare/path\x07", "/bare/path"},
		{"\x1b]7;http://host/x\x07", ""},
		{"\x1b]7;file://host/%zz\x07", ""},
		{"", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, feed(10, 1, tt.in).Cwd(), "%q", tt.in)
	}
}

func TestOSCClipboard(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"set", "\x1b]52;c;aGVsbG8=\x07", []string{"hello"}},
		{"set unpadded", "\x1b]52;c;aGVsbG8\x07", []string{"hello"}},
		{"set ST", "\x1b]52;;d29ybGQ=\x1b\\", []string{"world"}},
		{"query", "\x1b]52;c;?\x07", nil},
		{"invalid base64", "\x1b]52;c;!!!\x07", nil},
		{"no selection field", "\x1b]52;aGVsbG8=\x07", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(10, 1, Options{})
			var got []string
			s.OnClipboard(func(b []byte) { got = append(got, string(b)) })
			s.Write([]byte(tt.in))
			assert.Equal(t, tt.want, got)
		})
	}

	// A payload of about a megabyte fits.
	big := make([]byte, 700_000)
	for i := range big {
		big[i] = 'A'
	}
	s := New(10, 1, Options{})
	var n int
	s.OnClipboard(func(b []byte) { n = len(b) })
	s.Write([]byte("\x1b]52;c;"))
	s.Write(big)
	s.Write([]byte("\x07"))
	assert.Equal(t, 525_000, n)

	s.OnClipboard(nil)
	s.Write([]byte("\x1b]52;c;aGVsbG8=\x07"))
}

func TestBell(t *testing.T) {
	s := New(10, 1, Options{})
	bells := 0
	s.OnBell(func() { bells++ })
	s.Write([]byte("a\x07\x1b]2;t\x07\x1b[\x07m"))
	assert.Equal(t, 2, bells)
	s.OnBell(nil)
	s.Write([]byte("\x07"))
}
