// update.go holds what the client does with each message of its program.

package ui

import (
	"strconv"
	"strings"

	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/tml"
)

// Update takes one message. It keeps every key and pointer event that belongs to a pane or to a command. It
// leaves the view's own controls to the focus ring.
func (c *Client) Update(p *tml.Program, msg tml.Msg) bool {
	switch msg := msg.(type) {
	case tml.Resize:
		c.width, c.height = msg.Width, msg.Height
		c.attach()
	case tml.Key:
		return c.key(p, msg)
	case tml.Paste:
		if c.overlay() != "" {
			return false
		}
		if pane := c.mux.ActivePane(c.session); pane != nil {
			pane.Paste(msg.Text)
		}
		return true
	case tml.Mouse:
		return c.mouse(p, msg)
	case tml.Event:
		c.event(p, msg)
	case endedMsg:
		c.ended(p, msg.session)
	case clipboardMsg:
		c.clip = msg.text
		p.Copy(msg.text)
	case disarmMsg:
		c.disarm()
	case closeMsg:
		c.quit(p, ServerClosed)
	}
	return false
}

// ended moves the client off a session that ended: to another session when one is left, else out.
func (c *Client) ended(p *tml.Program, s *mux.Session) {
	if s != c.session {
		return
	}
	next, err := c.mux.Find("")
	if err != nil {
		c.quit(p, SessionEnded)
		return
	}
	c.show(next)
}

// key routes one key press.
func (c *Client) key(p *tml.Program, key tml.Key) bool {
	name := key.String()
	switch c.overlay() {
	case "prompt":
		if name == "esc" {
			c.closeOverlay()
			return true
		}
		// The field edits itself through the focus ring, and enter comes back as its action.
		return false
	case "menu":
		c.menuKey(p, name)
		return true
	case "help":
		c.closeOverlay()
		return true
	}
	if c.prefix {
		// A prefix kept on for a repeat only takes a key that repeats. Any other key is typed as usual.
		repeating := c.cancelArmed != nil
		c.disarm()
		if !repeating || repeats(name) {
			if c.command(p, key) {
				c.arm(p)
			}
			return true
		}
	}
	if name == prefixKey {
		c.prefix = true
		return true
	}
	pane := c.mux.ActivePane(c.session)
	if pane == nil {
		return true
	}
	if pane.Info().Offset > 0 && c.scrollKey(pane, name) {
		return true
	}
	pane.Key(key)
	return true
}

// arm keeps the prefix on for a moment, so a repeatable key can follow without it.
func (c *Client) arm(p *tml.Program) {
	c.prefix = true
	c.cancelArmed = p.After(repeatTime, disarmMsg{})
}

func (c *Client) disarm() {
	c.prefix = false
	if c.cancelArmed != nil {
		c.cancelArmed()
		c.cancelArmed = nil
	}
}

// scrollKey moves a scrolled-back view. It reports false for a key that is not a scroll key. This then goes to
// the program and returns the view to the live screen.
func (c *Client) scrollKey(pane *mux.Pane, name string) bool {
	info := pane.Info()
	page := max(1, info.Rows-1)
	switch name {
	case "up", "k":
		pane.Scroll(1)
	case "down", "j":
		pane.Scroll(-1)
	case "pgup", "ctrl+b":
		pane.Scroll(page)
	case "pgdown", "ctrl+f", "space":
		pane.Scroll(-page)
	case "ctrl+u":
		pane.Scroll(page / 2)
	case "ctrl+d":
		pane.Scroll(-page / 2)
	case "home", "g":
		pane.ScrollTo(info.History)
	case "end", "G", "q", "esc", "enter":
		pane.ScrollTo(0)
	default:
		return false
	}
	return true
}

// menuKey moves through an open menu.
func (c *Client) menuKey(p *tml.Program, name string) {
	m := c.menu
	switch name {
	case "up", "k", "shift+tab":
		m.hover = (max(m.hover, 0) - 1 + len(m.items)) % len(m.items)
	case "down", "j", "tab":
		m.hover = (m.hover + 1) % len(m.items)
	case "enter", "space":
		if m.hover >= 0 && m.hover < len(m.items) {
			action := m.items[m.hover].action
			c.closeOverlay()
			c.act(p, action)
			return
		}
		c.closeOverlay()
	default:
		c.closeOverlay()
	}
}

// event takes what the focus ring made of an input value: a click on a bar element or a wheel turn over one.
// This also covers a right click, or an edit of the prompt.
func (c *Client) event(p *tml.Program, event tml.Event) {
	switch event.Kind {
	case tml.Activated:
		c.activated(p, event)
	case tml.Changed:
		if c.ask != nil && event.ID == "prompt-input" {
			c.ask.value, c.ask.cursor = event.Value, event.Cursor
		}
	case tml.Scrolled:
		// The wheel over the window row turns through the windows.
		if event.ID == "session" || event.ID == idMore || event.ID == "bar-windows" ||
			strings.HasPrefix(event.ID, tabPrefix) {
			c.mux.StepWindow(c.session, event.Delta)
		}
	case tml.Menu:
		c.contextMenu(event)
	}
}

// activated runs the action of the element that was clicked.
func (c *Client) activated(p *tml.Program, event tml.Event) {
	if event.Action == "prompt" {
		if ask := c.ask; ask != nil {
			c.closeOverlay()
			ask.accept(p, ask.value)
		}
		return
	}
	if event.Action == "" {
		return
	}
	menuItem := strings.HasPrefix(event.ID, menuPrefix)
	if c.overlay() != "" && !menuItem {
		return
	}
	c.closeOverlay()
	c.act(p, event.Action)
}

// contextMenu opens the menu of the bar element under a right click.
func (c *Client) contextMenu(event tml.Event) {
	target, ok := c.view.UI().Target(event.ID)
	if !ok {
		return
	}
	x, y := target.Rect.X+max(0, event.X), target.Rect.Y+max(0, event.Y)
	switch {
	case strings.HasPrefix(event.ID, tabPrefix):
		index, err := strconv.Atoi(strings.TrimPrefix(event.ID, tabPrefix))
		if err == nil {
			c.windowMenu(index, x, y)
		}
	case event.ID == "session":
		c.sessionMenu(x, y)
	case event.ID == idMore:
		c.windowList(x, y)
	}
}
