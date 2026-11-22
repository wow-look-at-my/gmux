package ui

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/go-tmux/stats"
	"github.com/wow-look-at-my/tml"
	"github.com/wow-look-at-my/tml/cell"
	"github.com/wow-look-at-my/tml/input"
)

const wait = 10 * time.Second

// fakeStats is a host with a fixed load.
type fakeStats struct{}

func (fakeStats) Bar(detail bool) stats.Bar {
	bar := stats.Bar{CPU: "12%", CPUHistory: []string{"5", "40", "12"}, Mem: "3.2G/16G", Net: "↓1.2K ↑300B", Load: "0.52"}
	if detail {
		bar.Load = "0.52 0.40 0.31"
	}
	return bar
}

// rig is a client with no terminal: the test hands it messages the way a program would, and reads frames back.
type rig struct {
	t             *testing.T
	mux           *mux.Mux
	session       *mux.Session
	client        *Client
	program       *tml.Program
	width, height int
	now           time.Time
}

func newRig(t *testing.T, width, height int) *rig { return newRigWith(t, width, height, 0) }

// newRigWith is a rig whose panes keep a gutter of some columns, as a server's panes do.
func newRigWith(t *testing.T, width, height, gutter int) *rig {
	t.Helper()
	r := &rig{t: t, width: width, height: height, now: time.Date(2026, 3, 4, 15, 4, 5, 0, time.UTC)}
	r.mux = mux.New(mux.Config{Shell: "/bin/sh", Scrollback: 500, Gutter: gutter})
	t.Cleanup(r.mux.Close)
	var err error
	r.session, err = r.mux.NewSession(mux.SessionOptions{
		Name: "main", Env: append(os.Environ(), "PS1=$ ", "ENV="), Spawn: mux.Spawn{Dir: t.TempDir()},
	})
	require.NoError(t, err)
	r.client, err = New(Options{Mux: r.mux, Session: r.session, ID: 1, Stats: fakeStats{}, Now: func() time.Time { return r.now }})
	require.NoError(t, err)
	r.program, err = tml.NewProgram(r.client.View(), r.client, tml.ProgramOptions{
		Input: strings.NewReader(""), Output: io.Discard, Width: width, Height: height,
	})
	require.NoError(t, err)
	r.send(tml.Resize{Width: width, Height: height})
	r.frame()
	return r
}

// send routes one message the way a program does: to the client first, then through the focus ring, whose events
// go back to the client.
func (r *rig) send(msg tml.Msg) {
	r.t.Helper()
	switch msg.(type) {
	case tml.Key, tml.Mouse, tml.Paste:
		if r.client.Update(r.program, msg) {
			return
		}
		for _, event := range r.client.View().UI().Update(msg) {
			r.client.Update(r.program, event)
		}
	default:
		r.client.Update(r.program, msg)
	}
}

// frame draws one frame, which is also what publishes where every element is.
func (r *rig) frame() *cell.Buffer {
	r.t.Helper()
	frame, err := r.client.View().Draw(r.client.Props(), r.width, r.height)
	require.NoError(r.t, err)
	return frame
}

func (r *rig) lines() []string { return r.frame().Lines() }

// bars is the text of the rows under the window.
func (r *rig) bars() (windows, info string) {
	lines := r.lines()
	return lines[r.height-2], lines[r.height-1]
}

func (r *rig) keys(names ...string) {
	r.t.Helper()
	for _, name := range names {
		r.send(input.ParseKey(name))
		r.frame()
	}
}

func (r *rig) mouse(action input.Action, button input.Button, x, y int) {
	r.t.Helper()
	r.send(tml.Mouse{X: x, Y: y, Button: button, Action: action})
	r.frame()
}

func (r *rig) click(button input.Button, x, y int) {
	r.t.Helper()
	r.mouse(input.Press, button, x, y)
	r.mouse(input.Release, button, x, y)
}

// at is the middle of the element with an id.
func (r *rig) at(id string) (x, y int) {
	r.t.Helper()
	r.frame()
	target, ok := r.client.View().UI().Target(id)
	require.True(r.t, ok, "no element %q on screen", id)
	return target.Rect.X + target.Rect.W/2, target.Rect.Y + target.Rect.H/2
}

func (r *rig) clickOn(id string) {
	r.t.Helper()
	x, y := r.at(id)
	r.click(input.ButtonLeft, x, y)
}

// item is a cell of an item of the open menu, counted from the first.
func (r *rig) item(n int) (x, y int) {
	r.t.Helper()
	r.frame()
	target, ok := r.client.View().UI().Target(menuID)
	require.True(r.t, ok, "no menu on screen")
	return target.Rect.X + 2, target.Rect.Y + 1 + n
}

func (r *rig) clickItem(n int) {
	r.t.Helper()
	x, y := r.item(n)
	r.click(input.ButtonLeft, x, y)
}

// shows waits until the window area holds text.
func (r *rig) shows(text string) {
	r.t.Helper()
	require.Eventually(r.t, func() bool {
		return strings.Contains(strings.Join(r.lines()[:r.height-2], "\n"), text)
	}, wait, 5*time.Millisecond, "the window never showed %q; it shows:\n%s", text, strings.Join(r.lines(), "\n"))
}

// reader starts a program that prints back what it is sent, after an optional
// setup command, and waits until it reads.
func (r *rig) reader(setup string) {
	r.t.Helper()
	r.typeLine(setup + `stty -echo; echo reader-is-"up"; cat -v`)
	r.shows("reader-is-up")
}

func (r *rig) typeLine(line string) {
	r.mux.ActivePane(r.session).Input([]byte(line + "\n"))
}

// windows is the names of the session's windows, the active one in brackets.
func (r *rig) windows() string {
	info, _ := r.mux.Info(r.session)
	var names []string
	for _, w := range info.Windows {
		if w.Active {
			names = append(names, "["+w.Name+"]")
		} else {
			names = append(names, w.Name)
		}
	}
	return strings.Join(names, " ")
}

func (r *rig) eventually(cond func() bool, msg string) {
	r.t.Helper()
	require.Eventually(r.t, cond, wait, 5*time.Millisecond, msg)
}
