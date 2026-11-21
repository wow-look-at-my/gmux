// Package ttytest gives a test a terminal to hand to a client, and reads back what was drawn on it.
package ttytest

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wow-look-at-my/go-tmux/pty"
	"github.com/wow-look-at-my/go-tmux/vt"
)

// Wait is how long a test waits for the screen to show something.
const Wait = 15 * time.Second

// Screen is a terminal as a test sees it: the far end of the terminal a client hands over, drawn by the emulator.
type Screen struct {
	t      testing.TB
	master *os.File
	// TTY is the end a client uses as its terminal.
	TTY *os.File

	mu sync.Mutex
	vt *vt.Screen
}

// Open opens a terminal of the given size and starts reading what is drawn on it.
func Open(t testing.TB, cols, rows int) *Screen {
	t.Helper()
	master, slave, err := pty.Open(cols, rows)
	require.NoError(t, err)
	s := &Screen{t: t, master: master, TTY: slave, vt: vt.New(cols, rows, vt.Options{})}
	t.Cleanup(func() {
		master.Close()
		slave.Close()
	})
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				s.Feed(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	return s
}

// Feed draws bytes on the screen, as if the program on the terminal had written them.
func (s *Screen) Feed(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vt.Write(b)
}

// Text is what the screen shows.
func (s *Screen) Text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vt.String()
}

// Modes is the terminal modes the program on the terminal has set.
func (s *Screen) Modes() vt.Modes {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vt.Modes()
}

// CursorVisible reports whether the cursor shows.
func (s *Screen) CursorVisible() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vt.Cursor().Visible
}

// Shows waits until the screen holds text.
func (s *Screen) Shows(text string) {
	s.t.Helper()
	require.Eventually(s.t, func() bool { return strings.Contains(s.Text(), text) }, Wait, 10*time.Millisecond,
		"the terminal never showed %q", text)
}

// Gone waits until the screen no longer holds text.
func (s *Screen) Gone(text string) {
	s.t.Helper()
	require.Eventually(s.t, func() bool { return !strings.Contains(s.Text(), text) }, Wait, 10*time.Millisecond,
		"the terminal kept showing %q", text)
}

// Send types input on the terminal.
func (s *Screen) Send(input string) {
	s.t.Helper()
	_, err := s.master.WriteString(input)
	require.NoError(s.t, err)
}

// Find is the cell where text starts on the screen.
func (s *Screen) Find(text string) (x, y int) {
	s.t.Helper()
	for row, line := range strings.Split(s.Text(), "\n") {
		if at := strings.Index(line, text); at >= 0 {
			return len([]rune(line[:at])), row
		}
	}
	require.Fail(s.t, "not on screen", "%q", text)
	return 0, 0
}

// Click presses and releases the left button on a cell, the way a terminal reports it.
func (s *Screen) Click(x, y int) {
	s.t.Helper()
	at := strconv.Itoa(x+1) + ";" + strconv.Itoa(y+1)
	s.Send("\x1b[<0;" + at + "M\x1b[<0;" + at + "m")
}

// Resize changes the terminal's size and tells this process, the way a terminal tells its foreground program.
func (s *Screen) Resize(cols, rows int) {
	s.t.Helper()
	s.mu.Lock()
	s.vt.Resize(cols, rows)
	s.mu.Unlock()
	require.NoError(s.t, pty.SetSize(s.master, cols, rows))
	require.NoError(s.t, syscall.Kill(os.Getpid(), syscall.SIGWINCH))
}
