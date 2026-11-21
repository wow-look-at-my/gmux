// scrollbar_test.go covers the scrollbar in a pane's gutter.

package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wow-look-at-my/tml/input"
)

// gutterColumn is the last column of the window area, top to bottom: where
// the pane's scrollbar is drawn.
func (r *rig) gutterColumn() string {
	var column strings.Builder
	for _, line := range r.lines()[:r.height-barRows] {
		cells := []rune(line)
		column.WriteRune(cells[r.width-1])
	}
	return column.String()
}

func TestAPaneWithHistoryShowsAScrollbar(t *testing.T) {
	r := newRigWith(t, 60, 12, Gutter)
	pane := r.mux.ActivePane(r.session)
	require.Equal(t, 59, pane.Info().Cols, "the terminal leaves the gutter free")
	assert.Equal(t, strings.Repeat(" ", 10), r.gutterColumn(), "a pane with nothing to scroll back to has no bar")

	r.typeLine("i=0; while [ $i -lt 58 ]; do echo line-$i; i=$((i+1)); done")
	r.shows("line-57")
	r.eventually(func() bool { return pane.Info().History == 50 }, "the history never filled")
	assert.Equal(t, "│││││││││█", r.gutterColumn(), "the live screen is the end of the bar")

	pane.ScrollTo(50)
	assert.Equal(t, "█│││││││││", r.gutterColumn(), "the oldest line is its start")
	pane.ScrollTo(25)
	assert.Equal(t, "││││█│││││", r.gutterColumn())
}

func TestThePointerMovesAPanesScrollbar(t *testing.T) {
	r := newRigWith(t, 60, 12, Gutter)
	pane := r.mux.ActivePane(r.session)
	r.typeLine("i=0; while [ $i -lt 58 ]; do echo line-$i; i=$((i+1)); done")
	r.shows("line-57")
	r.eventually(func() bool { return pane.Info().History == 50 }, "the history never filled")
	bar := r.width - 1

	// A press on the track brings the thumb under the pointer.
	r.mouse(input.Press, input.ButtonLeft, bar, 0)
	assert.Equal(t, 50, pane.Info().Offset)
	// The held thumb follows the pointer, past the pane's edge too.
	r.mouse(input.Motion, input.ButtonLeft, bar-20, 5)
	assert.Equal(t, 22, pane.Info().Offset)
	r.mouse(input.Motion, input.ButtonLeft, bar, 30)
	assert.Zero(t, pane.Info().Offset)
	r.mouse(input.Release, input.ButtonLeft, bar, 30)
	r.mouse(input.Motion, input.ButtonNone, bar, 0)
	assert.Zero(t, pane.Info().Offset, "a pointer that holds nothing moves nothing")

	// The wheel over the bar scrolls as it does over the pane.
	r.mouse(input.Wheel, input.WheelUp, bar, 3)
	assert.Equal(t, scrollStep, pane.Info().Offset)
	pane.ScrollTo(0)

	// The gutter is no part of the terminal: a press there selects no text and reaches no program.
	r.click(input.ButtonRight, bar, 3)
	assert.Equal(t, "", r.client.overlay())
}

func TestAFullScreenProgramHasNoScrollbar(t *testing.T) {
	r := newRigWith(t, 60, 12, Gutter)
	pane := r.mux.ActivePane(r.session)
	r.typeLine("i=0; while [ $i -lt 58 ]; do echo line-$i; i=$((i+1)); done")
	r.shows("line-57")
	r.reader(`printf '\033[?1049h'; `)
	r.eventually(func() bool { return pane.Modes().AltScreen }, "never full screen")
	assert.Equal(t, strings.Repeat(" ", 10), r.gutterColumn())
	r.mouse(input.Press, input.ButtonLeft, r.width-1, 0)
	r.mouse(input.Release, input.ButtonLeft, r.width-1, 0)
	assert.Zero(t, pane.Info().Offset)
}
