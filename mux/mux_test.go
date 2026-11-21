package mux

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wow-look-at-my/go-tmux/layout"
	"github.com/wow-look-at-my/tml/input"
)

const wait = 10 * time.Second

func newMux(t *testing.T) *Mux {
	t.Helper()
	m := New(Config{Shell: "/bin/sh", Scrollback: 500, Socket: "/tmp/test.sock"})
	t.Cleanup(m.Close)
	return m
}

func newSession(t *testing.T, m *Mux, opts SessionOptions) *Session {
	t.Helper()
	if opts.Env == nil {
		opts.Env = append(os.Environ(), "PS1=$ ", "ENV=")
	}
	s, err := m.NewSession(opts)
	require.NoError(t, err)
	return s
}

// shows waits until the active pane's screen holds text.
func shows(t *testing.T, m *Mux, s *Session, text string) {
	t.Helper()
	require.Eventually(t, func() bool {
		pane := m.ActivePane(s)
		return pane != nil && strings.Contains(pane.Capture(), text)
	}, wait, 5*time.Millisecond, "the pane never showed %q", text)
}

func typeLine(m *Mux, s *Session, line string) {
	m.ActivePane(s).Input([]byte(line + "\n"))
}

func TestASessionRunsTheShellOnATerminal(t *testing.T) {
	m := newMux(t)
	s := newSession(t, m, SessionOptions{})
	typeLine(m, s, "echo sum-$((6*7)) $TERM pane-$GMUX_PANE $GMUX")
	shows(t, m, s, "sum-42 xterm-256color pane-1 /tmp/test.sock")

	info, ok := m.Info(s)
	require.True(t, ok)
	assert.Equal(t, "mux", info.Name, "a session with no name takes the name of its directory, which is this package's")
	assert.Equal(t, DefaultCols, info.Cols)
	require.Len(t, info.Windows, 1)
	assert.True(t, info.Windows[0].Active)
	assert.Equal(t, 1, info.Windows[0].Panes)
	assert.False(t, m.Empty())
}

func TestSessionNamesAndTargets(t *testing.T) {
	m := newMux(t)
	_, err := m.Find("")
	require.ErrorContains(t, err, "no sessions")

	dir := filepath.Join(t.TempDir(), "notes")
	require.NoError(t, os.Mkdir(dir, 0o700))
	first := newSession(t, m, SessionOptions{Spawn: Spawn{Dir: dir}})
	second := newSession(t, m, SessionOptions{Spawn: Spawn{Dir: dir}})
	work := newSession(t, m, SessionOptions{Name: "work"})
	web := newSession(t, m, SessionOptions{Name: "web"})
	assert.Equal(t, "notes", first.Name)
	assert.Equal(t, "notes-2", second.Name, "a name in use gets a number after it")
	root := newSession(t, m, SessionOptions{Spawn: Spawn{Dir: "/"}})
	assert.Equal(t, fallbackName, root.Name, "the root directory has no name to give")
	m.KillSession(root)

	_, err = m.NewSession(SessionOptions{Name: "work"})
	require.ErrorContains(t, err, "already exists")

	found, err := m.Find("wor")
	require.NoError(t, err)
	assert.Same(t, work, found)
	_, err = m.Find("w")
	require.ErrorContains(t, err, "more than one")
	_, err = m.Find("nope")
	require.ErrorContains(t, err, "no session named")

	m.Touch(web)
	recent, err := m.Find("")
	require.NoError(t, err)
	assert.Same(t, web, recent)

	require.ErrorContains(t, m.RenameSession(web, "work"), "already exists")
	require.ErrorContains(t, m.RenameSession(web, ""), "empty")
	require.NoError(t, m.RenameSession(web, "site"))
	names := []string{}
	for _, info := range m.List() {
		names = append(names, info.Name)
	}
	assert.Equal(t, []string{"notes", "notes-2", "work", "site"}, names)
}

func TestWindowsOfASession(t *testing.T) {
	m := newMux(t)
	s := newSession(t, m, SessionOptions{})
	require.NoError(t, m.NewWindow(s, Spawn{Command: []string{"/bin/sh", "-c", "echo second; sleep 600"}}))
	require.NoError(t, m.NewWindow(s, Spawn{Command: []string{"/bin/sh", "-c", "echo third; sleep 600"}}))
	shows(t, m, s, "third")

	active := func() int {
		info, _ := m.Info(s)
		for _, w := range info.Windows {
			if w.Active {
				return w.Index
			}
		}
		return -1
	}
	assert.Equal(t, 2, active())
	m.SelectWindow(s, 0)
	assert.Equal(t, 0, active())
	m.SelectWindow(s, 9)
	assert.Equal(t, 0, active())
	m.LastWindow(s)
	assert.Equal(t, 2, active())
	m.StepWindow(s, 1)
	assert.Equal(t, 0, active())
	m.StepWindow(s, -1)
	assert.Equal(t, 2, active())

	m.RenameWindow(s, 1, "logs")
	m.MoveWindow(s, 1, 0)
	info, _ := m.Info(s)
	assert.Equal(t, "logs", info.Windows[0].Name)
	assert.Equal(t, 2, active(), "the window on screen stays on screen when another moves")
	m.MoveWindow(s, 0, 7)
	m.RenameWindow(s, 0, "")
	info, _ = m.Info(s)
	assert.Equal(t, "sh", info.Windows[0].Name)

	m.KillWindow(s, 2)
	m.KillWindow(s, 9)
	require.Eventually(t, func() bool {
		info, _ := m.Info(s)
		return len(info.Windows) == 2
	}, wait, 5*time.Millisecond)
	assert.Equal(t, 1, active(), "the window shown before the closed one comes back")
}

func TestPanesOfAWindow(t *testing.T) {
	m := newMux(t)
	s := newSession(t, m, SessionOptions{Cols: 81, Rows: 25})
	first := m.ActivePane(s)
	require.NoError(t, m.Split(s, layout.Columns, Spawn{}))
	require.NoError(t, m.Split(s, layout.Rows, Spawn{}))

	view := m.View(s)
	require.Len(t, view.Panes, 3)
	require.Len(t, view.Dividers, 2)
	assert.Equal(t, layout.Rect{W: 81, H: 25}, view.Area)
	third := m.ActivePane(s)
	assert.NotSame(t, first, third)

	m.FocusSide(s, layout.Left)
	assert.Same(t, first, m.ActivePane(s))
	m.FocusSide(s, layout.Left)
	assert.Same(t, first, m.ActivePane(s), "there is no pane further left")
	m.LastPane(s)
	assert.Same(t, third, m.ActivePane(s))
	m.StepPane(s, 1)
	assert.Same(t, first, m.ActivePane(s))
	m.SelectPane(s, third)
	assert.Same(t, third, m.ActivePane(s))

	before := first.Info().Cols
	m.SelectPane(s, first)
	m.ResizePane(s, layout.Right, 5)
	assert.Equal(t, before+5, first.Info().Cols)
	m.DragDivider(s, m.View(s).Dividers[0].Ref, 20, 0)
	assert.Equal(t, 20, first.Info().Cols)
	m.Equalize(s)
	assert.Equal(t, 40, first.Info().Cols)

	m.ToggleZoom(s)
	zoomed := m.View(s)
	require.Len(t, zoomed.Panes, 1)
	assert.True(t, zoomed.Zoomed)
	assert.Equal(t, 81, first.Info().Cols)
	m.FocusSide(s, layout.Right)
	assert.Same(t, first, m.ActivePane(s), "a zoomed pane has no neighbour")
	m.ToggleZoom(s)

	m.SwapPane(s, 1)
	assert.Same(t, first, m.ActivePane(s), "the focus stays with the program")
	assert.NotEqual(t, 0, m.View(s).Panes[1].Rect.X)

	m.KillPane(s)
	require.Eventually(t, func() bool { return len(m.View(s).Panes) == 2 }, wait, 5*time.Millisecond)
	assert.NotSame(t, first, m.ActivePane(s))
}

func TestSplitRefusesAPaneWithNoRoom(t *testing.T) {
	m := newMux(t)
	s := newSession(t, m, SessionOptions{Cols: 2, Rows: 2})
	require.ErrorContains(t, m.Split(s, layout.Columns, Spawn{}), "too small")
	require.ErrorContains(t, m.Split(s, layout.Rows, Spawn{}), "too small")
}

func TestASessionEndsWithItsLastProgram(t *testing.T) {
	m := newMux(t)
	var mu sync.Mutex
	var kinds []EventKind
	stop := m.Watch(func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		kinds = append(kinds, e.Kind)
	})
	s := newSession(t, m, SessionOptions{Spawn: Spawn{Command: []string{"/bin/sh", "-c", "echo bye"}}})
	require.Eventually(t, m.Empty, wait, 5*time.Millisecond)
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(kinds) > 0 && kinds[len(kinds)-1] == Ended
	}, wait, 5*time.Millisecond)
	_, ok := m.Info(s)
	assert.False(t, ok)
	assert.Nil(t, m.ActivePane(s))
	assert.Empty(t, m.View(s).Panes)
	require.Error(t, m.NewWindow(s, Spawn{}))
	require.Error(t, m.Split(s, layout.Rows, Spawn{}))

	stop()
	mu.Lock()
	seen := len(kinds)
	mu.Unlock()
	newSession(t, m, SessionOptions{})
	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, kinds, seen, "a stopped watcher hears nothing")
}

func TestKillSessionEndsItsPrograms(t *testing.T) {
	m := newMux(t)
	s := newSession(t, m, SessionOptions{})
	pane := m.ActivePane(s)
	m.KillSession(s)
	assert.True(t, m.Empty())
	select {
	case <-pane.pty.Done():
	case <-time.After(wait):
		t.Fatal("the program outlived its session")
	}
}

func TestOutputOutOfSightMarksItsWindow(t *testing.T) {
	m := newMux(t)
	s := newSession(t, m, SessionOptions{})
	quiet := m.ActivePane(s)
	require.NoError(t, m.NewWindow(s, Spawn{}))
	quiet.Input([]byte("printf 'hidden\\a\\n'\n"))
	require.Eventually(t, func() bool {
		info, _ := m.Info(s)
		return info.Windows[0].Activity && info.Windows[0].Bell
	}, wait, 5*time.Millisecond)
	m.SelectWindow(s, 0)
	info, _ := m.Info(s)
	assert.False(t, info.Windows[0].Activity)
	assert.False(t, info.Windows[0].Bell)
}

func TestClientsSizeTheSession(t *testing.T) {
	m := newMux(t)
	s := newSession(t, m, SessionOptions{})
	pane := m.ActivePane(s)
	m.Attach(s, 1, 100, 30)
	assert.Equal(t, 100, pane.Info().Cols)
	assert.Equal(t, 30, pane.Info().Rows)
	m.Attach(s, 2, 60, 40)
	assert.Equal(t, 60, pane.Info().Cols, "the smallest client decides")
	assert.Equal(t, 30, pane.Info().Rows)
	m.Detach(s, 2)
	assert.Equal(t, 100, pane.Info().Cols)
	m.Detach(s, 1)
	assert.Equal(t, 100, pane.Info().Cols, "a session nobody shows keeps its size")
	info, _ := m.Info(s)
	assert.Zero(t, info.Attached)

	typeLine(m, s, "stty size")
	shows(t, m, s, "30 100")
}

func TestANarrowerThenWiderSessionKeepsItsText(t *testing.T) {
	m := newMux(t)
	s := newSession(t, m, SessionOptions{Cols: 80, Rows: 10})
	pane := m.ActivePane(s)
	long := strings.Repeat("0123456789", 6)
	typeLine(m, s, "echo "+long)
	shows(t, m, s, "\n"+long)

	m.Attach(s, 1, 25, 10)
	require.Equal(t, 25, pane.Info().Cols)
	assert.Contains(t, pane.Capture(), "\n"+long[:25]+"\n"+long[25:50]+"\n"+long[50:], "the line is cut at the new width")

	m.Attach(s, 1, 80, 10)
	assert.Contains(t, pane.Capture(), "\n"+long+"\n", "and is whole again on a wider screen")
}

func TestRefreshFollowsTheRunningProgram(t *testing.T) {
	m := newMux(t)
	dir := t.TempDir()
	s := newSession(t, m, SessionOptions{Spawn: Spawn{Dir: dir}})
	pane := m.ActivePane(s)
	assert.Equal(t, dir, pane.Dir())
	typeLine(m, s, "sleep 600")
	require.Eventually(t, func() bool {
		m.Refresh()
		info, _ := m.Info(s)
		return info.Windows[0].Name == "sleep"
	}, wait, 20*time.Millisecond)
	assert.Equal(t, "sleep", pane.Info().Command)
	assert.Equal(t, dir, pane.Info().Dir)

	require.NoError(t, m.NewWindow(s, Spawn{}))
	assert.Equal(t, dir, m.ActivePane(s).Dir(), "a new pane starts where the active one is")
}

func TestAProgramSetsTheClipboardAndTheTitle(t *testing.T) {
	m := newMux(t)
	texts := make(chan string, 4)
	defer m.Watch(func(e Event) {
		if e.Kind == Clipboard {
			texts <- e.Text
		}
	})()
	s := newSession(t, m, SessionOptions{})
	typeLine(m, s, `printf '\033]52;c;aGVsbG8=\007\033]2;my title\007'`)
	select {
	case text := <-texts:
		assert.Equal(t, "hello", text)
	case <-time.After(wait):
		t.Fatal("the clipboard was never set")
	}
	require.Eventually(t, func() bool { return m.ActivePane(s).Info().Title == "my title" }, wait, 5*time.Millisecond)
}

func TestKeysPasteAndThePointerReachTheProgram(t *testing.T) {
	m := newMux(t)
	script := `printf '\033[?1000h\033[?1006h\033[?2004h'; stty -echo; echo reader-is-up; cat -v`
	s := newSession(t, m, SessionOptions{Spawn: Spawn{Command: []string{"/bin/sh", "-c", script}}})
	pane := m.ActivePane(s)
	// Input sent before the program turned the echo off is drawn by the terminal as well.
	shows(t, m, s, "reader-is-up")
	require.Eventually(t, func() bool { return pane.Modes().BracketedPaste }, wait, 5*time.Millisecond)

	pane.Key(input.ParseKey("up"))
	pane.Paste("pasted")
	assert.True(t, pane.Mouse(input.Mouse{Button: input.ButtonLeft, Action: input.Press}, 2, 3))
	assert.True(t, pane.Mouse(input.Mouse{Button: input.ButtonNone, Action: input.Motion}, 2, 3), "taken, and not sent")
	pane.Key(input.ParseKey("enter"))
	shows(t, m, s, "^[[A^[[200~pasted^[[201~^[[<0;3;4M")
	assert.NotContains(t, pane.Capture(), "<35;")
}

func TestAProgramThatDidNotAskGetsNoPointer(t *testing.T) {
	m := newMux(t)
	s := newSession(t, m, SessionOptions{})
	assert.False(t, m.ActivePane(s).Mouse(input.Mouse{Button: input.ButtonLeft, Action: input.Press}, 0, 0))
}
