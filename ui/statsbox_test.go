package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wow-look-at-my/tml/input"
)

func TestAClickOnAReadingOpensTheDetailView(t *testing.T) {
	r := newRig(t, 120, 20)
	r.reader("")
	screen := func() string { return strings.Join(r.lines(), "\n") }
	assert.NotContains(t, screen(), "of 16 cores")

	r.clickOn("stat-cpu")
	shown := screen()
	for _, text := range []string{"host", "CPU", "of 16 cores", "NET ↓", "1.2K/s", "0.52 0.40 0.31", "3d 4h"} {
		assert.Contains(t, shown, text)
	}
	// Each row with a history draws it as a graph. The row with none draws no graph.
	assert.Contains(t, shown, "CPU    ▁▃▁ ")
	assert.Contains(t, shown, "NET ↓  ▁█▅ ")
	assert.Contains(t, shown, "UP      ")

	box, ok := r.client.View().UI().Target(statsID)
	require.True(t, ok)
	assert.Equal(t, r.width, box.Rect.X+box.Rect.W, "the box is at the right edge")
	assert.Equal(t, r.height-bottomRows, box.Rect.Y+box.Rect.H, "the box stands on the information row")

	// The box takes no key: what is typed still goes to the pane.
	assert.Empty(t, r.client.overlay())
	r.keys("s", "t", "i", "l", "l", "enter")
	r.shows("still")
	assert.Contains(t, screen(), "of 16 cores")

	// A click on another reading closes the box, and a press on the box does too. The pane gets neither.
	r.clickOn("stat-mem")
	assert.NotContains(t, screen(), "of 16 cores")
	r.clickOn("stat-net")
	assert.Contains(t, screen(), "of 16 cores")
	x, y := r.at(statsID)
	r.mouse(input.Press, input.ButtonLeft, x, y)
	assert.Equal(t, dragNone, r.client.drag.kind, "a press on the box selects nothing in the pane under it")
	assert.NotContains(t, screen(), "of 16 cores")
}

func TestTheDetailViewFitsANarrowTerminal(t *testing.T) {
	r := newRig(t, 40, 14)
	r.client.statsOpen = true
	box := func() int {
		r.frame()
		target, ok := r.client.View().UI().Target(statsID)
		require.True(t, ok)
		return target.Rect.W
	}
	assert.Equal(t, 40, box())
}
