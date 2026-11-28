// bindings.go holds the keys that follow the prefix, and the actions the keys, the bars and the menus all name.

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

// prefixKey is the key that says the next key is a command.
const prefixKey = "ctrl+b"

// repeatTime is how long a repeatable key may follow another without the prefix.
const repeatTime = 600 * time.Millisecond

// scrollStep is how many lines one notch of the wheel moves.
const scrollStep = 3

// binding is one key that follows the prefix.
type binding struct {
	key    string
	action string
	// help says what the key does. A binding with none is left out of the help.
	help string
	// repeat lets the key be pressed again without the prefix, for a moment.
	repeat bool
}

// label is the key as the help writes it.
func (b binding) label() string { return b.key }

var bindings = []binding{
	{key: "c", action: "new-window", help: "new window"},
	{key: "n", action: "step-window:1", help: "next window", repeat: true},
	{key: "p", action: "step-window:-1", help: "previous window", repeat: true},
	{key: "l", action: "last-window", help: "the window before this one"},
	{key: "1-9, 0", help: "window by number, 0 for the tenth"},
	{key: "w", action: "windows", help: "list of windows"},
	{key: ",", action: "rename-window", help: "rename window"},
	{key: "&", action: "kill-window", help: "close window"},
	{key: "%", action: "split-right", help: "split right"},
	{key: "\"", action: "split-down", help: "split down"},
	{key: "left", action: "focus:left", help: "focus the pane on that side", repeat: true},
	{key: "right", action: "focus:right", repeat: true},
	{key: "up", action: "focus:up", repeat: true},
	{key: "down", action: "focus:down", repeat: true},
	{key: "o", action: "step-pane:1", help: "next pane", repeat: true},
	{key: ";", action: "last-pane", help: "the pane before this one"},
	{key: "ctrl+left", action: "resize:left:1", help: "resize pane (alt: faster)", repeat: true},
	{key: "ctrl+right", action: "resize:right:1", repeat: true},
	{key: "ctrl+up", action: "resize:up:1", repeat: true},
	{key: "ctrl+down", action: "resize:down:1", repeat: true},
	{key: "alt+left", action: "resize:left:5", repeat: true},
	{key: "alt+right", action: "resize:right:5", repeat: true},
	{key: "alt+up", action: "resize:up:5", repeat: true},
	{key: "alt+down", action: "resize:down:5", repeat: true},
	{key: "space", action: "equalize", help: "even out panes"},
	{key: "{", action: "swap:-1", help: "swap pane with previous", repeat: true},
	{key: "}", action: "swap:1", help: "swap pane with next", repeat: true},
	{key: "z", action: "zoom", help: "zoom pane"},
	{key: "x", action: "kill-pane", help: "close pane"},
	{key: "[", action: "scroll", help: "scroll back (pgup too)"},
	{key: "pgup", action: "scroll"},
	{key: "s", action: "sessions", help: "sessions"},
	{key: "$", action: "rename-session", help: "rename session"},
	{key: "d", action: "detach", help: "detach"},
	{key: prefixKey, action: "send-prefix", help: "send ctrl+b itself"},
	{key: "?", action: "help", help: "this help"},
}

var sides = map[string]layout.Side{
	"left": layout.Left, "right": layout.Right, "up": layout.Up, "down": layout.Down,
}

// repeats reports whether a key may follow another without the prefix.
func repeats(name string) bool {
	for _, b := range bindings {
		if b.key == name {
			return b.repeat
		}
	}
	return false
}

// command runs the key that followed the prefix. It reports whether the prefix stays armed for a repeat.
func (c *Client) command(p *tml.Program, key tml.Key) bool {
	name := key.String()
	if len(name) == 1 && name[0] >= '0' && name[0] <= '9' {
		c.act(p, actSelect+strconv.Itoa(numberedWindow(int(name[0]-'0'))))
		return false
	}
	for _, b := range bindings {
		if b.key == name && b.action != "" {
			c.act(p, b.action)
			return b.repeat
		}
	}
	return false
}

// act runs one named action. An action can carry arguments after colons.
func (c *Client) act(p *tml.Program, action string) {
	name, arg, _ := strings.Cut(action, ":")
	s := c.session
	// A menu opens under the bar element that asked for it, or in the middle of the window.
	x, y := c.width/3, c.height/3
	if c.anchored {
		x, y = c.anchorX, c.anchorY
	}
	switch name {
	case "new-window":
		c.report(p, c.mux.NewWindow(s, mux.Spawn{}))
	case "select-window":
		c.mux.SelectWindow(s, number(arg))
	case "step-window":
		c.mux.StepWindow(s, number(arg))
	case "last-window":
		c.mux.LastWindow(s)
	case "kill-window":
		index := c.activeWindow()
		if arg != "" {
			index = number(arg)
		}
		c.mux.KillWindow(s, index)
	case "move-window":
		from, step, _ := strings.Cut(arg, ":")
		c.mux.MoveWindow(s, number(from), number(from)+number(step))
	case "rename-window":
		c.renameWindow(arg)
	case "windows":
		c.windowList(x, y)
	case "split-right":
		c.report(p, c.mux.Split(s, layout.Columns, mux.Spawn{}))
	case "split-down":
		c.report(p, c.mux.Split(s, layout.Rows, mux.Spawn{}))
	case "focus":
		c.mux.FocusSide(s, sides[arg])
	case "step-pane":
		c.mux.StepPane(s, number(arg))
	case "last-pane":
		c.mux.LastPane(s)
	case "resize":
		side, cells, _ := strings.Cut(arg, ":")
		c.mux.ResizePane(s, sides[side], number(cells))
	case "equalize":
		c.mux.Equalize(s)
	case "swap":
		c.mux.SwapPane(s, number(arg))
	case "zoom":
		c.mux.ToggleZoom(s)
	case "kill-pane":
		c.mux.KillPane(s)
	default:
		c.actClient(p, name, arg, x, y)
	}
}

// actClient runs the actions that change this client and not the session.
func (c *Client) actClient(p *tml.Program, name, arg string, x, y int) {
	pane := c.mux.ActivePane(c.session)
	switch name {
	case "scroll":
		if pane != nil {
			pane.Scroll(max(1, pane.Info().Rows/2))
		}
	case "paste":
		if pane != nil && c.clip != "" {
			pane.Paste(c.clip)
		}
	case "send-prefix":
		if pane != nil {
			pane.Key(input.ParseKey(prefixKey))
		}
	case "copy-dir":
		if pane != nil {
			dir := pane.Info().Dir
			c.clip = dir
			p.Copy(dir)
			c.say(p, "copied "+shortDir(dir))
		}
	case "main-menu":
		c.mainMenu(x, y)
	case "sessions":
		c.sessionMenu(x, y)
	case "switch-session":
		if next := c.sessionByID(number(arg)); next != nil && next != c.session {
			c.show(next)
		}
	case "new-session":
		next, err := c.mux.NewSession(mux.SessionOptions{Cols: c.width, Rows: max(1, c.height-barRows)})
		if c.report(p, err) {
			c.show(next)
		}
	case "rename-session":
		info, _ := c.mux.Info(c.session)
		c.openPrompt("rename session", info.Name, func(p *tml.Program, value string) {
			c.report(p, c.mux.RenameSession(c.session, value))
		})
	case "detach":
		c.quit(p, Detached)
	case "help":
		c.help = true
		c.menu, c.ask = nil, nil
	case "stats":
		c.statsOpen = !c.statsOpen
	case "clock":
		c.longClock = !c.longClock
	}
}

// renameWindow asks for a new name for a window: the given by number, else
// the active one.
func (c *Client) renameWindow(arg string) {
	index := c.activeWindow()
	if arg != "" {
		index = number(arg)
	}
	info, _ := c.mux.Info(c.session)
	if index < 0 || index >= len(info.Windows) {
		return
	}
	c.openPrompt("rename window", info.Windows[index].Name, func(_ *tml.Program, value string) {
		c.mux.RenameWindow(c.session, index, value)
	})
}

// activeWindow is the index of the window on screen.
func (c *Client) activeWindow() int {
	info, _ := c.mux.Info(c.session)
	for _, w := range info.Windows {
		if w.Active {
			return w.Index
		}
	}
	return -1
}

// report puts an error on the bar, and reports whether there was none.
func (c *Client) report(p *tml.Program, err error) bool {
	if err != nil {
		c.say(p, err.Error())
		return false
	}
	return true
}

// number reads an action's numeric argument.
func number(arg string) int {
	n, err := strconv.Atoi(arg)
	if err != nil {
		return -1
	}
	return n
}
