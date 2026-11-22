package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wow-look-at-my/go-tmux/layout"
	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/tml"
	"github.com/wow-look-at-my/tml/cell"
	"github.com/wow-look-at-my/tml/input"
)

func TestTheScreenIsTheWindowAndTwoBars(t *testing.T) {
	r := newRig(t, 100, 20)
	r.typeLine("echo hello-$((40+2))")
	r.shows("hello-42")

	windows, info := r.bars()
	assert.True(t, strings.HasPrefix(windows, " ☰  main  1:sh ×  + "), windows)
	assert.True(t, strings.HasSuffix(strings.TrimRight(windows, " "), "  ─"), windows)
	assert.Equal(t, windows, r.lines()[0], "the window row is the first row of the screen")
	assert.Equal(t, info, r.lines()[19], "the information row is the last")
	assert.Contains(t, info, "100×18")
	assert.Contains(t, info, "CPU")
	assert.Contains(t, info, "12%")
	assert.Contains(t, info, "MEM 3.2G/16G")
	assert.Contains(t, info, "NET ↓1.2K ↑300B")
	assert.Contains(t, info, "LOAD 0.52")
	assert.True(t, strings.HasSuffix(strings.TrimRight(info, " "), "15:04"), info)

	pane := r.mux.ActivePane(r.session).Info()
	assert.Equal(t, 100, pane.Cols)
	assert.Equal(t, 18, pane.Rows, "the bars take their rows from the window")
}

func TestTypingGoesToThePane(t *testing.T) {
	r := newRig(t, 80, 12)
	r.keys("e", "c", "h", "o", "space", "t", "y", "p", "e", "d", "enter")
	r.shows("typed")
	r.send(tml.Paste{Text: "echo pasted-$((1+1))\n"})
	r.shows("pasted-2")
	// A key the focus ring would take, and a key that would end a program left alone, both go to the pane.
	r.keys("tab", "ctrl+c")
	assert.Equal(t, Detached, r.client.Outcome())
}

func TestThePrefixRunsCommands(t *testing.T) {
	r := newRig(t, 100, 20)
	r.keys("ctrl+b")
	_, info := r.bars()
	assert.Contains(t, info, "PREFIX")
	r.keys("c")
	_, info = r.bars()
	assert.NotContains(t, info, "PREFIX")
	assert.Equal(t, "sh [sh]", r.windows())

	r.keys("ctrl+b", "1")
	assert.Equal(t, "[sh] sh", r.windows(), "the digits count the windows from one")
	r.keys("ctrl+b", "2")
	assert.Equal(t, "sh [sh]", r.windows())
	r.keys("ctrl+b", "0")
	assert.Equal(t, "sh [sh]", r.windows(), "zero is the tenth window, and there is none")
	r.keys("ctrl+b", "1")
	r.keys("ctrl+b", "n")
	assert.Equal(t, "sh [sh]", r.windows())
	// A key that repeats needs no second prefix.
	r.keys("n")
	assert.Equal(t, "[sh] sh", r.windows())
	r.send(disarmMsg{})
	r.keys("ctrl+b", "p", "ctrl+b", "l")
	assert.Equal(t, "[sh] sh", r.windows())

	r.keys("ctrl+b", "%", "ctrl+b", "\"")
	view := r.mux.View(r.session)
	require.Len(t, view.Panes, 3)
	third := r.mux.ActivePane(r.session)
	r.keys("ctrl+b", "left")
	first := r.mux.ActivePane(r.session)
	assert.NotSame(t, third, first)
	r.send(disarmMsg{})
	r.keys("ctrl+b", ";")
	assert.Same(t, third, r.mux.ActivePane(r.session))
	r.keys("ctrl+b", "o")
	r.send(disarmMsg{})
	assert.Same(t, first, r.mux.ActivePane(r.session))

	cols := first.Info().Cols
	r.keys("ctrl+b", "ctrl+right")
	assert.Equal(t, cols+1, first.Info().Cols)
	r.keys("alt+right")
	assert.Equal(t, cols+6, first.Info().Cols)
	r.send(disarmMsg{})
	r.keys("ctrl+b", "space")
	assert.Equal(t, cols, first.Info().Cols)

	r.keys("ctrl+b", "z")
	assert.Len(t, r.mux.View(r.session).Panes, 1)
	windows, _ := r.bars()
	assert.Contains(t, windows, "1:sh Z")
	r.keys("ctrl+b", "z", "ctrl+b", "}")
	r.send(disarmMsg{})
	assert.NotEqual(t, 0, r.mux.View(r.session).Panes[1].Rect.X+r.mux.View(r.session).Panes[1].Rect.Y)

	r.keys("ctrl+b", "x")
	r.eventually(func() bool { return len(r.mux.View(r.session).Panes) == 2 }, "the pane never closed")
	r.keys("ctrl+b", "&")
	r.eventually(func() bool { return r.windows() == "[sh]" }, "the window never closed")

	// A key with no command drops the prefix and is not typed.
	r.keys("ctrl+b", "Q")
	_, info = r.bars()
	assert.NotContains(t, info, "PREFIX")
}

func TestThePrefixTwiceSendsItToTheProgram(t *testing.T) {
	r := newRig(t, 80, 12)
	r.reader("")
	r.keys("ctrl+b", "ctrl+b", "enter")
	r.shows("^B")
}

func TestDetachLeavesTheSessionRunning(t *testing.T) {
	r := newRig(t, 80, 12)
	r.keys("ctrl+b", "d")
	assert.Equal(t, Detached, r.client.Outcome())
	info, ok := r.mux.Info(r.session)
	require.True(t, ok)
	assert.Zero(t, info.Attached)
}

func TestTheClientLeavesWhenItsLastSessionEnds(t *testing.T) {
	r := newRig(t, 80, 12)
	other, err := r.mux.NewSession(mux.SessionOptions{Name: "other"})
	require.NoError(t, err)

	r.send(endedMsg{session: other})
	assert.Same(t, r.session, r.client.Session(), "another session ending changes nothing")

	r.mux.KillSession(r.session)
	r.send(endedMsg{session: r.session})
	assert.Same(t, other, r.client.Session(), "the client moves to a session that still runs")
	windows, _ := r.bars()
	assert.Contains(t, windows, " other ")

	r.mux.KillSession(other)
	r.send(endedMsg{session: other})
	assert.Equal(t, SessionEnded, r.client.Outcome())
}

func TestTheServerClosingEndsTheClient(t *testing.T) {
	r := newRig(t, 80, 12)
	r.client.Close(r.program, ServerClosed)
	r.send(closeMsg{why: ServerClosed})
	assert.Equal(t, ServerClosed, r.client.Outcome())
	// The first reason stands.
	r.send(closeMsg{why: SessionEnded})
	assert.Equal(t, ServerClosed, r.client.Outcome())
}

func TestScrollingBackThroughHistory(t *testing.T) {
	r := newRig(t, 60, 12)
	r.typeLine("i=0; while [ $i -lt 60 ]; do echo line-$i; i=$((i+1)); done")
	r.shows("line-59")
	pane := r.mux.ActivePane(r.session)

	r.keys("ctrl+b", "[")
	_, info := r.bars()
	assert.Contains(t, info, "SCROLL")
	assert.Equal(t, 5, pane.Info().Offset)
	assert.Contains(t, r.window()[0], "[5/")

	r.keys("up", "k")
	assert.Equal(t, 7, pane.Info().Offset)
	r.keys("down", "j")
	assert.Equal(t, 5, pane.Info().Offset)
	r.keys("pgup")
	assert.Equal(t, 14, pane.Info().Offset)
	r.keys("pgdown", "ctrl+u", "ctrl+d")
	assert.Equal(t, 5, pane.Info().Offset)
	r.keys("g")
	assert.Equal(t, pane.Info().History, pane.Info().Offset)
	r.keys("G")
	assert.Zero(t, pane.Info().Offset)

	// The wheel scrolls too, and a key that is not a scroll key goes back to the live screen and is typed.
	r.mouse(input.Wheel, input.WheelUp, 5, 5)
	assert.Equal(t, scrollStep, pane.Info().Offset)
	r.mouse(input.Wheel, input.WheelDown, 5, 5)
	assert.Zero(t, pane.Info().Offset)
	r.mouse(input.Wheel, input.WheelUp, 5, 5)
	r.keys("x")
	assert.Zero(t, pane.Info().Offset)
}

func TestThePageKeysOnTheLiveScreenMoveThroughTheHistory(t *testing.T) {
	r := newRig(t, 60, 12)
	r.typeLine("i=0; while [ $i -lt 60 ]; do echo line-$i; i=$((i+1)); done")
	r.shows("line-59")
	r.reader("")
	pane := r.mux.ActivePane(r.session)

	// Page down has nowhere to go, and the program must not get it: a shell prints a tilde for it.
	r.keys("pgdown")
	assert.Zero(t, pane.Info().Offset)
	r.keys("pgup")
	assert.Equal(t, 9, pane.Info().Offset, "page up starts to scroll back")
	r.keys("pgdown", "pgdown")
	assert.Zero(t, pane.Info().Offset)
	r.keys("x", "y", "z", "enter")
	r.shows("xyz")
	assert.NotContains(t, pane.Capture(), "~", "no page key reached the program")
}

func TestThePageKeysGoToAFullScreenProgram(t *testing.T) {
	r := newRig(t, 60, 12)
	r.reader(`printf '\033[?1049h'; `)
	r.eventually(func() bool { return r.mux.ActivePane(r.session).Modes().AltScreen }, "never full screen")
	r.keys("pgup", "pgdown", "enter")
	r.shows("^[[5~^[[6~")
}

func TestTheWheelInAFullScreenProgramSendsArrows(t *testing.T) {
	r := newRig(t, 60, 12)
	r.reader(`printf '\033[?1049h'; `)
	r.eventually(func() bool { return r.mux.ActivePane(r.session).Modes().AltScreen }, "never full screen")
	r.mouse(input.Wheel, input.WheelUp, 5, 5)
	r.mouse(input.Wheel, input.WheelDown, 5, 5)
	r.mouse(input.Wheel, input.WheelLeft, 5, 5)
	r.keys("enter")
	r.shows("^[[A^[[A^[[A^[[B^[[B^[[B")
}

func TestAProgramThatAsksGetsThePointer(t *testing.T) {
	r := newRig(t, 60, 12)
	r.reader(`printf '\033[?1002h\033[?1006h'; `)
	r.eventually(func() bool { return r.mux.ActivePane(r.session).Modes().MouseSGR }, "never asked")
	// The program counts its rows from the top of its pane, which is the row under the window row.
	r.mouse(input.Press, input.ButtonLeft, 3, topRows+4)
	r.mouse(input.Motion, input.ButtonLeft, 5, topRows+4)
	// A drag that leaves the pane still reports, at the pane's edge.
	r.mouse(input.Motion, input.ButtonLeft, 5, 11)
	r.mouse(input.Release, input.ButtonLeft, 5, topRows+4)
	r.keys("enter")
	r.shows("^[[<0;4;5M^[[<32;6;5M^[[<32;6;10M^[[<0;6;5m")
}

func TestSelectingTextCopiesIt(t *testing.T) {
	r := newRig(t, 60, 12)
	r.typeLine("clear; echo alpha beta gamma")
	row := -1
	r.eventually(func() bool {
		for y, line := range r.lines() {
			if strings.HasPrefix(line, "alpha beta gamma") {
				row = y
			}
		}
		return row >= 0
	}, "the line never showed")

	r.mouse(input.Press, input.ButtonLeft, 6, row)
	r.mouse(input.Motion, input.ButtonLeft, 9, row)
	assert.True(t, r.frame().At(7, row).Style.Has(cell.Reverse), "the selection is drawn")
	r.mouse(input.Release, input.ButtonLeft, 9, row)
	assert.Equal(t, "beta", r.client.clip)
	_, info := r.bars()
	assert.Contains(t, info, "copied 4 chars")

	// The note leaves the bar after its time.
	r.now = r.now.Add(noteTime + time.Second)
	_, info = r.bars()
	assert.NotContains(t, info, "copied")

	// A second press on a cell takes the word, a third the line.
	r.now = r.now.Add(time.Hour)
	r.click(input.ButtonLeft, 1, row)
	r.click(input.ButtonLeft, 1, row)
	assert.Equal(t, "alpha", r.client.clip)
	r.click(input.ButtonLeft, 1, row)
	assert.Equal(t, "alpha beta gamma", r.client.clip)

	// The middle button pastes what was copied.
	r.reader("")
	r.click(input.ButtonMiddle, 3, 3)
	r.keys("enter")
	r.shows("\nalpha beta gamma")
}

func TestDraggingTheLineBetweenPanesResizesThem(t *testing.T) {
	r := newRig(t, 81, 20)
	require.NoError(t, r.mux.Split(r.session, layout.Columns, mux.Spawn{}))
	view := r.mux.View(r.session)
	require.Len(t, view.Dividers, 1)
	x := view.Dividers[0].Rect.X
	assert.Equal(t, 40, x)
	assert.Equal(t, "│", r.frame().At(x, 3).Text)

	r.mouse(input.Press, input.ButtonLeft, x, 3)
	r.mouse(input.Motion, input.ButtonLeft, 25, 6)
	r.mouse(input.Release, input.ButtonLeft, 25, 6)
	assert.Equal(t, 25, r.mux.View(r.session).Dividers[0].Rect.X)
	// The button is up, so the pointer moving on does nothing more.
	r.mouse(input.Motion, input.ButtonNone, 50, 6)
	assert.Equal(t, 25, r.mux.View(r.session).Dividers[0].Rect.X)

	// A press on a pane gives it the focus.
	second := r.mux.ActivePane(r.session)
	r.click(input.ButtonLeft, 3, 3)
	assert.NotSame(t, second, r.mux.ActivePane(r.session))
}

func TestLinesJoinWherePanesMeet(t *testing.T) {
	r := newRig(t, 41, 13)
	require.NoError(t, r.mux.Split(r.session, layout.Columns, mux.Spawn{}))
	require.NoError(t, r.mux.Split(r.session, layout.Rows, mux.Spawn{}))
	frame := r.frame()
	assert.Equal(t, "│", frame.At(20, 0).Text)
	assert.Equal(t, "├", frame.At(20, 5).Text)
	assert.Equal(t, "─", frame.At(30, 5).Text)
	assert.Equal(t, activeStyle.Fg, frame.At(30, 5).Style.Fg, "the lines around the active pane take the accent")
	assert.Equal(t, borderStyle.Fg, frame.At(20, 0).Style.Fg)
}

func TestAClientLargerThanTheSessionShowsTheRestAsAField(t *testing.T) {
	r := newRig(t, 60, 12)
	r.mux.Attach(r.session, 99, 30, 5)
	frame := r.frame()
	assert.Equal(t, "·", frame.At(45, 2).Text)
	assert.Equal(t, "·", frame.At(5, 8).Text)
	assert.Equal(t, " ", frame.At(29, 4).Text)
}
