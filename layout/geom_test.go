package layout

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRect(t *testing.T) {
	r := Rect{2, 3, 4, 5}
	assert.True(t, r.Contains(2, 3))
	assert.True(t, r.Contains(5, 7))
	assert.False(t, r.Contains(6, 3))
	assert.False(t, r.Contains(2, 8))
	assert.False(t, r.Contains(1, 3))
	assert.False(t, r.Empty())
	assert.True(t, Rect{0, 0, 0, 5}.Empty())
	assert.True(t, Rect{0, 0, 5, -1}.Empty())
	assert.False(t, Rect{0, 0, 0, 5}.Contains(0, 0))
}

func pair(dir Dir) *Tree {
	tr := New(1)
	tr.Split(1, dir, 2, false)
	return tr
}

func TestLayoutColumns(t *testing.T) {
	f := pair(Columns).Layout(Rect{2, 3, 11, 4})
	assert.Equal(t, Rect{2, 3, 11, 4}, f.Area)
	assert.Equal(t, []Placed{{1, Rect{2, 3, 5, 4}}, {2, Rect{8, 3, 5, 4}}}, f.Panes)
	require.Len(t, f.Dividers, 1)
	assert.Equal(t, Rect{7, 3, 1, 4}, f.Dividers[0].Rect)
	assert.Equal(t, Columns, f.Dividers[0].Dir)
}

func TestLayoutRows(t *testing.T) {
	f := pair(Rows).Layout(Rect{0, 0, 6, 7})
	assert.Equal(t, []Placed{{1, Rect{0, 0, 6, 3}}, {2, Rect{0, 4, 6, 3}}}, f.Panes)
	require.Len(t, f.Dividers, 1)
	assert.Equal(t, Rect{0, 3, 6, 1}, f.Dividers[0].Rect)
	assert.Equal(t, Rows, f.Dividers[0].Dir)
}

func TestLayoutTooSmall(t *testing.T) {
	f := pair(Columns).Layout(Rect{0, 0, 2, 4})
	assert.Equal(t, []Placed{{1, Rect{0, 0, 2, 4}}, {2, Rect{2, 0, 0, 4}}}, f.Panes)
	assert.Empty(t, f.Dividers)

	f = pair(Rows).Layout(Rect{0, 0, 4, 1})
	assert.Equal(t, []Placed{{1, Rect{0, 0, 4, 1}}, {2, Rect{0, 1, 4, 0}}}, f.Panes)
	assert.Empty(t, f.Dividers)

	f = pair(Columns).Layout(Rect{0, 0, 10, 0})
	assert.Empty(t, f.Dividers, "no cross-axis space")

	f = pair(Columns).Layout(Rect{1, 1, -5, -3})
	assert.Equal(t, Rect{1, 1, 0, 0}, f.Area)
	for _, p := range f.Panes {
		assert.True(t, p.Rect.Empty())
		assert.GreaterOrEqual(t, p.Rect.W, 0)
		assert.GreaterOrEqual(t, p.Rect.H, 0)
	}
	assert.Empty(t, f.Dividers)
}

func TestLayoutClampsMinimum(t *testing.T) {
	tr := pair(Columns)
	ref := tr.Layout(Rect{0, 0, 30, 1}).Dividers[0].Ref
	require.True(t, tr.Drag(ref, 29, 0, Rect{0, 0, 30, 1}))
	f := tr.Layout(Rect{0, 0, 3, 1})
	assert.Equal(t, []Placed{{1, Rect{0, 0, 1, 1}}, {2, Rect{2, 0, 1, 1}}}, f.Panes)
	require.True(t, tr.Drag(ref, 0, 0, Rect{0, 0, 30, 1}))
	f = tr.Layout(Rect{0, 0, 3, 1})
	assert.Equal(t, []Placed{{1, Rect{0, 0, 1, 1}}, {2, Rect{2, 0, 1, 1}}}, f.Panes)
}

func TestFrameLookup(t *testing.T) {
	f := pair(Columns).Layout(Rect{0, 0, 11, 4})
	r, ok := f.Pane(2)
	assert.True(t, ok)
	assert.Equal(t, Rect{6, 0, 5, 4}, r)
	_, ok = f.Pane(9)
	assert.False(t, ok)

	id, ok := f.PaneAt(0, 0)
	assert.True(t, ok)
	assert.Equal(t, ID(1), id)
	id, ok = f.PaneAt(10, 3)
	assert.True(t, ok)
	assert.Equal(t, ID(2), id)
	_, ok = f.PaneAt(5, 0)
	assert.False(t, ok, "divider cell")
	_, ok = f.PaneAt(11, 0)
	assert.False(t, ok)

	d, ok := f.DividerAt(5, 2)
	assert.True(t, ok)
	assert.Equal(t, Rect{5, 0, 1, 4}, d.Rect)
	_, ok = f.DividerAt(4, 2)
	assert.False(t, ok)

	small := pair(Columns).Layout(Rect{0, 0, 2, 4})
	id, ok = small.PaneAt(1, 1)
	assert.True(t, ok)
	assert.Equal(t, ID(1), id, "empty rects never match")
}

func grid() (*Tree, Rect) {
	tr := New(1)
	tr.Split(1, Columns, 3, false)
	tr.Split(1, Rows, 2, false)
	tr.Split(3, Rows, 4, false)
	return tr, Rect{0, 0, 21, 9}
}

func TestNeighbor(t *testing.T) {
	tr := New(1)
	tr.Split(1, Columns, 2, false)
	tr.Split(2, Rows, 3, false)
	area := Rect{0, 0, 21, 9}
	cases := []struct {
		id   ID
		side Side
		want ID
		ok   bool
	}{
		{1, Right, 2, true},
		{3, Left, 1, true},
		{2, Left, 1, true},
		{2, Down, 3, true},
		{3, Up, 2, true},
		{1, Left, 0, false},
		{1, Up, 0, false},
		{3, Down, 0, false},
		{2, Right, 0, false},
		{9, Right, 0, false},
	}
	for _, c := range cases {
		got, ok := tr.Neighbor(c.id, c.side, area)
		assert.Equal(t, c.ok, ok, "%d %d", c.id, c.side)
		if c.ok {
			assert.Equal(t, c.want, got, "%d %d", c.id, c.side)
		}
	}

	ref := tr.Layout(area).Dividers[1].Ref
	require.True(t, tr.Drag(ref, 15, 2, area))
	got, ok := tr.Neighbor(1, Right, area)
	assert.True(t, ok)
	assert.Equal(t, ID(3), got, "largest overlap wins")
}

func TestNeighborTieAndOverlap(t *testing.T) {
	tr := New(1)
	tr.Split(1, Rows, 3, false)
	tr.Split(1, Columns, 2, false)
	area := Rect{0, 0, 21, 9}
	got, ok := tr.Neighbor(3, Up, area)
	assert.True(t, ok)
	assert.Equal(t, ID(1), got, "tie goes to the leftmost")

	g, garea := grid()
	ref := g.Layout(garea).Dividers[2].Ref
	require.True(t, g.Drag(ref, 0, 6, garea))
	got, ok = g.Neighbor(1, Right, garea)
	assert.True(t, ok)
	assert.Equal(t, ID(3), got)
	got, ok = g.Neighbor(2, Right, garea)
	assert.True(t, ok)
	assert.Equal(t, ID(4), got)
	got, ok = g.Neighbor(4, Left, garea)
	assert.True(t, ok)
	assert.Equal(t, ID(2), got)
}

func TestNeighborEmptyPane(t *testing.T) {
	tr := pair(Columns)
	_, ok := tr.Neighbor(2, Left, Rect{0, 0, 2, 4})
	assert.False(t, ok)
	_, ok = tr.Neighbor(1, Right, Rect{0, 0, 2, 4})
	assert.False(t, ok)
}

func width(t *testing.T, tr *Tree, id ID, area Rect) int {
	r, ok := tr.Layout(area).Pane(id)
	require.True(t, ok)
	return r.W
}

func TestResize(t *testing.T) {
	tr := pair(Columns)
	area := Rect{0, 0, 21, 5}
	require.True(t, tr.Resize(1, Right, 3, area))
	assert.Equal(t, 13, width(t, tr, 1, area))
	require.True(t, tr.Resize(2, Left, 3, area))
	assert.Equal(t, 10, width(t, tr, 1, area))
	require.True(t, tr.Resize(1, Left, 2, area), "fallback moves the divider toward the side")
	assert.Equal(t, 8, width(t, tr, 1, area))
	require.True(t, tr.Resize(1, Right, -2, area))
	assert.Equal(t, 6, width(t, tr, 1, area))

	require.True(t, tr.Resize(1, Right, 100, area))
	assert.Equal(t, 19, width(t, tr, 1, area))
	assert.False(t, tr.Resize(1, Right, 1, area), "already at the limit")
	assert.False(t, tr.Resize(1, Right, 0, area))
	assert.False(t, tr.Resize(1, Up, 1, area), "no rows split")
	assert.False(t, tr.Resize(9, Right, 1, area), "unknown pane")
	assert.False(t, tr.Resize(1, Right, 1, Rect{0, 0, 2, 5}), "no room")
	assert.False(t, New(1).Resize(1, Down, 1, area))
}

func TestResizePicksBoundingSplit(t *testing.T) {
	tr := New(1)
	tr.Split(1, Columns, 2, false)
	tr.Split(2, Columns, 3, false)
	area := Rect{0, 0, 21, 5}
	require.True(t, tr.Resize(2, Left, 2, area))
	assert.Equal(t, 8, width(t, tr, 1, area), "the outer divider bounds pane 2 on the left")
	w2 := width(t, tr, 2, area)
	require.True(t, tr.Resize(2, Right, 1, area))
	assert.Equal(t, w2+1, width(t, tr, 2, area))
	assert.Equal(t, 8, width(t, tr, 1, area))

	rows := New(1)
	rows.Split(1, Rows, 2, false)
	rows.Split(2, Columns, 3, false)
	rarea := Rect{0, 0, 10, 11}
	require.True(t, rows.Resize(3, Up, 2, rarea))
	r, _ := rows.Layout(rarea).Pane(1)
	assert.Equal(t, 3, r.H)
	require.True(t, rows.Resize(1, Down, 4, rarea))
	r, _ = rows.Layout(rarea).Pane(1)
	assert.Equal(t, 7, r.H)
}

func TestDrag(t *testing.T) {
	tr := pair(Columns)
	area := Rect{0, 0, 21, 5}
	ref := tr.Layout(area).Dividers[0].Ref
	require.True(t, tr.Drag(ref, 5, 3, area))
	assert.Equal(t, 5, width(t, tr, 1, area))
	assert.Equal(t, Rect{5, 0, 1, 5}, tr.Layout(area).Dividers[0].Rect)
	require.True(t, tr.Drag(ref, -10, 0, area))
	assert.Equal(t, 1, width(t, tr, 1, area))
	require.True(t, tr.Drag(ref, 100, 0, area))
	assert.Equal(t, 19, width(t, tr, 1, area))

	assert.False(t, tr.Drag(Ref{}, 5, 0, area), "zero ref")
	assert.False(t, tr.Drag(ref, 5, 0, Rect{0, 0, 2, 5}), "no room")
	require.True(t, tr.Split(2, Rows, 3, false))
	assert.False(t, tr.Drag(ref, 5, 0, area), "stale ref")
	assert.False(t, New(1).Drag(ref, 5, 0, area), "ref of another tree")

	rows := pair(Rows)
	rarea := Rect{3, 4, 10, 11}
	rref := rows.Layout(rarea).Dividers[0].Ref
	require.True(t, rows.Drag(rref, 0, 6, rarea))
	assert.Equal(t, Rect{3, 6, 10, 1}, rows.Layout(rarea).Dividers[0].Rect)
}
