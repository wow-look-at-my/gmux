// mouse.go holds what the pointer does: in the window, on the lines between panes, and on the bars.

package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/wow-look-at-my/go-tmux/layout"
	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/tml"
	"github.com/wow-look-at-my/tml/input"
)

type dragKind uint8

const (
	dragNone dragKind = iota
	// dragDivider moves the line between a pair of panes.
	dragDivider
	// dragSelect selects text in a pane.
	dragSelect
	// dragProgram belongs to the program in a pane, which asked for the pointer.
	dragProgram
	// dragTab moves a window's tab along the bar.
	dragTab
	// dragScroll holds the thumb of a pane's scrollbar.
	dragScroll
)

// drag is what the held button is doing. It owns the pointer until the button comes up.
type drag struct {
	kind dragKind
	ref  layout.Ref
	pane *mux.Pane
	rect layout.Rect
	// index is the window whose tab is held, and startX where the pointer took it.
	index, startX int
	// grab is the row of a scrollbar's thumb that the pointer holds.
	grab int
	// moved is set once the pointer has gone far enough for the press to be a drag and not a click.
	moved bool
}

// multiClick is how close in time a pair of presses on one cell must be to count together.
const multiClick = 400 * time.Millisecond

// clickTracker counts presses on one cell: a second selects a word, a third a line.
type clickTracker struct {
	at    time.Time
	x, y  int
	count int
}

func (t *clickTracker) press(now time.Time, x, y int) int {
	if x == t.x && y == t.y && now.Sub(t.at) <= multiClick {
		t.count = t.count%3 + 1
	} else {
		t.count = 1
	}
	t.at, t.x, t.y = now, x, y
	return t.count
}

// hit is the id of the innermost element with an id under a cell.
func (c *Client) hit(x, y int) string {
	id := ""
	for _, target := range c.view.UI().Targets() {
		r := target.Rect
		if target.ID != "" && x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H {
			id = target.ID
		}
	}
	return id
}

// mouse routes one pointer event. It reports true for an event the focus ring must not see.
func (c *Client) mouse(p *tml.Program, event tml.Mouse) bool {
	if c.drag.kind != dragNone && event.Action != input.Press {
		return c.dragging(p, event)
	}
	c.drag = drag{}
	if c.overlay() != "" {
		return c.overlayMouse(event)
	}
	if event.Y >= c.height-barRows {
		return c.barMouse(p, event)
	}
	if c.hover == "" && event.Action == input.Motion && event.Button == input.ButtonNone {
		// The pointer moving over the window changes nothing the client draws.
		p.Unchanged()
	}
	c.hover = ""
	c.windowMouse(p, event)
	return true
}

// overlayMouse leaves a menu or a prompt its own clicks, and closes it on a press anywhere else.
func (c *Client) overlayMouse(event tml.Mouse) bool {
	over := c.hit(event.X, event.Y)
	inside := over == menuID || strings.HasPrefix(over, "prompt") || over == "help-box"
	if c.menu != nil {
		// The pointer marks the item it is on, as the keys do.
		c.menu.hover = c.menuItemAt(event.X, event.Y)
	}
	if event.Action == input.Press && (!inside || c.help) {
		c.closeOverlay()
		return true
	}
	return !inside
}

// barMouse handles the pointer on the bars. A plain click is left to the focus ring, which reports the element's
// action. The middle button, a second click and a drag of a tab are handled here.
func (c *Client) barMouse(p *tml.Program, event tml.Mouse) bool {
	over := c.hit(event.X, event.Y)
	c.hover = over
	index, onTab := tabIndex(over)
	if event.Action != input.Press || !onTab {
		return false
	}
	switch event.Button {
	case input.ButtonMiddle:
		c.mux.KillWindow(c.session, index)
		return true
	case input.ButtonLeft:
		if c.click.press(c.now(), event.X, event.Y) == 2 {
			c.renameWindow(strconv.Itoa(index))
			return true
		}
		c.drag = drag{kind: dragTab, index: index, startX: event.X}
	}
	return false
}

// tabIndex is the window a tab's id names.
func tabIndex(id string) (int, bool) {
	rest, ok := strings.CutPrefix(id, tabPrefix)
	if !ok {
		return 0, false
	}
	index, err := strconv.Atoi(rest)
	return index, err == nil
}

// dragging moves whatever the held button holds, and lets go of it when the button comes up.
func (c *Client) dragging(p *tml.Program, event tml.Mouse) bool {
	d := &c.drag
	release := event.Action == input.Release
	x, y := event.X-d.rect.X, event.Y-d.rect.Y
	switch d.kind {
	case dragDivider:
		if !release {
			c.mux.DragDivider(c.session, d.ref, event.X, event.Y)
		}
	case dragProgram:
		d.pane.Mouse(event, clamp(x, d.rect.W), clamp(y, d.rect.H))
	case dragSelect:
		c.selecting(p, event, x, y)
	case dragScroll:
		if !release {
			scrollUnder(d.pane, d.rect.H, y, d.grab)
		}
	case dragTab:
		return c.movingTab(event)
	}
	if release {
		c.drag = drag{}
	}
	return true
}

// clamp keeps a coordinate inside an extent.
func clamp(v, extent int) int { return max(0, min(v, extent-1)) }

// selecting grows the selection under the pointer, scrolls when the pointer leaves the pane, and copies the text
// when the button comes up.
func (c *Client) selecting(p *tml.Program, event tml.Mouse, x, y int) {
	d := &c.drag
	if event.Action != input.Release {
		switch {
		case y < 0:
			d.pane.Scroll(1)
		case y >= d.rect.H:
			d.pane.Scroll(-1)
		}
		d.pane.SelectTo(clamp(x, d.rect.W), clamp(y, d.rect.H))
		return
	}
	c.copySelection(p, d.pane)
}

// copySelection puts a pane's selected text on the clipboard.
func (c *Client) copySelection(p *tml.Program, pane *mux.Pane) {
	text := pane.SelectedText()
	if text == "" {
		return
	}
	c.clip = text
	p.Copy(text)
	c.say(p, "copied "+strconv.Itoa(len([]rune(text)))+" chars")
}

// tabDragStart is how far the pointer moves before a press on a tab is a drag.
const tabDragStart = 2

// movingTab drags a window's tab. The window takes the place of the tab the pointer is on when the button comes up.
func (c *Client) movingTab(event tml.Mouse) bool {
	d := &c.drag
	if event.X-d.startX >= tabDragStart || d.startX-event.X >= tabDragStart {
		d.moved = true
	}
	if event.Action != input.Release {
		return d.moved
	}
	moved, from := d.moved, d.index
	c.drag = drag{}
	if !moved {
		// The press was a click, and the ring reports it.
		return false
	}
	if to, ok := c.tabAt(event.X); ok {
		c.mux.MoveWindow(c.session, from, to)
	}
	return true
}

// tabAt is the window whose tab holds a column of the bar: the nearest tab when the column is past either end.
func (c *Client) tabAt(x int) (int, bool) {
	best, bestGap, found := 0, 0, false
	for _, target := range c.view.UI().Targets() {
		index, ok := tabIndex(target.ID)
		if !ok {
			continue
		}
		gap := 0
		switch r := target.Rect; {
		case x < r.X:
			gap = r.X - x
		case x >= r.X+r.W:
			gap = x - (r.X + r.W - 1)
		}
		if !found || gap < bestGap {
			best, bestGap, found = index, gap, true
		}
	}
	return best, found
}

// windowMouse handles the pointer in the window area.
func (c *Client) windowMouse(p *tml.Program, event tml.Mouse) {
	view := c.mux.View(c.session)
	if event.Action == input.Press && event.Button == input.ButtonLeft {
		for _, divider := range view.Dividers {
			if divider.Rect.Contains(event.X, event.Y) {
				c.drag = drag{kind: dragDivider, ref: divider.Ref}
				return
			}
		}
	}
	var under *mux.PaneView
	for i := range view.Panes {
		if view.Panes[i].Rect.Contains(event.X, event.Y) {
			under = &view.Panes[i]
		}
	}
	if under == nil {
		return
	}
	pane, rect := under.Pane, under.Rect
	x, y := event.X-rect.X, event.Y-rect.Y
	if event.Action == input.Press && !under.Active {
		c.mux.SelectPane(c.session, pane)
	}
	if gutter := c.mux.Gutter(); gutter > 0 && x >= rect.W-gutter {
		c.scrollbarMouse(pane, rect, event, y)
		return
	}
	// Shift takes the pointer back from a program that asked for it.
	if event.Mod&input.Shift == 0 && pane.Mouse(event, x, y) {
		if event.Action == input.Press {
			c.drag = drag{kind: dragProgram, pane: pane, rect: rect}
		}
		return
	}
	switch event.Action {
	case input.Wheel:
		c.wheel(pane, event)
	case input.Press:
		c.press(p, pane, rect, event, x, y)
	}
}

// scrollbarMouse handles the pointer in a pane's gutter. The left button holds the thumb where it is pressed, and
// a press on the track brings the thumb's middle under the pointer. The wheel scrolls as it does over the pane.
func (c *Client) scrollbarMouse(pane *mux.Pane, rect layout.Rect, event tml.Mouse, y int) {
	if pane.Modes().AltScreen {
		return
	}
	if event.Action == input.Wheel {
		c.wheel(pane, event)
		return
	}
	if event.Action != input.Press || event.Button != input.ButtonLeft {
		return
	}
	info := pane.Info()
	start, size, ok := paneBar(info.History, info.Offset, rect.H).Thumb()
	if !ok {
		return
	}
	grab := size / 2
	if y >= start && y < start+size {
		grab = y - start
	}
	c.drag = drag{kind: dragScroll, pane: pane, rect: rect, grab: grab}
	scrollUnder(pane, rect.H, y, grab)
}

// scrollUnder scrolls a pane so that a row of its scrollbar's thumb is under a row of the bar.
func scrollUnder(pane *mux.Pane, rows, y, grab int) {
	info := pane.Info()
	pane.ScrollTo(info.History - paneBar(info.History, info.Offset, rows).At(y, grab))
}

// wheel scrolls a pane's history. A full-screen program has none, so it gets arrow keys, which is what a pager and
// an editor both scroll on.
func (c *Client) wheel(pane *mux.Pane, event tml.Mouse) {
	lines, key := scrollStep, input.KeyUp
	if event.Button == input.WheelDown {
		lines, key = -scrollStep, input.KeyDown
	} else if event.Button != input.WheelUp {
		return
	}
	if !pane.Modes().AltScreen {
		pane.Scroll(lines)
		return
	}
	for range scrollStep {
		pane.Key(tml.Key{Code: key})
	}
}

// press handles a button going down on a pane whose program does not take the pointer.
func (c *Client) press(p *tml.Program, pane *mux.Pane, rect layout.Rect, event tml.Mouse, x, y int) {
	switch event.Button {
	case input.ButtonLeft:
		by := mux.ByCell
		switch c.click.press(c.now(), event.X, event.Y) {
		case 2:
			by = mux.ByWord
		case 3:
			by = mux.ByLine
		}
		pane.SelectStart(x, y, by)
		c.drag = drag{kind: dragSelect, pane: pane, rect: rect}
	case input.ButtonMiddle:
		if c.clip != "" {
			pane.Paste(c.clip)
		}
	case input.ButtonRight:
		c.paneMenu(event.X, event.Y)
	}
}
