package client

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/go-tmux/pty"
	"github.com/wow-look-at-my/go-tmux/server"
	"github.com/wow-look-at-my/go-tmux/vt"
)

const wait = 15 * time.Second

// TestMain lets the test binary stand in for the program a client starts as its server.
func TestMain(m *testing.M) {
	if len(os.Args) == 4 && os.Args[1] == "server" && os.Args[2] == "--socket" {
		srv := server.New(server.Options{Socket: os.Args[3], NoStats: true, Mux: mux.Config{Shell: "/bin/sh"}})
		if err := srv.Listen(); err != nil {
			os.Exit(1)
		}
		_ = srv.Serve()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func socketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "gmux")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func env() []string { return append(os.Environ(), "PS1=$ ", "ENV=", "TERM=xterm-256color") }

// screen is a terminal as a test sees it: the far end of the terminal a client hands over, drawn by the emulator.
type screen struct {
	t      *testing.T
	master *os.File
	slave  *os.File

	mu sync.Mutex
	vt *vt.Screen
}

func openScreen(t *testing.T, cols, rows int) *screen {
	t.Helper()
	master, slave, err := pty.Open(cols, rows)
	require.NoError(t, err)
	s := &screen{t: t, master: master, slave: slave, vt: vt.New(cols, rows, vt.Options{})}
	t.Cleanup(func() {
		master.Close()
		slave.Close()
	})
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				s.mu.Lock()
				s.vt.Write(buf[:n])
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	return s
}

func (s *screen) tty() Terminal { return Terminal{In: s.slave, Out: s.slave} }

func (s *screen) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vt.String()
}

func (s *screen) shows(text string) {
	s.t.Helper()
	require.Eventually(s.t, func() bool { return strings.Contains(s.text(), text) }, wait, 10*time.Millisecond,
		"the terminal never showed %q; it shows:\n%s", text, s.text())
}

func (s *screen) send(input string) {
	s.t.Helper()
	_, err := s.master.WriteString(input)
	require.NoError(s.t, err)
}

// find is the cell where text starts on the screen.
func (s *screen) find(text string) (x, y int) {
	s.t.Helper()
	for row, line := range strings.Split(s.text(), "\n") {
		if at := strings.Index(line, text); at >= 0 {
			return len([]rune(line[:at])), row
		}
	}
	require.Fail(s.t, "not on screen", "%q", text)
	return 0, 0
}

// click presses and releases the left button on a cell, the way a terminal reports it.
func (s *screen) click(x, y int) {
	at := strconv.Itoa(x+1) + ";" + strconv.Itoa(y+1)
	s.send("\x1b[<0;" + at + "M\x1b[<0;" + at + "m")
}

func (s *screen) resize(cols, rows int) {
	s.t.Helper()
	s.mu.Lock()
	s.vt.Resize(cols, rows)
	s.mu.Unlock()
	require.NoError(s.t, pty.SetSize(s.master, cols, rows))
	require.NoError(s.t, syscall.Kill(os.Getpid(), syscall.SIGWINCH))
}

type result struct {
	resp server.Response
	err  error
}

func attach(socket string, req server.Request, s *screen) chan result {
	done := make(chan result, 1)
	go func() {
		resp, err := Attach(socket, req, s.tty())
		done <- result{resp, err}
	}()
	return done
}

func await(t *testing.T, done chan result) result {
	t.Helper()
	select {
	case r := <-done:
		return r
	case <-time.After(wait):
		t.Fatal("the attach never ended")
		return result{}
	}
}

func TestAttachDrawsTheSessionOnTheTerminalAndDetachGivesItBack(t *testing.T) {
	socket := filepath.Join(socketDir(t), "s.sock")
	srv := server.New(server.Options{Socket: socket, NoStats: true, Mux: mux.Config{Shell: "/bin/sh"}})
	require.NoError(t, srv.Listen())
	go srv.Serve()
	defer srv.Close()

	term := openScreen(t, 100, 24)
	done := attach(socket, server.Request{Create: true, Name: "e2e", Env: env()}, term)
	term.shows(" e2e ")
	term.shows("0:sh")
	term.shows("100×22")

	term.send("echo attached-$((2+3))\r")
	term.shows("attached-5")

	// The pointer works on the bar: the button that adds a window, then the first tab.
	x, y := term.find(" + ")
	term.click(x+1, y)
	term.shows("1:sh")
	x, y = term.find("0:sh")
	term.click(x, y)
	term.shows("attached-5")

	term.resize(70, 20)
	term.shows("70×18")

	term.send("\x02d")
	r := await(t, done)
	require.NoError(t, r.err)
	assert.Equal(t, server.ExitDetached, r.resp.Exit)
	assert.Equal(t, "e2e", r.resp.Session)
	require.Eventually(t, func() bool {
		term.mu.Lock()
		defer term.mu.Unlock()
		return !term.vt.Modes().AltScreen && term.vt.Modes().Mouse == vt.MouseOff
	}, wait, 10*time.Millisecond, "the terminal was not given back as it was")

	// The session kept running, and a second terminal picks it up where it was.
	second := openScreen(t, 90, 20)
	done = attach(socket, server.Request{Target: "e2e"}, second)
	second.shows("attached-5")
	second.send("exit\r")
	second.shows("0:sh ")
	second.send("exit\r")
	r = await(t, done)
	require.NoError(t, r.err)
	assert.Equal(t, server.ExitExited, r.resp.Exit)
	select {
	case <-srv.Done():
	case <-time.After(wait):
		t.Fatal("the server outlived its last session")
	}
}

func TestAttachEndsWhenTheServerCloses(t *testing.T) {
	socket := filepath.Join(socketDir(t), "s.sock")
	srv := server.New(server.Options{Socket: socket, NoStats: true, Mux: mux.Config{Shell: "/bin/sh"}})
	require.NoError(t, srv.Listen())
	go srv.Serve()

	term := openScreen(t, 80, 20)
	done := attach(socket, server.Request{Create: true, Env: env()}, term)
	term.shows("0:sh")
	srv.Close()
	r := await(t, done)
	require.NoError(t, r.err)
	assert.Equal(t, server.ExitClosed, r.resp.Exit)
}

func TestAttachReportsWhatTheServerRefuses(t *testing.T) {
	socket := filepath.Join(socketDir(t), "s.sock")
	srv := server.New(server.Options{Socket: socket, NoStats: true, Mux: mux.Config{Shell: "/bin/sh"}})
	require.NoError(t, srv.Listen())
	go srv.Serve()
	defer srv.Close()

	term := openScreen(t, 80, 20)
	_, err := Attach(socket, server.Request{Target: "nothing"}, term.tty())
	require.ErrorContains(t, err, "no sessions")
}

func TestAttachNeedsATerminalAndAServer(t *testing.T) {
	notTerminal, err := os.Open(os.DevNull)
	require.NoError(t, err)
	defer notTerminal.Close()
	_, err = Attach("/nonexistent/s.sock", server.Request{}, Terminal{In: notTerminal, Out: notTerminal})
	require.ErrorContains(t, err, "needs a terminal")

	term := openScreen(t, 80, 20)
	_, err = Attach(filepath.Join(socketDir(t), "none.sock"), server.Request{}, term.tty())
	require.ErrorContains(t, err, "no server is running")
}

func TestAttachPutsTheTerminalBackWhenTheServerIsLost(t *testing.T) {
	socket := filepath.Join(socketDir(t), "s.sock")
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	require.NoError(t, err)
	defer ln.Close()
	go func() {
		// A server that takes the request and dies.
		conn, err := ln.AcceptUnix()
		if err != nil {
			return
		}
		var req server.Request
		_ = server.NewReader(conn).Read(&req)
		conn.Close()
	}()

	term := openScreen(t, 80, 20)
	term.mu.Lock()
	term.vt.Write([]byte("\x1b[?1049h\x1b[?25l"))
	term.mu.Unlock()
	_, err = Attach(socket, server.Request{}, term.tty())
	require.ErrorContains(t, err, "lost the server")
	require.Eventually(t, func() bool {
		term.mu.Lock()
		defer term.mu.Unlock()
		return !term.vt.Modes().AltScreen && term.vt.Cursor().Visible
	}, wait, 10*time.Millisecond)
}

func TestStartRunsAServerThatOutlivesTheCall(t *testing.T) {
	socket := filepath.Join(socketDir(t), "started.sock")
	exe, err := os.Executable()
	require.NoError(t, err)
	require.NoError(t, Start(socket, exe))
	defer server.Ask(socket, server.Request{Op: server.OpKillServer})

	_, err = server.Ask(socket, server.Request{Op: server.OpNew, Env: env()})
	require.NoError(t, err)
	resp, err := server.Ask(socket, server.Request{Op: server.OpList})
	require.NoError(t, err)
	assert.Len(t, resp.Sessions, 1)

	// A server that already answers is left alone.
	require.NoError(t, Start(socket, "/nonexistent/program"))
	log, err := os.ReadFile(socket + ".log")
	require.NoError(t, err)
	assert.Empty(t, string(log))
}

func TestStartReportsAServerThatDoesNotCome(t *testing.T) {
	socket := filepath.Join(socketDir(t), "never.sock")
	err := Start(socket, "/bin/false")
	require.ErrorContains(t, err, "did not start")
	require.ErrorContains(t, err, socket+".log")
}
