package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/tml/input"
)

const wait = 10 * time.Second

// startServer runs a server on a socket of its own. The directory is short, because a socket path has a limit.
func startServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "gmux")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "nested", "s.sock")
	srv := New(Options{Socket: path, NoStats: true, Mux: mux.Config{Shell: "/bin/sh"}})
	require.NoError(t, srv.Listen())
	go srv.Serve()
	t.Cleanup(srv.Close)
	return srv, path
}

func env() []string { return append(os.Environ(), "PS1=$ ", "ENV=") }

func TestRequestsThatNeedNoTerminal(t *testing.T) {
	_, path := startServer(t)

	resp, err := Ask(path, Request{Op: OpNew, Name: "one", Env: env(), Cols: 90, Rows: 20})
	require.NoError(t, err)
	assert.Equal(t, "one", resp.Session)
	_, err = Ask(path, Request{Op: OpNew, Name: "one"})
	require.ErrorContains(t, err, "already exists")
	_, err = Ask(path, Request{Op: OpNew, Env: env()})
	require.NoError(t, err)

	resp, err = Ask(path, Request{Op: OpList})
	require.NoError(t, err)
	require.Len(t, resp.Sessions, 2)
	assert.Equal(t, SessionLine{Name: "one", Windows: 1, Created: resp.Sessions[0].Created, Cols: 90, Rows: 20}, resp.Sessions[0])
	assert.Equal(t, "0", resp.Sessions[1].Name)

	_, err = Ask(path, Request{Op: OpSendKeys, Target: "one", Keys: []string{"echo sum-$((20+3))", "Enter"}})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		resp, err := Ask(path, Request{Op: OpCapture, Target: "one"})
		return err == nil && strings.Contains(resp.Output, "sum-23")
	}, wait, 10*time.Millisecond)

	_, err = Ask(path, Request{Op: OpSendKeys, Target: "one", Keys: []string{"Enter"}, Literal: true})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		resp, _ := Ask(path, Request{Op: OpCapture, Target: "one"})
		return strings.Contains(resp.Output, "$ Enter")
	}, wait, 10*time.Millisecond, "a literal key name is typed as text")

	_, err = Ask(path, Request{Op: OpRename, Target: "one", Name: "renamed"})
	require.NoError(t, err)
	_, err = Ask(path, Request{Op: OpRename, Target: "renamed", Name: "0"})
	require.ErrorContains(t, err, "already exists")
	_, err = Ask(path, Request{Op: OpCapture, Target: "missing"})
	require.ErrorContains(t, err, "no session named")
	_, err = Ask(path, Request{Op: "dance", Target: "0"})
	require.ErrorContains(t, err, "unknown request")

	_, err = Ask(path, Request{Op: OpKillSession, Target: "renamed"})
	require.NoError(t, err)
	resp, err = Ask(path, Request{Op: OpList})
	require.NoError(t, err)
	assert.Len(t, resp.Sessions, 1)
}

func TestKeyNames(t *testing.T) {
	keys := map[string]string{
		"Enter": "enter", "enter": "enter", "Escape": "esc", "Tab": "tab", "BTab": "shift+tab", "Space": "space",
		"BSpace": "backspace", "Up": "up", "PageUp": "pgup", "NPage": "pgdown", "F5": "f5", "DC": "delete",
		"C-c": "ctrl+c", "M-x": "alt+x", "C-M-a": "ctrl+alt+a", "ctrl+c": "ctrl+c", "alt+enter": "alt+enter",
		"S-Up": "shift+up", "C-Left": "ctrl+left",
	}
	for name, want := range keys {
		key, ok := parseKey(name)
		require.True(t, ok, name)
		assert.Equal(t, input.ParseKey(want).Keystroke(), key.Keystroke(), name)
	}
	for _, text := range []string{"a", "ls -la", "hello", "C-", "x-y", "echo", "é", ""} {
		_, ok := parseKey(text)
		assert.False(t, ok, "%q is text", text)
	}
}

func TestASecondServerOnASocketIsRefused(t *testing.T) {
	_, path := startServer(t)
	second := New(Options{Socket: path, NoStats: true})
	require.ErrorIs(t, second.Listen(), ErrRunning)
}

func TestASocketNobodyAnswersIsReplaced(t *testing.T) {
	first, path := startServer(t)
	// A server that dies leaves its socket file behind.
	first.ln.SetUnlinkOnClose(false)
	first.Close()
	<-first.Done()
	_, err := os.Stat(path)
	require.NoError(t, err)

	second := New(Options{Socket: path, NoStats: true})
	require.NoError(t, second.Listen())
	second.Close()
}

func TestListenReportsADirectoryItCannotMake(t *testing.T) {
	srv := New(Options{Socket: "/proc/nope/s.sock", NoStats: true})
	require.ErrorContains(t, srv.Listen(), "cannot create")
}

func TestKillServerEndsEverything(t *testing.T) {
	srv, path := startServer(t)
	_, err := Ask(path, Request{Op: OpNew, Env: env()})
	require.NoError(t, err)
	_, err = Ask(path, Request{Op: OpKillServer})
	require.NoError(t, err)
	select {
	case <-srv.Done():
	case <-time.After(wait):
		t.Fatal("the server outlived its kill")
	}
	_, err = Ask(path, Request{Op: OpList})
	require.ErrorContains(t, err, "no server is running")
	assert.True(t, srv.Mux().Empty())
}

func TestTheServerEndsWithItsLastSession(t *testing.T) {
	srv, path := startServer(t)
	_, err := Ask(path, Request{Op: OpNew, Command: []string{"/bin/sh", "-c", "exit 0"}})
	require.NoError(t, err)
	select {
	case <-srv.Done():
	case <-time.After(wait):
		t.Fatal("the server outlived its last session")
	}
}

func TestAnIdleServerCloses(t *testing.T) {
	srv, _ := startServer(t)
	assert.False(t, srv.idle(), "a server just started is given time")
	srv.mu.Lock()
	srv.lastUsed = time.Now().Add(-2 * idleLimit)
	srv.mu.Unlock()
	assert.True(t, srv.idle())
	select {
	case <-srv.Done():
	case <-time.After(wait):
		t.Fatal("the idle server never closed")
	}
}

func TestAnAttachWithoutATerminalIsRefused(t *testing.T) {
	_, path := startServer(t)
	_, err := Ask(path, Request{Op: OpAttach, Create: true})
	require.ErrorContains(t, err, "hands over the terminal")
}

func TestSocketPath(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/7")
	t.Setenv(SocketEnv, "")
	assert.Equal(t, "/explicit.sock", SocketPath("/explicit.sock", "name"))
	assert.Equal(t, "/run/user/7/gmux/default.sock", SocketPath("", ""))
	assert.Equal(t, "/run/user/7/gmux/work.sock", SocketPath("", "work"))
	t.Setenv(SocketEnv, "/from/env.sock")
	assert.Equal(t, "/from/env.sock", SocketPath("", ""))
	assert.Equal(t, "/run/user/7/gmux/work.sock", SocketPath("", "work"), "a name asks for the default directory")
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv(SocketEnv, "")
	assert.Contains(t, SocketPath("", ""), "gmux-")
}

func TestLookupReadsAnEnvironmentList(t *testing.T) {
	get := lookup([]string{"TERM=xterm", "COLORTERM=truecolor", "EMPTY="})
	assert.Equal(t, "xterm", get("TERM"))
	assert.Equal(t, "truecolor", get("COLORTERM"))
	assert.Equal(t, "", get("EMPTY"))
	assert.Equal(t, "", get("MISSING"))
}
