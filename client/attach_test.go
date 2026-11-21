package client

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wow-look-at-my/go-tmux/internal/ttytest"
	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/go-tmux/server"
	"github.com/wow-look-at-my/go-tmux/vt"
)

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

// serve runs a server in this process.
func serve(t *testing.T) (*server.Server, string) {
	t.Helper()
	socket := filepath.Join(socketDir(t), "s.sock")
	srv := server.New(server.Options{Socket: socket, NoStats: true, Mux: mux.Config{Shell: "/bin/sh"}})
	require.NoError(t, srv.Listen())
	go srv.Serve()
	t.Cleanup(srv.Close)
	return srv, socket
}

type result struct {
	resp server.Response
	err  error
}

func attach(socket string, req server.Request, s *ttytest.Screen) chan result {
	done := make(chan result, 1)
	go func() {
		resp, err := Attach(socket, req, Terminal{In: s.TTY, Out: s.TTY})
		done <- result{resp, err}
	}()
	return done
}

func await(t *testing.T, done chan result) result {
	t.Helper()
	select {
	case r := <-done:
		return r
	case <-time.After(ttytest.Wait):
		t.Fatal("the attach never ended")
		return result{}
	}
}

func TestAttachDrawsTheSessionOnTheTerminalAndDetachGivesItBack(t *testing.T) {
	srv, socket := serve(t)
	term := ttytest.Open(t, 100, 24)
	done := attach(socket, server.Request{Create: true, Name: "e2e", Env: env()}, term)
	term.Shows(" e2e ")
	term.Shows("1:sh")
	term.Shows("99×22")

	term.Send("echo attached-$((2+3))\r")
	term.Shows("attached-5")

	// The pointer works on the bar: the button that adds a window, then the first tab.
	x, y := term.Find(" + ")
	term.Click(x+1, y)
	term.Shows("2:sh")
	term.Gone("attached-5")
	x, y = term.Find("1:sh")
	term.Click(x, y)
	term.Shows("attached-5")

	term.Resize(70, 20)
	term.Shows("69×18")

	term.Send("\x02d")
	r := await(t, done)
	require.NoError(t, r.err)
	assert.Equal(t, server.ExitDetached, r.resp.Exit)
	assert.Equal(t, "e2e", r.resp.Session)
	require.Eventually(t, func() bool {
		modes := term.Modes()
		return !modes.AltScreen && modes.Mouse == vt.MouseOff && term.CursorVisible()
	}, ttytest.Wait, 10*time.Millisecond, "the terminal was not given back as it was")

	// The session kept running, and a second terminal picks it up where it was.
	second := ttytest.Open(t, 90, 20)
	done = attach(socket, server.Request{Target: "e2e"}, second)
	second.Shows("attached-5")
	second.Send("exit\r")
	second.Gone("2:sh")
	second.Send("exit\r")
	r = await(t, done)
	require.NoError(t, r.err)
	assert.Equal(t, server.ExitExited, r.resp.Exit)
	select {
	case <-srv.Done():
	case <-time.After(ttytest.Wait):
		t.Fatal("the server outlived its last session")
	}
}

func TestAttachEndsWhenTheServerCloses(t *testing.T) {
	srv, socket := serve(t)
	term := ttytest.Open(t, 80, 20)
	done := attach(socket, server.Request{Create: true, Env: env()}, term)
	term.Shows("1:sh")
	srv.Close()
	r := await(t, done)
	require.NoError(t, r.err)
	assert.Equal(t, server.ExitClosed, r.resp.Exit)
}

func TestAttachReportsWhatTheServerRefuses(t *testing.T) {
	_, socket := serve(t)
	term := ttytest.Open(t, 80, 20)
	_, err := Attach(socket, server.Request{Target: "nothing"}, Terminal{In: term.TTY, Out: term.TTY})
	require.ErrorContains(t, err, "no sessions")
}

func TestAttachNeedsATerminalAndAServer(t *testing.T) {
	notTerminal, err := os.Open(os.DevNull)
	require.NoError(t, err)
	defer notTerminal.Close()
	_, err = Attach("/nonexistent/s.sock", server.Request{}, Terminal{In: notTerminal, Out: notTerminal})
	require.ErrorContains(t, err, "needs a terminal")

	term := ttytest.Open(t, 80, 20)
	_, err = Attach(filepath.Join(socketDir(t), "none.sock"), server.Request{}, Terminal{In: term.TTY, Out: term.TTY})
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

	term := ttytest.Open(t, 80, 20)
	term.Feed([]byte("\x1b[?1049h\x1b[?25l"))
	_, err = Attach(socket, server.Request{}, Terminal{In: term.TTY, Out: term.TTY})
	require.ErrorContains(t, err, "lost the server")
	require.Eventually(t, func() bool { return !term.Modes().AltScreen && term.CursorVisible() },
		ttytest.Wait, 10*time.Millisecond)
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
}

func TestStartReportsAServerThatDoesNotCome(t *testing.T) {
	socket := filepath.Join(socketDir(t), "never.sock")
	err := Start(socket, "/bin/false")
	require.ErrorContains(t, err, "did not start")
	require.ErrorContains(t, err, socket+".log")
}
