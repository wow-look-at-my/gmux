package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wow-look-at-my/go-tmux/layout"
	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/tml/input"
)

func TestTheWindowRowIsOverTheWindowAndTheInformationRowUnderIt(t *testing.T) {
	r := newRig(t, 100, 12)
	r.typeLine("clear; echo first-row")
	r.eventually(func() bool { return strings.HasPrefix(r.lines()[1], "first-row") },
		"the first row of the window is not the row under the window row")
	lines := r.lines()

	assert.True(t, strings.HasPrefix(lines[0], " ≡  main  1:sh × "), "the first row is the window row: %q", lines[0])
	assert.Contains(t, lines[0], " + ")
	assert.Contains(t, lines[11], "CPU", "the last row is the information row")
	assert.NotContains(t, lines[11], "1:sh")

	// The pointer finds a pane where the pane is drawn: one row under where the pane's own rows count from.
	require.NoError(t, r.mux.Split(r.session, layout.Rows, mux.Spawn{}))
	upper, lower := r.mux.View(r.session).Panes[0], r.mux.View(r.session).Panes[1]
	if upper.Rect.Y > lower.Rect.Y {
		upper, lower = lower, upper
	}
	require.True(t, lower.Active, "the new pane has the focus")
	r.click(input.ButtonLeft, 2, topRows+upper.Rect.Y+upper.Rect.H-1)
	for _, pane := range r.mux.View(r.session).Panes {
		assert.Equal(t, pane.Rect.Y == upper.Rect.Y, pane.Active, "the last row of the upper pane belongs to the upper pane")
	}
}

// Every window has the cell that closes it on its own tab, and nothing else on the row closes anything.
func TestEachTabClosesItsOwnWindow(t *testing.T) {
	r := newRig(t, 100, 12)
	r.withQuietWindows("logs", "edit")
	windows, _ := r.bars()
	assert.Contains(t, windows, " 1:sh ×  2:logs ×  3:edit × ")
	assert.Equal(t, 3, strings.Count(windows, "×"), "one close cell for each tab, and no other: %q", windows)

	x, y := r.at(closePrefix + "1")
	r.mouse(input.Press, input.ButtonLeft, x, y)
	assert.Equal(t, closePrefix+"1", r.client.hover, "the press is on the close cell, not on its tab")
	assert.Equal(t, "×", r.frame().At(x, y).Text)
	r.mouse(input.Release, input.ButtonLeft, x, y)
	require.Eventually(t, func() bool { return r.windows() == "[sh] edit" }, wait, 5*time.Millisecond,
		"the window of the tab never closed: %s", r.windows())
	windows, _ = r.bars()
	assert.Equal(t, 2, strings.Count(windows, "×"))

	// A click on the close cell is not a click on its tab: the window on screen does not change first.
	r.clickOn(closePrefix + "1")
	r.eventually(func() bool { return r.windows() == "[sh]" }, "the second window never closed")
}

// The tab of the window on screen closes its window the same way, and the client shows the window that is left.
func TestTheCloseCellOfTheTabOnScreenClosesIt(t *testing.T) {
	r := newRig(t, 100, 12)
	r.withQuietWindows("logs", "edit")
	r.mux.SelectWindow(r.session, 2)
	r.frame()
	require.Equal(t, "sh logs [edit]", r.windows())

	x, y := r.at(closePrefix + "2")
	r.mouse(input.Press, input.ButtonLeft, x, y)
	assert.Equal(t, closePrefix+"2", r.client.hover)
	r.mouse(input.Release, input.ButtonLeft, x, y)
	require.Eventually(t, func() bool { return len(strings.Fields(r.windows())) == 2 }, wait, 5*time.Millisecond,
		"the window on screen never closed: %s", r.windows())
	assert.NotContains(t, r.windows(), "edit")
}

// A click on a tab, a click on another cell, and a click on the tab again are not a second click on the tab.
func TestAPressElsewhereEndsTheWaitForASecondClick(t *testing.T) {
	r := newRig(t, 100, 12)
	r.withQuietWindows("logs")
	x, y := r.at(tabPrefix + "1")
	r.click(input.ButtonLeft, x, y)
	r.clickOn(idNewWindow)
	r.click(input.ButtonLeft, x, y)
	assert.Equal(t, "", r.client.overlay(), "no rename prompt opened")

	r.click(input.ButtonLeft, x, y)
	assert.Equal(t, "prompt", r.client.overlay(), "a click straight after the last one is the second click")
}

// A program that takes the pointer gets every click in its pane. The menu cell of the window row still splits.
func TestTheMenuCellSplitsUnderAProgramThatTakesThePointer(t *testing.T) {
	r := newRig(t, 100, 24)
	r.reader(`printf '\033[?1000h\033[?1006h'; `)
	r.eventually(func() bool { return r.mux.ActivePane(r.session).Modes().Mouse != 0 }, "the program never asked for the pointer")

	r.click(input.ButtonRight, 10, 6)
	assert.Equal(t, "", r.client.overlay(), "the right click went to the program, so no pane menu opened")

	r.clickOn(idMainMenu)
	require.Equal(t, "menu", r.client.overlay())
	screen := strings.Join(r.lines(), "\n")
	for _, item := range []string{"New window", "Split right", "Split down", "Zoom the pane", "Close pane", "Detach", "Keys…"} {
		assert.Contains(t, screen, item)
	}
	menu, ok := r.client.View().UI().Target(menuID)
	require.True(t, ok)
	assert.Equal(t, 1, menu.Rect.Y, "the menu opens under the window row")
	assert.Equal(t, 0, menu.Rect.X)

	r.clickItem(1)
	assert.Equal(t, "", r.client.overlay())
	require.Len(t, r.mux.View(r.session).Panes, 2, "the menu split the window")

	r.clickOn(idMainMenu)
	r.clickItem(2)
	require.Len(t, r.mux.View(r.session).Panes, 3)

	// The same menu says what the zoom item does now.
	r.clickOn(idMainMenu)
	r.clickItem(3)
	assert.True(t, r.mux.View(r.session).Zoomed)
	r.clickOn(idMainMenu)
	assert.Contains(t, strings.Join(r.lines(), "\n"), "Show every pane")
	r.keys("esc")

	// The right button on the menu cell opens it too.
	x, y := r.at(idMainMenu)
	r.mouse(input.Press, input.ButtonRight, x, y)
	assert.Equal(t, "menu", r.client.overlay())
}

func TestTheMenuCellReachesTheSessionsAndTheKeys(t *testing.T) {
	r := newRig(t, 100, 50)
	r.clickOn(idMainMenu)
	labels := make([]string, 0, len(r.client.menu.items))
	for _, item := range r.client.menu.items {
		labels = append(labels, item.label)
	}
	index := func(label string) int {
		for i, have := range labels {
			if have == label {
				return i
			}
		}
		t.Fatalf("the menu has no item %q: %v", label, labels)
		return -1
	}
	assert.NotContains(t, labels, "Paste", "there is nothing to paste yet")

	r.clickItem(index("Sessions…"))
	require.Equal(t, "menu", r.client.overlay())
	assert.Contains(t, strings.Join(r.lines(), "\n"), "● main")
	r.keys("esc")

	r.clickOn(idMainMenu)
	r.clickItem(index("Keys…"))
	assert.Equal(t, "help", r.client.overlay())
	r.keys("a")

	r.client.clip = "copied text"
	r.clickOn(idMainMenu)
	assert.Contains(t, strings.Join(r.lines(), "\n"), "Paste")
	r.keys("esc")

	r.clickOn(idMainMenu)
	r.clickItem(index("Detach"))
	assert.Equal(t, Detached, r.client.Outcome())
}

func TestTheBarsSayWhatTheirCellsDo(t *testing.T) {
	r := newRig(t, 140, 12)
	r.withQuietWindows("logs")
	require.NoError(t, r.mux.Split(r.session, layout.Columns, mux.Spawn{}))
	r.frame()

	want := map[string]string{
		idMainMenu:        "Menu: split the window",
		"session":         "Click for the list of sessions",
		tabPrefix + "0":   "Window 1. Click to show it",
		closePrefix + "1": "Close window 2",
		idNewWindow:       "New window",
		idZoom:            "Zoom: give the pane in focus the whole window",
		idDetach:          "Detach: leave gmux. The session and its programs keep running",
		"dir":             "Click to copy it",
		"stat-cpu":        "Processor use of the whole host",
		"clock":           "Click to show the date",
	}
	for id, text := range want {
		tip, _ := r.tooltip(id)
		assert.Contains(t, tip, text, "the tooltip of %q", id)
	}

	// A tooltip of the window row shows under the row. It has no border and no title, so it is not a menu.
	tip, lines := r.tooltip(idNewWindow)
	assert.Contains(t, lines[1], " "+tip+" ")
	assert.NotContains(t, strings.Join(lines, "\n"), "╭", "a tooltip has no box around it")
	x, _ := r.at(idNewWindow)
	theme := r.client.View().Theme()
	drawn := r.frame().At(x+1, 1).Style
	assert.Equal(t, theme["tooltip"], drawn.Bg)
	assert.Equal(t, theme["tooltipText"], drawn.Fg)
	assert.NotEqual(t, theme["menu"], theme["tooltip"], "a tooltip does not take the surface of a menu")

	// A tooltip of the information row shows over the row.
	tip, lines = r.tooltip("clock")
	assert.Contains(t, lines[10], tip)

	// The pointer moving takes the tooltip away.
	r.mouse(input.Motion, input.ButtonNone, 3, 5)
	_, shown := r.client.View().UI().Tooltip()
	assert.False(t, shown)
	assert.NotContains(t, strings.Join(r.lines(), "\n"), "Click to show the date")

	r.clickOn(idZoom)
	tip, _ = r.tooltip(idZoom)
	assert.Contains(t, tip, "Zoomed. Click to show every pane")
}
