// statsbox.go holds the detail view of the host's readings: a box over the window that a click on a reading opens.

package ui

import (
	"strconv"

	"github.com/wow-look-at-my/go-tmux/stats"
	"github.com/wow-look-at-my/tml"
	"github.com/wow-look-at-my/tml/input"
	"github.com/wow-look-at-my/tml/sema"
)

// statsID is the id of the box's element.
const statsID = "stats-box"

// statsWidth is the cells the box takes on a terminal with that much room.
const statsWidth = 64

// statsProps adds what the box draws from. The box is not an overlay: the keys stay with the pane.
func (c *Client) statsProps(props tml.Props) {
	if !c.statsOpen {
		return
	}
	props["statsOpen"] = sema.BoolValue(true)
	props["statsWidth"] = sema.StringValue(strconv.Itoa(min(statsWidth, c.width)))
	props["statRows"] = statRows(c.stats.Detail())
}

func statRows(rows []stats.Row) sema.Value {
	items := make([]map[string]sema.Value, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]sema.Value{
			"label":  sema.StringValue(row.Label),
			"values": sema.ListValue(row.History),
			"max":    sema.StringValue(strconv.FormatFloat(row.Max, 'f', -1, 64)),
			"text":   sema.StringValue(row.Text),
		})
	}
	return sema.RecordListValue(items)
}

// statsMouse keeps the pointer from the pane under the box. A press on the box closes it.
func (c *Client) statsMouse(p *tml.Program, event tml.Mouse) bool {
	switch {
	case event.Action == input.Press:
		c.statsOpen = false
	case c.hover == "":
		p.Unchanged()
	}
	c.hover = ""
	return true
}
