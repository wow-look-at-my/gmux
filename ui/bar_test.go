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
	"github.com/wow-look-at-my/tml/input"
)

// withWindows adds named windows to the rig's session, then shows the first.
func (r *rig) withWindows(names ...string) {
	r.t.Helper()
	for i, name := range names {
		require.NoError(r.t, r.mux.NewWindow(r.session, mux.Spawn{}))
		r.mux.RenameWindow(r.session, i+1, name)
	}
	r.mux.SelectWindow(r.session, 0)
	r.frame()
}

func TestClickingATabShowsItsWindow(t *testing.T) {
	r := newRig(t, 100, 12)
	r.withWindows("logs", "edit")
	assert.Equal(t, "[sh] logs edit", r.windows())
	r.clickOn("tab-2")
	assert.Equal(t, "sh logs [edit]", r.windows())
	r.clickOn("tab-1")
	assert.Equal(t, "sh [logs] edit", r.windows())
}

func TestTheButtonsOfTheWindowRow(t *testing.T) {
	r := newRig(t, 100, 20)
	r.clickOn("new-window")
	assert.Equal(t, "sh [sh]", r.windows())
	r.clickOn("split-right")
	r.clickOn("split-down")
	require.Len(t, r.mux.View(r.session).Panes, 3)
	r.clickOn("zoom")
	assert.True(t, r.mux.View(r.session).Zoomed)
	r.clickOn("zoom")
	r.clickOn("kill-pane")
	r.eventually(func() bool { return len(r.mux.View(r.session).Panes) == 2 }, "the pane never closed")
	r.clickOn("help")
	assert.Equal(t, "help", r.client.overlay())
}

func TestTheWheelOverTheTabsTurnsThroughTheWindows(t *testing.T) {
	r := newRig(t, 100, 12)
	r.withWindows("logs", "edit")
	x, y := r.at("tab-0")
	r.mouse(input.Wheel, input.WheelDown, x, y)
	assert.Equal(t, "sh [logs] edit", r.windows())
	r.mouse(input.Wheel, input.WheelUp, x, y)
	r.mouse(input.Wheel, input.WheelUp, x, y)
	assert.Equal(t, "sh logs [edit]", r.windows())
	// The wheel over the information row belongs to nothing.
	x, y = r.at("clock")
	r.mouse(input.Wheel, input.WheelDown, x, y)
	assert.Equal(t, "sh logs [edit]", r.windows())
}

func TestTheMiddleButtonClosesATab(t *testing.T) {
	r := newRig(t, 100, 12)
	r.withWindows("logs", "edit")
	x, y := r.at("tab-1")
	r.click(input.ButtonMiddle, x, y)
	r.eventually(func() bool { return r.windows() == "[sh] edit" }, "the window never closed")
}

func TestDraggingATabMovesItsWindow(t *testing.T) {
	r := newRig(t, 100, 12)
	r.withWindows("logs", "edit")
	fromX, y := r.at("tab-0")
	toX, _ := r.at("tab-2")
	r.mouse(input.Press, input.ButtonLeft, fromX, y)
	r.mouse(input.Motion, input.ButtonLeft, fromX+3, y)
	_, info := r.bars()
	assert.Contains(t, info, "MOVE")
	r.mouse(input.Motion, input.ButtonLeft, toX, y)
	r.mouse(input.Release, input.ButtonLeft, toX, y)
	assert.Equal(t, "logs edit [sh]", r.windows())

	// Let go past the end of the tabs, and the window goes to the nearest end.
	fromX, _ = r.at("tab-2")
	r.mouse(input.Press, input.ButtonLeft, fromX, y)
	r.mouse(input.Motion, input.ButtonLeft, 1, y)
	r.mouse(input.Release, input.ButtonLeft, 1, y)
	assert.Equal(t, "[sh] logs edit", r.windows())
}

func TestASecondClickOnATabRenamesItsWindow(t *testing.T) {
	r := newRig(t, 100, 12)
	r.withWindows("logs")
	x, y := r.at("tab-1")
	r.click(input.ButtonLeft, x, y)
	r.mouse(input.Press, input.ButtonLeft, x, y)
	assert.Equal(t, "prompt", r.client.overlay())
	assert.Contains(t, strings.Join(r.lines(), "\n"), "rename window")

	// The field edits itself, and enter accepts what it holds.
	r.keys("backspace", "backspace", "backspace", "backspace", "d", "b", "enter")
	assert.Equal(t, "", r.client.overlay())
	assert.Equal(t, "sh [db]", r.windows())

	// Escape leaves the name alone.
	r.keys("ctrl+b", ",")
	r.keys("x", "esc")
	assert.Equal(t, "sh [db]", r.windows())
	r.keys("ctrl+b", "$")
	r.send(tml.Paste{Text: "2"})
	r.frame()
	r.keys("enter")
	windows, _ := r.bars()
	assert.Contains(t, windows, " main2 ")
}

func TestARightClickOnATabOpensItsMenu(t *testing.T) {
	r := newRig(t, 100, 14)
	r.withWindows("logs", "edit")
	x, y := r.at("tab-1")
	r.mouse(input.Press, input.ButtonRight, x, y)
	require.Equal(t, "menu", r.client.overlay())
	screen := strings.Join(r.lines(), "\n")
	assert.Contains(t, screen, "window 2", "the second tab's window, by the number on its tab")
	assert.Contains(t, screen, "Move right")

	r.clickItem(2)
	assert.Equal(t, "", r.client.overlay())
	assert.Equal(t, "[sh] edit logs", r.windows())

	// The keys move through a menu, and enter runs the item they are on.
	x, y = r.at("tab-2")
	r.mouse(input.Press, input.ButtonRight, x, y)
	r.keys("down", "down", "up", "enter")
	assert.Equal(t, "prompt", r.client.overlay(), "the first item renames")
	r.keys("esc")

	// A press outside a menu closes it and does nothing else.
	r.mouse(input.Press, input.ButtonRight, x, y)
	r.mouse(input.Press, input.ButtonLeft, 2, 2)
	assert.Equal(t, "", r.client.overlay())
	r.mouse(input.Release, input.ButtonLeft, 2, 2)

	// Any other key closes it too.
	r.mouse(input.Press, input.ButtonRight, x, y)
	r.keys("q")
	assert.Equal(t, "", r.client.overlay())
	r.mouse(input.Press, input.ButtonRight, x, y)
	r.keys("enter")
	assert.Equal(t, "", r.client.overlay(), "enter on no item closes the menu")
}

func TestARightClickInAPaneOpensThePaneMenu(t *testing.T) {
	r := newRig(t, 100, 20)
	r.mouse(input.Press, input.ButtonRight, 10, 4)
	require.Equal(t, "menu", r.client.overlay())
	screen := strings.Join(r.lines(), "\n")
	assert.Contains(t, screen, "Split right")
	assert.NotContains(t, screen, "Paste", "there is nothing to paste yet")
	x, y := r.item(0)
	r.mouse(input.Motion, input.ButtonNone, x, y)
	assert.Equal(t, 0, r.client.menu.hover, "the pointer marks the item it is on")
	r.mouse(input.Motion, input.ButtonNone, x, y-1)
	assert.Equal(t, -1, r.client.menu.hover, "the border is no item")
	r.click(input.ButtonLeft, x, y-1)
	require.Equal(t, "menu", r.client.overlay(), "a click on the border does nothing")
	r.click(input.ButtonLeft, x, y)
	assert.Len(t, r.mux.View(r.session).Panes, 2)

	r.client.clip = "echo from-the-menu\n"
	r.mouse(input.Press, input.ButtonRight, 10, 4)
	assert.Contains(t, strings.Join(r.lines(), "\n"), "Paste")
	r.clickItem(5)
	r.shows("from-the-menu")

	// A menu opened at the edge of the screen moves so that all of it shows.
	r.mouse(input.Press, input.ButtonRight, 99, 17)
	target, ok := r.client.View().UI().Target("menu")
	require.True(t, ok)
	assert.LessOrEqual(t, target.Rect.X+target.Rect.W, 100)
	assert.LessOrEqual(t, target.Rect.Y+target.Rect.H, 20)
}

// menuRect opens a menu at a cell and reports where it was drawn.
func (r *rig) menuRect(title string, x, y int, items ...menuItem) layout.Rect {
	r.t.Helper()
	r.client.openMenu(title, x, y, items)
	r.frame()
	target, ok := r.client.View().UI().Target("menu")
	require.True(r.t, ok, "no menu on screen")
	return layout.Rect{X: target.Rect.X, Y: target.Rect.Y, W: target.Rect.W, H: target.Rect.H}
}

func TestAMenuIsAsWideAsItsLongestLine(t *testing.T) {
	r := newRig(t, 100, 20)
	short, long := menuItem{label: "Zoom", action: "zoom"}, menuItem{label: "Even out panes", action: "equalize"}

	rect := r.menuRect("pane", 10, 4, short, long)
	assert.Equal(t, len("Even out panes")+4, rect.W, "the longest label, its padding and the border")
	assert.Equal(t, 10, rect.X, "a menu that fits stays where it was opened")
	assert.Contains(t, r.lines()[5], " Zoom ", "the menu still draws its items")

	rect = r.menuRect("a title longer than the item", 10, 4, short)
	assert.Equal(t, len("a title longer than the item")+4, rect.W, "a longer title sets the width")

	rect = r.menuRect("pane", 10, 4, menuItem{label: strings.Repeat("w", 90), action: "zoom"})
	assert.Equal(t, 40, rect.W, "a label longer than the widget's cap is cut")

	narrow := newRig(t, 30, 12)
	rect = narrow.menuRect("pane", 5, 2, menuItem{label: strings.Repeat("w", 90), action: "zoom"})
	assert.Equal(t, layout.Rect{X: 0, Y: 2, W: 30, H: 3}, rect, "a screen narrower than the cap is the limit")
}

func TestTheSessionChipListsTheSessions(t *testing.T) {
	r := newRig(t, 100, 16)
	other, err := r.mux.NewSession(mux.SessionOptions{Name: "other"})
	require.NoError(t, err)

	r.clickOn("session")
	require.Equal(t, "menu", r.client.overlay())
	screen := strings.Join(r.lines(), "\n")
	assert.Contains(t, screen, "● main")
	assert.Contains(t, screen, "other  1 win")
	r.clickItem(1)
	assert.Same(t, other, r.client.Session())
	info, _ := r.mux.Info(r.session)
	assert.Zero(t, info.Attached, "the client left its first session")

	r.keys("ctrl+b", "s")
	r.clickItem(2)
	assert.Len(t, r.mux.List(), 3, "the menu started a session")
	assert.NotSame(t, other, r.client.Session())

	x, y := r.at("session")
	r.mouse(input.Press, input.ButtonRight, x, y)
	r.clickItem(5)
	assert.Equal(t, Detached, r.client.Outcome())
}

func TestTabsThatDoNotFitStandBehindOneCell(t *testing.T) {
	r := newRig(t, 60, 12)
	r.withWindows("alpha-window", "beta-window", "gamma-window", "delta-window", "epsilon-window")
	r.mux.SelectWindow(r.session, 3)
	windows, _ := r.bars()
	assert.Contains(t, windows, "4:gamma-window", "the active window always shows")
	assert.Contains(t, windows, "…")
	assert.NotContains(t, windows, "1:sh")
	assert.LessOrEqual(t, len([]rune(strings.TrimRight(windows, " "))), 60)

	r.clickOn(idMore)
	require.Equal(t, "menu", r.client.overlay())
	assert.Contains(t, strings.Join(r.lines(), "\n"), "6:epsilon-window")
	r.clickItem(0)
	windows, _ = r.bars()
	assert.Contains(t, windows, "1:sh")

	r.keys("ctrl+b", "w")
	assert.Equal(t, "menu", r.client.overlay())
	x, y := r.at(idMore)
	r.keys("esc")
	r.mouse(input.Press, input.ButtonRight, x, y)
	assert.Equal(t, "menu", r.client.overlay())
}

func TestTheInformationRowAnswersClicks(t *testing.T) {
	r := newRig(t, 120, 12)
	_, info := r.bars()
	assert.Contains(t, info, "LOAD 0.52 ")
	r.clickOn("stat-load")
	_, info = r.bars()
	assert.Contains(t, info, "LOAD 0.52 0.40 0.31")

	r.clickOn("clock")
	_, info = r.bars()
	assert.Contains(t, info, "Wed 4 Mar 15:04:05")

	r.clickOn("dir")
	_, info = r.bars()
	assert.Contains(t, info, "copied ")
	assert.Equal(t, r.mux.ActivePane(r.session).Info().Dir, r.client.clip)
}

func TestHoverMarksTheElementUnderThePointer(t *testing.T) {
	r := newRig(t, 100, 12)
	r.withWindows("logs")
	x, y := r.at("tab-1")
	before := r.frame().At(x, y).Style
	r.mouse(input.Motion, input.ButtonNone, x, y)
	assert.Equal(t, "tab-1", r.client.hover)
	assert.NotEqual(t, before, r.frame().At(x, y).Style)
	x, y = r.at("zoom")
	r.mouse(input.Motion, input.ButtonNone, x, y)
	assert.Equal(t, "zoom", r.client.hover)
	r.mouse(input.Motion, input.ButtonNone, 5, 2)
	assert.Equal(t, "", r.client.hover, "the pointer left the bars")
}

func TestHelpListsTheKeysAndAnyKeyClosesIt(t *testing.T) {
	r := newRig(t, 100, 50)
	r.keys("ctrl+b", "?")
	screen := strings.Join(r.lines(), "\n")
	assert.Contains(t, screen, "gmux keys")
	assert.Contains(t, screen, "ctrl+b %")
	assert.Contains(t, screen, "split right")
	r.keys("a")
	assert.Equal(t, "", r.client.overlay())
	r.keys("ctrl+b", "?")
	r.mouse(input.Press, input.ButtonLeft, 50, 20)
	assert.Equal(t, "", r.client.overlay())
}

func TestAnErrorShowsOnTheBar(t *testing.T) {
	r := newRig(t, 2, 6)
	r.keys("ctrl+b", "%")
	assert.Contains(t, r.client.note, "too small")
}

func TestAProgramSettingTheClipboardReachesTheTerminal(t *testing.T) {
	r := newRig(t, 60, 12)
	stop := r.client.Start(r.program)
	defer stop()
	r.send(clipboardMsg{text: "from-a-program"})
	assert.Equal(t, "from-a-program", r.client.clip)
}

func TestPaneOutputAsksForFramesAtAHeldRate(t *testing.T) {
	r := newRig(t, 60, 12)
	r.client.pace(r.program)
	r.client.pace(r.program)
	r.client.paceMu.Lock()
	first := r.client.last
	r.client.paceMu.Unlock()
	assert.False(t, first.IsZero(), "the first request goes out at once")
	r.client.pace(r.program)
	r.eventually(func() bool {
		r.client.paceMu.Lock()
		defer r.client.paceMu.Unlock()
		return !r.client.pending && r.client.last.Sub(first) >= minFrame-time.Millisecond
	}, "the held request never went out")
}
