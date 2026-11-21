// bar.go holds what the view is drawn from: the window tabs, the buttons, and the information row.

package ui

import (
	"os"
	"strconv"
	"strings"

	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/tml"
	"github.com/wow-look-at-my/tml/cell"
	"github.com/wow-look-at-my/tml/sema"
)

// chip is one clickable cell of a bar.
type chip struct {
	id, action, label, look string
}

func (c chip) record() map[string]sema.Value {
	return map[string]sema.Value{
		"id":     sema.StringValue(c.id),
		"action": sema.StringValue(c.action),
		"label":  sema.StringValue(c.label),
		"look":   sema.StringValue(c.look),
	}
}

func records(chips []chip) sema.Value {
	items := make([]map[string]sema.Value, 0, len(chips))
	for _, c := range chips {
		items = append(items, c.record())
	}
	return sema.RecordListValue(items)
}

// width is the cells a chip takes: its label and a cell of padding on each side.
func (c chip) width() int { return cell.Width(c.label) + 2 }

// The ids and actions of the bar's own elements.
const (
	tabPrefix   = "tab-"
	actSelect   = "select-window:"
	idNewWindow = "new-window"
	idMore      = "tabs-more"
)

// buttons are the cells at the right of the window row.
var buttons = []chip{
	{id: idNewWindow, action: "new-window", label: "+"},
	{id: "split-right", action: "split-right", label: "┃"},
	{id: "split-down", action: "split-down", label: "━"},
	{id: "zoom", action: "zoom", label: "□"},
	{id: "kill-pane", action: "kill-pane", label: "×"},
	{id: "help", action: "help", label: "?"},
}

// look picks a chip's style: the given, or the hover style while the pointer
// is on it.
func (c *Client) look(id, style string) string {
	if c.hover == id && style == "tab" {
		return "tab.hover"
	}
	if c.hover == id && style == "chip" {
		return "chip.hover"
	}
	return style
}

// tabLabel is what a window's tab says: its number, its name, and a mark for what happened in it.
func tabLabel(w mux.WindowInfo) string {
	label := strconv.Itoa(w.Index) + ":" + w.Name
	switch {
	case w.Bell:
		label += "!"
	case w.Activity:
		label += "#"
	}
	if w.Zoomed {
		label += " Z"
	}
	return label
}

// tabs is the window row's tabs, cut to the room the row has. The tabs that do not fit stand behind one cell that
// opens the list of all windows, and the active window always shows.
func (c *Client) tabs(info mux.SessionInfo, room int) []chip {
	all := make([]chip, 0, len(info.Windows))
	active := 0
	for i, w := range info.Windows {
		style := "tab"
		switch {
		case c.drag.kind == dragTab && c.drag.moved && c.drag.index == i:
			style = "tab.drag"
		case w.Active:
			style, active = "tab.active", i
		case w.Bell:
			style = "tab.alert"
		case w.Activity:
			style = "tab.busy"
		}
		id := tabPrefix + strconv.Itoa(i)
		all = append(all, chip{id: id, action: actSelect + strconv.Itoa(i), label: tabLabel(w), look: c.look(id, style)})
	}
	total := 0
	for _, tab := range all {
		total += tab.width()
	}
	if total <= room || len(all) == 0 {
		return all
	}
	// Grow outward from the active tab while there is room for the tab and for the cell that names the rest.
	first, last := active, active
	more := func(hidden int) chip {
		return chip{id: idMore, action: "windows", label: "+" + strconv.Itoa(hidden) + " …", look: c.look(idMore, "tab")}
	}
	used := all[active].width()
	for {
		grew := false
		for _, next := range []int{last + 1, first - 1} {
			if next < 0 || next >= len(all) || next < first-1 || next > last+1 {
				continue
			}
			hidden := len(all) - (last - first + 2)
			reserve := 0
			if hidden > 0 {
				reserve = more(hidden).width()
			}
			if used+all[next].width()+reserve > room {
				continue
			}
			used += all[next].width()
			first, last = min(first, next), max(last, next)
			grew = true
		}
		if !grew {
			break
		}
	}
	shown := append([]chip(nil), all[first:last+1]...)
	if hidden := len(all) - len(shown); hidden > 0 {
		shown = append(shown, more(hidden))
	}
	return shown
}

// dirLeast is the room the working directory keeps on the bar while anything else can still give way.
const dirLeast = 24

// home is the user's home directory, which a path on the bar abbreviates.
var home = os.Getenv("HOME")

// shortDir writes a directory the way a prompt does.
func shortDir(dir string) string {
	if home != "" && (dir == home || strings.HasPrefix(dir, home+"/")) {
		return "~" + dir[len(home):]
	}
	return dir
}

// Props is what the view draws from.
func (c *Client) Props() tml.Props {
	info, _ := c.mux.Info(c.session)
	actions := make([]chip, 0, len(buttons))
	actionsWidth := 0
	for _, b := range buttons {
		b.look = c.look(b.id, "chip")
		actions = append(actions, b)
		actionsWidth += b.width()
	}
	sessionWidth := cell.Width(info.Name) + 2
	props := tml.Props{
		"session": sema.StringValue(info.Name),
		"tabs":    records(c.tabs(info, c.width-sessionWidth-actionsWidth)),
		"actions": records(actions),
		"mode":    sema.StringValue(c.mode()),
		"clock":   sema.StringValue(c.clock()),
	}
	if pane := c.mux.ActivePane(c.session); pane != nil {
		p := pane.Info()
		dir := shortDir(p.Dir)
		props["dir"] = sema.StringValue(dir)
		// The directory's column takes room ahead of the gap beside it, up to what the directory needs.
		need := cell.Width(dir) + 2
		props["infoColumns"] = sema.StringValue("auto,1000*[" + strconv.Itoa(min(need, dirLeast)) + ".." +
			strconv.Itoa(need) + "],auto,auto,auto,*,auto,auto,auto,auto,auto,auto")
		props["command"] = sema.StringValue(p.Command)
		props["size"] = sema.StringValue(strconv.Itoa(p.Cols) + "×" + strconv.Itoa(p.Rows))
	}
	if c.note != "" && c.now().Before(c.noteUntil) {
		props["note"] = sema.StringValue(c.note)
	}
	if c.stats != nil {
		bar := c.stats.Bar(c.detail)
		props["cpu"] = sema.StringValue(bar.CPU)
		props["cpuHistory"] = sema.ListValue(bar.CPUHistory)
		props["mem"] = sema.StringValue(bar.Mem)
		props["net"] = sema.StringValue(bar.Net)
		props["load"] = sema.StringValue(bar.Load)
		props["gpu"] = sema.StringValue(bar.GPU)
	}
	c.overlayProps(props)
	return props
}

// mode names what the keys do now, when it is not the usual thing.
func (c *Client) mode() string {
	switch {
	case c.prefix:
		return "PREFIX"
	case c.drag.kind == dragTab && c.drag.moved:
		return "MOVE"
	}
	if pane := c.mux.ActivePane(c.session); pane != nil && pane.Info().Offset > 0 {
		return "SCROLL"
	}
	return ""
}

func (c *Client) clock() string {
	if c.longClock {
		return c.now().Format("Mon 2 Jan 15:04:05")
	}
	return c.now().Format("15:04")
}
