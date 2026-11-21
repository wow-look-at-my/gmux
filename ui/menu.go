// menu.go holds what is drawn over the window: the menu, the prompt and the help.

package ui

import (
	"strconv"

	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/tml"
	"github.com/wow-look-at-my/tml/sema"
)

type menuItem struct{ label, action string }

// menu is a list of actions opened at a cell of the screen.
type menu struct {
	title string
	x, y  int
	items []menuItem
	// hover is the item the keys or the pointer are on, or negative.
	hover int
}

// menuID is the id of the menu's element. The view's <Menu> sizes itself and stays on screen by itself.
const menuID = "menu"

// openMenu shows a menu at a cell.
func (c *Client) openMenu(title string, x, y int, items []menuItem) {
	c.menu = &menu{title: title, x: x, y: y, items: items, hover: -1}
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

// menuItemOn is the item on a row of the open menu's box, or negative. The first and the last row are the border.
func (c *Client) menuItemOn(row int) int {
	if c.menu == nil || row < 1 || row > len(c.menu.items) {
		return -1
	}
	return row - 1
}

// menuItemAt is the item of the open menu under a cell of the screen, or negative.
func (c *Client) menuItemAt(x, y int) int {
	target, ok := c.view.UI().Target(menuID)
	if !ok || x < target.Rect.X || x >= target.Rect.X+target.Rect.W {
		return -1
	}
	return c.menuItemOn(y - target.Rect.Y)
}

// pick runs an item of the open menu and closes the menu.
func (c *Client) pick(p *tml.Program, index int) {
	m := c.menu
	c.closeOverlay()
	if m != nil && index >= 0 && index < len(m.items) {
		c.act(p, m.items[index].action)
	}
}

// overlayProps adds what the menu, the prompt and the help draw from.
func (c *Client) overlayProps(props tml.Props) {
	if m := c.menu; m != nil {
		labels := make([]string, len(m.items))
		for i, item := range m.items {
			labels[i] = item.label
		}
		props["menuOpen"] = sema.BoolValue(true)
		props["menuTitle"] = sema.StringValue(m.title)
		props["menuX"] = sema.StringValue(strconv.Itoa(m.x))
		props["menuY"] = sema.StringValue(strconv.Itoa(m.y))
		props["menuItems"] = sema.ListValue(labels)
		props["menuSelected"] = sema.StringValue(strconv.Itoa(m.hover))
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
	c.openMenu("window "+strconv.Itoa(windowNumber(index)), x, y, []menuItem{
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
