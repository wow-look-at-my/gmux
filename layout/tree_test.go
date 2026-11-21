package layout

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tr := New(7)
	assert.Equal(t, 1, tr.Len())
	assert.Equal(t, []ID{7}, tr.Panes())
	assert.True(t, tr.Has(7))
	assert.False(t, tr.Has(8))
}

func TestSplit(t *testing.T) {
	tr := New(1)
	require.True(t, tr.Split(1, Columns, 2, false))
	assert.Equal(t, []ID{1, 2}, tr.Panes())
	require.True(t, tr.Split(1, Rows, 3, true))
	assert.Equal(t, []ID{3, 1, 2}, tr.Panes())
	require.True(t, tr.Split(2, Columns, 4, false))
	assert.Equal(t, []ID{3, 1, 2, 4}, tr.Panes())
	assert.Equal(t, 4, tr.Len())

	assert.False(t, tr.Split(9, Columns, 5, false), "unknown target")
	assert.False(t, tr.Split(1, Columns, 2, false), "added already present")
	assert.False(t, tr.Split(1, Columns, 1, false), "added equals target")
	assert.Equal(t, []ID{3, 1, 2, 4}, tr.Panes())
}

func TestRemove(t *testing.T) {
	tr := New(1)
	assert.False(t, tr.Remove(1), "last pane")
	assert.False(t, tr.Remove(2), "unknown pane")
	require.True(t, tr.Split(1, Columns, 2, false))
	require.True(t, tr.Split(2, Rows, 3, false))
	require.True(t, tr.Remove(2))
	assert.Equal(t, []ID{1, 3}, tr.Panes())
	require.True(t, tr.Remove(1))
	assert.Equal(t, []ID{3}, tr.Panes())
	assert.False(t, tr.Remove(3))

	f := tr.Layout(Rect{0, 0, 10, 5})
	assert.Equal(t, []Placed{{3, Rect{0, 0, 10, 5}}}, f.Panes)
	assert.Empty(t, f.Dividers)
}

func TestRemoveSiblingTakesSpace(t *testing.T) {
	tr := New(1)
	tr.Split(1, Columns, 2, false)
	tr.Split(2, Rows, 3, false)
	tr.Remove(3)
	f := tr.Layout(Rect{0, 0, 21, 6})
	r, ok := f.Pane(2)
	require.True(t, ok)
	assert.Equal(t, Rect{11, 0, 10, 6}, r)
}

func TestSwap(t *testing.T) {
	tr := New(1)
	tr.Split(1, Columns, 2, false)
	tr.Split(2, Rows, 3, false)
	assert.False(t, tr.Swap(1, 9))
	assert.False(t, tr.Swap(9, 1))
	assert.True(t, tr.Swap(2, 2))
	assert.Equal(t, []ID{1, 2, 3}, tr.Panes())
	before := tr.Layout(Rect{0, 0, 21, 9})
	require.True(t, tr.Swap(1, 3))
	assert.Equal(t, []ID{3, 2, 1}, tr.Panes())
	after := tr.Layout(Rect{0, 0, 21, 9})
	r1, _ := before.Pane(1)
	r3, _ := after.Pane(3)
	assert.Equal(t, r1, r3)
}

func TestNext(t *testing.T) {
	tr := New(1)
	tr.Split(1, Columns, 2, false)
	tr.Split(2, Columns, 3, false)
	assert.Equal(t, ID(2), tr.Next(1, 1))
	assert.Equal(t, ID(1), tr.Next(3, 1))
	assert.Equal(t, ID(3), tr.Next(1, -1))
	assert.Equal(t, ID(1), tr.Next(1, 3))
	assert.Equal(t, ID(2), tr.Next(1, -5))
	assert.Equal(t, ID(1), tr.Next(2, -7))
	assert.Equal(t, ID(1), tr.Next(99, 1), "unknown id gives the first pane")
	assert.Equal(t, ID(1), New(1).Next(1, 5))
}

func TestEqualize(t *testing.T) {
	tr := New(1)
	tr.Split(1, Columns, 2, false)
	tr.Split(2, Columns, 3, false)
	tr.Split(3, Rows, 4, false)
	tr.Equalize()
	f := tr.Layout(Rect{0, 0, 32, 10})
	for _, id := range []ID{1, 2, 3} {
		r, ok := f.Pane(id)
		require.True(t, ok)
		assert.Equal(t, 10, r.W, "pane %d", id)
	}
	r4, _ := f.Pane(4)
	assert.Equal(t, 10, r4.W)

	// Weights only follow same-direction splits.
	tr2 := New(1)
	tr2.Split(1, Rows, 2, false)
	tr2.Split(2, Rows, 3, false)
	tr2.Split(3, Columns, 4, false)
	tr2.Equalize()
	f2 := tr2.Layout(Rect{0, 0, 10, 32})
	for _, id := range []ID{1, 2, 3, 4} {
		r, _ := f2.Pane(id)
		assert.Equal(t, 10, r.H, "pane %d", id)
	}
}

func TestClone(t *testing.T) {
	tr := New(1)
	tr.Split(1, Columns, 2, false)
	area := Rect{0, 0, 30, 10}
	ref := tr.Layout(area).Dividers[0].Ref
	c := tr.Clone()
	assert.Equal(t, tr.Panes(), c.Panes())
	assert.Equal(t, tr.Layout(area).Panes, c.Layout(area).Panes)

	assert.False(t, c.Drag(ref, 5, 0, area), "a ref of the original names nothing in the clone")
	require.True(t, c.Split(2, Rows, 3, false))
	require.True(t, c.Drag(c.Layout(area).Dividers[0].Ref, 5, 0, area))
	assert.Equal(t, []ID{1, 2}, tr.Panes())
	assert.Equal(t, []ID{1, 2, 3}, c.Panes())
	r, _ := tr.Layout(area).Pane(1)
	assert.Equal(t, 15, r.W)
	require.True(t, tr.Swap(1, 2))
	assert.Equal(t, []ID{1, 2, 3}, c.Panes())
}
