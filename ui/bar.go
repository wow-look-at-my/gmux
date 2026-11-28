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

// chip is one clickable cell of the window row. tip is what it says when the pointer rests on it.
type chip struct {
	id, action, label, look, tip string
	// close is the cell that closes a tab's window. A chip that is not a tab has none.
	close closer
	// mark is what a tab draws in the cell before its close cell, or nothing for a blank there.
	mark string
}

// closer is the cell at the end of a tab that closes the tab's window.
type closer struct {
	id, action, look, tip string
}

func (c chip) record() map[string]sema.Value {
	fields := map[string]sema.Value{
		"id":     sema.StringValue(c.id),
		"action": sema.StringValue(c.action),
		"label":  sema.StringValue(c.label),
		"look":   sema.StringValue(c.look),
		"tip":    sema.StringValue(c.tip),
	}
	if c.close.action != "" {
		fields["closeId"] = sema.StringValue(c.close.id)
		fields["closeAction"] = sema.StringValue(c.close.action)
		fields["closeLook"] = sema.StringValue(c.close.look)
		fields["closeTip"] = sema.StringValue(c.close.tip)
		fields["mark"] = sema.StringValue(c.mark)
		fields["blank"] = sema.BoolValue(c.mark == "")
	}
	return fields
}

func records(chips []chip) sema.Value {
	items := make([]map[string]sema.Value, 0, len(chips))
	for _, c := range chips {
		items = append(items, c.record())
	}
	return sema.RecordListValue(items)
}

// width is the cells a chip takes: its label, a cell of padding on each side, and the close cell with its gap.
func (c chip) width() int {
	width := cell.Width(c.label) + 2
	if c.close.action != "" {
		width += 2
	}
	return width
}

// The ids and actions of the bar's own elements.
const (
	tabPrefix   = "tab-"
	closePrefix = "close-"
	actSelect   = "select-window:"
	actKill     = "kill-window:"
	idNewWindow = "new-window"
	idMore      = "tabs-more"
	idMainMenu  = "main-menu"
	idZoom      = "zoom"
	idDetach    = "detach"
)

// newWindow is the cell after the last tab.
var newWindow = chip{id: idNewWindow, action: "new-window", label: "+", tip: "New window"}

// buttons are the cells at the right end of the window row. They act on this client's own view of the session,
// the way the buttons at the corner of a desktop window do. The zoom cell is there only when it does something:
// when the window has more than one pane.
func (c *Client) buttons(info mux.SessionInfo) []chip {
	var cells []chip
	for _, w := range info.Windows {
		if !w.Active || w.Panes < 2 {
			continue
		}
		zoom := chip{id: idZoom, action: "zoom", label: "□", tip: "Zoom: give the pane in focus the whole window"}
		if w.Zoomed {
			zoom.label, zoom.tip = "❐", "Zoomed. Click to show every pane of the window again"
		}
		cells = append(cells, zoom)
	}
	cells = append(cells, chip{
		id: idDetach, action: "detach", label: "─",
		tip: "Detach: leave gmux. The session and its programs keep running, and gmux session attach comes back to them",
	})
	for i := range cells {
		cells[i].look = c.look(cells[i].id, "chip")
	}
	return cells
}

// look picks a chip's style: the given, or the hover style while the pointer
// is on it.
func (c *Client) look(id, style string) string {
	if c.hover != id {
		return style
	}
	switch style {
	case "tab", "chip", "close":
		return style + ".hover"
	}
	return style
}

// windowNumber is the number a user knows a window by.
func windowNumber(index int) int { return index + 1 }

// numberedWindow is the window a digit key names.
func numberedWindow(digit int) int { return (digit + 9) % 10 }

// tabMark is the mark for what happened in a window nobody looks at. A tab
// draws it in the cell before its close cell, so the tab keeps its width.
func tabMark(w mux.WindowInfo) string {
	switch {
	case w.Bell:
		return "!"
	case w.Activity:
		return "#"
	}
	return ""
}

// tabLabel is what a window's tab says: its number and its name.
func tabLabel(w mux.WindowInfo) string {
	label := strconv.Itoa(windowNumber(w.Index)) + ":" + w.Name
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
		n, number := strconv.Itoa(i), strconv.Itoa(windowNumber(w.Index))
		id, closeID := tabPrefix+n, closePrefix+n
		all = append(all, chip{
			id: id, action: actSelect + n, label: tabLabel(w), mark: tabMark(w), look: c.look(id, style),
			tip: "Window " + number + ". Click to show it, drag to move it, click it again to rename it",
			close: closer{
				id: closeID, action: actKill + n, look: c.look(closeID, "close"),
				tip: "Close window " + number + " and end its programs",
			},
		})
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
		return chip{
			id: idMore, action: "windows", label: "+" + strconv.Itoa(hidden) + " …", look: c.look(idMore, "tab"),
			tip: "The windows with no room on the row. Click for the list",
		}
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

// menuWidth is the cells the menu cell takes at the left end of the window row.
const menuWidth = 3

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
	actions := c.buttons(info)
	plus := newWindow
	plus.look = c.look(plus.id, "chip")
	// The row also holds the menu cell, the session and the cell that adds a window.
	taken := menuWidth + cell.Width(info.Name) + 2 + plus.width()
	for _, b := range actions {
		taken += b.width()
	}
	props := tml.Props{
		"session":  sema.StringValue(info.Name),
		"menuLook": sema.StringValue(c.look(idMainMenu, "chip")),
		"tabs":     records(append(c.tabs(info, c.width-taken), plus)),
		"actions":  records(actions),
		"mode":     sema.StringValue(c.mode()),
		"clock":    sema.StringValue(c.clock()),
	}
	if pane := c.mux.ActivePane(c.session); pane != nil {
		p := pane.Info()
		dir := shortDir(p.Dir)
		props["dir"] = sema.StringValue(dir)
		// The directory's column takes room ahead of the gap beside it, up to what the directory needs.
		need := cell.Width(dir) + 2
		props["infoColumns"] = sema.StringValue("auto,1000*[" + strconv.Itoa(min(need, dirLeast)) + ".." +
			strconv.Itoa(need) + "],auto,auto,auto,*,auto,auto,auto,auto,auto")
		props["command"] = sema.StringValue(p.Command)
		props["size"] = sema.StringValue(strconv.Itoa(p.Cols) + "×" + strconv.Itoa(p.Rows))
	}
	if c.note != "" && c.now().Before(c.noteUntil) {
		props["note"] = sema.StringValue(c.note)
	}
	if c.stats != nil {
		bar := c.stats.Bar()
		props["cpu"] = sema.StringValue(bar.CPU)
		props["cpuHistory"] = sema.ListValue(bar.CPUHistory)
		props["mem"] = sema.StringValue(bar.Mem)
		props["net"] = sema.StringValue(bar.Net)
		props["gpu"] = sema.StringValue(bar.GPU)
		c.statsProps(props)
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
