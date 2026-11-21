// menu.go holds what is drawn over the window: the menu, the prompt and the help.

package ui

import (
	"strconv"

	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/tml"
	"github.com/wow-look-at-my/tml/cell"
	"github.com/wow-look-at-my/tml/sema"
)

type menuItem struct{ label, action string }

// menu is a list of actions opened at a cell of the screen.
type menu struct {
	title       string
	x, y, width int
	items       []menuItem
	// hover is the item the keys or the pointer are on, or negative.
	hover int
}

const menuPrefix = "menu-"

// menuCap is the widest a menu gets. A longer label is cut.
const menuCap = 40

// openMenu shows a menu near a cell, moved so that all of it is on screen. The menu is as wide as its longest
// line, up to the cap and the screen. Its rows fill it, so a menu with no width of its own takes the whole screen.
func (c *Client) openMenu(title string, x, y int, items []menuItem) {
	width := cell.Width(title) + 4
	for _, item := range items {
		width = max(width, cell.Width(item.label)+4)
	}
	width = min(width, menuCap, max(c.width, 1))
	height := len(items) + 2
	x = max(0, min(x, c.width-width))
	y = max(0, min(y, c.height-height))
	c.menu = &menu{title: title, x: x, y: y, width: width, items: items, hover: -1}
	c.ask, c.help = nil, false
}

// prompt asks for one line of text.
type prompt struct {
	title, value string
	cursor       int
	accept       func(p *tml.Program, value string)
}

// openPrompt asks for a line, starting from a value the user can edit.
func (c *Client) openPrompt(title, value string, accept func(p *tml.Program, value string)) {
	c.ask = &prompt{title: title, value: value, cursor: len([]rune(value)), accept: accept}
	c.menu, c.help = nil, false
}

// closeOverlay takes away whatever is drawn over the window.
func (c *Client) closeOverlay() {
	c.menu, c.ask, c.help = nil, nil, false
}

// overlayProps adds what the menu, the prompt and the help draw from.
func (c *Client) overlayProps(props tml.Props) {
	if m := c.menu; m != nil {
		items := make([]chip, 0, len(m.items))
		for i, item := range m.items {
			id := menuPrefix + strconv.Itoa(i)
			look := "menu.item"
			if i == m.hover || c.hover == id {
				look = "menu.item.hover"
			}
			items = append(items, chip{id: id, action: item.action, label: item.label, look: look})
		}
		props["menuOpen"] = sema.BoolValue(true)
		props["menuTitle"] = sema.StringValue(m.title)
		props["menuX"] = sema.StringValue(strconv.Itoa(m.x))
		props["menuY"] = sema.StringValue(strconv.Itoa(m.y))
		props["menuWidth"] = sema.StringValue(strconv.Itoa(m.width))
		props["menu"] = records(items)
	}
	if a := c.ask; a != nil {
		props["promptOpen"] = sema.BoolValue(true)
		props["promptTitle"] = sema.StringValue(a.title)
		props["promptValue"] = sema.StringValue(a.value)
		props["promptCursor"] = sema.StringValue(strconv.Itoa(a.cursor))
	}
	if c.help {
		lines := make([]map[string]sema.Value, 0, len(bindings))
		for _, b := range bindings {
			if b.help == "" {
				continue
			}
			lines = append(lines, map[string]sema.Value{
				"key":  sema.StringValue("ctrl+b " + b.label()),
				"what": sema.StringValue(b.help),
			})
		}
		props["helpOpen"] = sema.BoolValue(true)
		props["help"] = sema.RecordListValue(lines)
	}
}

// windowMenu is the actions on one window, opened from its tab.
func (c *Client) windowMenu(index, x, y int) {
	n := strconv.Itoa(index)
	c.openMenu("window "+n, x, y, []menuItem{
		{"Rename…", "rename-window:" + n},
		{"Move left", "move-window:" + n + ":-1"},
		{"Move right", "move-window:" + n + ":1"},
		{"New window", "new-window"},
		{"Close", "kill-window:" + n},
	})
}

// paneMenu is the actions on the active pane, opened in the window.
func (c *Client) paneMenu(x, y int) {
	items := []menuItem{
		{"Split right", "split-right"},
		{"Split down", "split-down"},
		{"Zoom", "zoom"},
		{"Even out panes", "equalize"},
		{"Scroll back", "scroll"},
	}
	if c.clip != "" {
		items = append(items, menuItem{"Paste", "paste"})
	}
	items = append(items, menuItem{"Close pane", "kill-pane"})
	c.openMenu("pane", x, y, items)
}

// sessionMenu lists the sessions to switch to, and what can be done to this.
func (c *Client) sessionMenu(x, y int) {
	var items []menuItem
	for _, info := range c.mux.List() {
		label := info.Name + "  " + strconv.Itoa(len(info.Windows)) + " win"
		if info.ID == c.session.ID {
			label = "● " + label
		} else {
			label = "  " + label
		}
		items = append(items, menuItem{label, "switch-session:" + strconv.Itoa(info.ID)})
	}
	items = append(items,
		menuItem{"New session", "new-session"},
		menuItem{"Rename…", "rename-session"},
		menuItem{"Detach", "detach"},
	)
	c.openMenu("sessions", x, y, items)
}

// windowList lists every window of the session.
func (c *Client) windowList(x, y int) {
	info, _ := c.mux.Info(c.session)
	items := make([]menuItem, 0, len(info.Windows))
	for _, w := range info.Windows {
		items = append(items, menuItem{tabLabel(w), actSelect + strconv.Itoa(w.Index)})
	}
	c.openMenu("windows", x, y, items)
}

// sessionByID is the session with an id, among those that still run.
func (c *Client) sessionByID(id int) *mux.Session {
	for _, info := range c.mux.List() {
		if info.ID == id {
			if s, err := c.mux.Find(info.Name); err == nil {
				return s
			}
		}
	}
	return nil
}
