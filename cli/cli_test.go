package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wow-look-at-my/go-tmux/client"
	"github.com/wow-look-at-my/go-tmux/internal/ttytest"
	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/go-tmux/server"
)

// harness runs the command line against a server in this process.
type harness struct {
	t      *testing.T
	socket string
	srv    *server.Server
	term   *ttytest.Screen
	// startErr makes starting the server fail.
	startErr error
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	t.Serial("the command line keeps its flags and its seams in package variables")
	dir, err := os.MkdirTemp("", "gmux")
	require.NoError(t, err)
	h := &harness{t: t, socket: filepath.Join(dir, "s.sock"), term: ttytest.Open(t, 100, 24)}

	oldStart, oldTerminal, oldNest, oldPS1 := start, terminal, os.Getenv(client.NestEnv), os.Getenv("PS1")
	os.Unsetenv(client.NestEnv)
	os.Setenv("PS1", "$ ")
	start = func(socket string) error {
		if h.startErr != nil {
			return h.startErr
		}
		if h.srv != nil {
			return nil
		}
		h.srv = server.New(server.Options{Socket: socket, NoStats: true, Mux: mux.Config{Shell: "/bin/sh"}})
		if err := h.srv.Listen(); err != nil {
			return err
		}
		go h.srv.Serve()
		return nil
	}
	terminal = func() client.Terminal { return client.Terminal{In: h.term.TTY, Out: h.term.TTY} }
	t.Cleanup(func() {
		start, terminal = oldStart, oldTerminal
		os.Setenv(client.NestEnv, oldNest)
		os.Setenv("PS1", oldPS1)
		if h.srv != nil {
			h.srv.Close()
		}
		os.RemoveAll(dir)
	})
	return h
}

// run executes one command line and returns what it printed.
func (h *harness) run(args ...string) (stdout, stderr string, code int) {
	var out, errOut bytes.Buffer
	code = Execute("test-version", append([]string{"-S", h.socket}, args...), &out, &errOut)
	return out.String(), errOut.String(), code
}

func (h *harness) ok(args ...string) string {
	h.t.Helper()
	stdout, stderr, code := h.run(args...)
	require.Zero(h.t, code, "gmux %s: %s", strings.Join(args, " "), stderr)
	return stdout
}

func (h *harness) fails(want string, args ...string) {
	h.t.Helper()
	_, stderr, code := h.run(args...)
	require.Equal(h.t, 1, code)
	require.Contains(h.t, stderr, want)
}

// background runs a command line that attaches, and returns where its output arrives when it ends.
func (h *harness) background(args ...string) chan string {
	done := make(chan string, 1)
	go func() {
		stdout, stderr, _ := h.run(args...)
		done <- stdout + stderr
	}()
	return done
}

func (h *harness) await(done chan string) string {
	h.t.Helper()
	select {
	case out := <-done:
		return out
	case <-time.After(ttytest.Wait):
		h.t.Fatal("the command never returned")
		return ""
	}
}

func TestCommandsThatNeedNoTerminal(t *testing.T) {
	h := newHarness(t)
	h.fails("no server is running", "session", "ls")
	h.fails("no server is running", "server", "kill")

	h.ok("session", "new", "-d", "-s", "work", "-c", t.TempDir())
	h.ok("session", "new", "-d")
	h.fails("already exists", "session", "new", "-d", "-s", "work")

	listing := h.ok("session", "list")
	lines := strings.Split(strings.TrimSpace(listing), "\n")
	require.Len(t, lines, 2)
	assert.Regexp(t, `^work: 1 windows \(created .+\) \[\d+x\d+\]$`, lines[0])
	assert.True(t, strings.HasPrefix(lines[1], "cli: 1 windows"), "a session with no name takes its directory's: %s", lines[1])

	h.ok("pane", "send", "-t", "work", "echo cli-$((3+4))", "Enter")
	require.Eventually(t, func() bool {
		return strings.Contains(h.ok("pane", "capture", "-p", "-t", "work"), "cli-7")
	}, ttytest.Wait, 20*time.Millisecond)
	h.ok("pane", "send", "-l", "-t", "work", "Enter")
	require.Eventually(t, func() bool {
		return strings.Contains(h.ok("pane", "capture", "-t", "work"), " Enter")
	}, ttytest.Wait, 20*time.Millisecond)

	h.ok("session", "rename", "-t", "work", "job")
	h.fails("no session named", "pane", "capture", "-t", "work")
	h.ok("session", "kill", "-t", "job")
	assert.NotContains(t, h.ok("session", "list"), "job")

	h.ok("server", "kill")
	select {
	case <-h.srv.Done():
	case <-time.After(ttytest.Wait):
		t.Fatal("the server outlived its kill")
	}
}

func TestAttachingCommands(t *testing.T) {
	h := newHarness(t)
	h.fails("no server is running", "session", "attach")

	done := h.background("session", "new", "-s", "live")
	h.term.Shows(" live ")
	h.term.Send("\x02d")
	assert.Equal(t, "[detached (from session live)]\n", h.await(done))
	assert.Contains(t, h.ok("session", "list"), "live: 1 windows")

	done = h.background("session", "attach", "-t", "live")
	h.term.Shows(" live ")
	// The command line is one at a time, so the listing is asked of the server directly while the attach runs.
	resp, err := server.Ask(h.socket, server.Request{Op: server.OpList})
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Sessions[0].Attached)
	h.term.Send("\x02d")
	assert.Contains(t, h.await(done), "detached")

	// With -A, a session that exists is joined and not made again.
	done = h.background("session", "new", "-A", "-s", "live")
	h.term.Shows(" live ")
	h.term.Send("exit\r")
	assert.Equal(t, "[exited]\n", h.await(done))
}

func TestNoCommandStartsASessionAndAttaches(t *testing.T) {
	h := newHarness(t)
	done := h.background()
	h.term.Shows(" cli ")
	h.srv.Close()
	assert.Equal(t, "[server exited]\n", h.await(done))
}

func TestAnAttachFromInsideASessionIsRefused(t *testing.T) {
	h := newHarness(t)
	os.Setenv(client.NestEnv, "/some/socket")
	h.fails("already inside a gmux session", "session", "attach")
	h.fails("already inside a gmux session", "session", "new")
	h.fails("already inside a gmux session")
	// A session nobody attaches to can still be started from inside one.
	h.ok("session", "new", "-d")
}

func TestTheServerCommandServesUntilKilled(t *testing.T) {
	h := newHarness(t)
	done := h.background("server", "run", "--socket", h.socket, "--no-stats")
	require.Eventually(t, func() bool {
		_, err := server.Ask(h.socket, server.Request{Op: server.OpList})
		return err == nil
	}, ttytest.Wait, 10*time.Millisecond)

	// A second server on the socket has nothing to do, and says so by ending cleanly.
	_, _, code := h.run("server", "run", "--no-stats")
	assert.Zero(t, code)

	h.ok("server", "kill")
	assert.Contains(t, h.await(done), "listening on "+h.socket)
}

func TestTheServerCommandReportsASocketItCannotTake(t *testing.T) {
	h := newHarness(t)
	h.fails("cannot create", "server", "run", "--socket", "/proc/nope/s.sock", "--no-stats")
}

func TestVersionAndUsage(t *testing.T) {
	h := newHarness(t)
	assert.Contains(t, h.ok("--version"), "test-version")
	assert.Equal(t, "gmux version test-version\n", h.ok("version"))
	h.fails("unknown command", "dance")
	h.fails("requires at least 1 arg", "pane", "send")
}

func TestABuildWithNoReleaseHasNoVersion(t *testing.T) {
	newHarness(t)
	var out, errOut bytes.Buffer
	require.Equal(t, 1, Execute("", []string{"version"}, &out, &errOut))
	assert.Empty(t, out.String())
	assert.Contains(t, errOut.String(), "not built for a release")
}

func TestTheGroupsHoldTheCommands(t *testing.T) {
	h := newHarness(t)
	help := h.ok("--help")
	for _, name := range []string{"session", "server", "pane", "version"} {
		assert.Regexp(t, `(?m)^  `+name+` `, help)
	}
	for group, names := range map[string][]string{
		"session": {"new", "attach", "rename", "kill", "list"},
		"server":  {"kill"},
		"pane":    {"send", "capture"},
	} {
		// A group with no command after it prints its help.
		help := h.ok(group)
		for _, name := range names {
			assert.Regexp(t, `(?m)^  `+name+` `, help, "gmux %s", group)
		}
		h.fails(`unknown command "dance" for "gmux `+group+`"`, group, "dance")
	}
	assert.NotContains(t, h.ok("server", "--help"), "  run ")

	// The names the commands had before the groups are gone.
	for _, old := range []string{"new-session", "attach-session", "rename-session", "kill-session", "list-sessions",
		"kill-server", "send-keys", "capture-pane", "new", "attach", "ls"} {
		h.fails("unknown command", old)
	}
}

func TestStartingTheServerFailsLoudly(t *testing.T) {
	h := newHarness(t)
	h.startErr = assert.AnError
	h.fails(assert.AnError.Error(), "session", "new", "-d")
	h.fails(assert.AnError.Error(), "session", "new")
}
