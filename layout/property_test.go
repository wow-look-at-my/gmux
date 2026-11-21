package layout

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sides = []Side{Left, Right, Up, Down}

func randArea(rng *rand.Rand) Rect {
	return Rect{rng.Intn(11) - 5, rng.Intn(11) - 5, rng.Intn(44) - 3, rng.Intn(44) - 3}
}

func randPane(rng *rand.Rand, tr *Tree) ID {
	ids := tr.Panes()
	return ids[rng.Intn(len(ids))]
}

// randTree builds a tree by random splits, removals and drags.
func randTree(rng *rand.Rand) *Tree {
	tr := New(0)
	next := ID(1)
	steps := rng.Intn(20)
	for range steps {
		switch k := rng.Intn(6); {
		case k == 0 && tr.Len() > 1:
			tr.Remove(randPane(rng, tr))
		case k == 1:
			big := Rect{0, 0, 200, 200}
			f := tr.Layout(big)
			if len(f.Dividers) > 0 {
				d := f.Dividers[rng.Intn(len(f.Dividers))]
				tr.Drag(d.Ref, rng.Intn(200), rng.Intn(200), big)
			}
		default:
			tr.Split(randPane(rng, tr), Dir(rng.Intn(2)), next, rng.Intn(2) == 0)
			next++
		}
	}
	return tr
}

func checkFrame(t *testing.T, tr *Tree, area Rect) Frame {
	f := tr.Layout(area)
	a := f.Area
	require.GreaterOrEqual(t, a.W, 0)
	require.GreaterOrEqual(t, a.H, 0)
	ids := make([]ID, 0, len(f.Panes))
	for _, p := range f.Panes {
		ids = append(ids, p.ID)
	}
	require.Equal(t, tr.Panes(), ids)
	require.LessOrEqual(t, len(f.Dividers), tr.Len()-1)

	painted := make([]int, a.W*a.H)
	count := 0
	paint := func(r Rect) {
		require.GreaterOrEqual(t, r.W, 0, "%v", r)
		require.GreaterOrEqual(t, r.H, 0, "%v", r)
		if r.Empty() {
			return
		}
		require.GreaterOrEqual(t, r.X, a.X, "%v in %v", r, a)
		require.GreaterOrEqual(t, r.Y, a.Y, "%v in %v", r, a)
		require.LessOrEqual(t, r.X+r.W, a.X+a.W, "%v in %v", r, a)
		require.LessOrEqual(t, r.Y+r.H, a.Y+a.H, "%v in %v", r, a)
		for y := r.Y; y < r.Y+r.H; y++ {
			for x := r.X; x < r.X+r.W; x++ {
				i := (y-a.Y)*a.W + (x - a.X)
				require.Zero(t, painted[i], "overlap at %d,%d", x, y)
				painted[i] = 1
				count++
			}
		}
	}
	for _, p := range f.Panes {
		paint(p.Rect)
	}
	for _, d := range f.Dividers {
		require.False(t, d.Rect.Empty())
		if d.Dir == Columns {
			require.Equal(t, 1, d.Rect.W)
		} else {
			require.Equal(t, 1, d.Rect.H)
		}
		paint(d.Rect)
	}
	if len(f.Dividers) == tr.Len()-1 {
		for _, p := range f.Panes {
			require.False(t, p.Rect.Empty())
		}
		require.Equal(t, a.W*a.H, count, "exact tiling")
	}
	return f
}

func TestPropertyLayout(t *testing.T) {
	for seed := range int64(400) {
		rng := rand.New(rand.NewSource(seed))
		tr := randTree(rng)
		for range 8 {
			checkFrame(t, tr, randArea(rng))
		}
		tr.Equalize()
		checkFrame(t, tr, randArea(rng))
		n := tr.Len()
		for _, id := range tr.Panes() {
			assert.Equal(t, id, tr.Next(id, n))
			assert.Equal(t, id, tr.Next(id, -n))
		}
	}
}

func TestPropertyDragStable(t *testing.T) {
	for seed := range int64(300) {
		rng := rand.New(rand.NewSource(seed))
		tr := randTree(rng)
		area := randArea(rng)
		f := tr.Layout(area)
		for _, d := range f.Dividers {
			g := tr.place(area).splits[d.Ref.n]
			x := area.X + rng.Intn(area.W+6) - 3
			y := area.Y + rng.Intn(area.H+6) - 3
			pos, org, at := x, g.rect.X, 0
			if d.Dir == Rows {
				pos, org = y, g.rect.Y
			}
			if !g.room {
				assert.False(t, tr.Drag(d.Ref, x, y, area))
				continue
			}
			want := org + clampInt(pos-org, 1, span(g.rect, d.Dir)-2)
			require.True(t, tr.Drag(d.Ref, x, y, area))
			found := false
			for _, nd := range tr.Layout(area).Dividers {
				if nd.Ref == d.Ref {
					found = true
					at = nd.Rect.X
					if d.Dir == Rows {
						at = nd.Rect.Y
					}
				}
			}
			require.True(t, found)
			assert.Equal(t, want, at)
		}
		checkFrame(t, tr, area)
	}
}

func TestPropertyResizeReverses(t *testing.T) {
	tested := 0
	for seed := range int64(300) {
		rng := rand.New(rand.NewSource(seed))
		tr := randTree(rng)
		area := randArea(rng)
		for range 10 {
			id := randPane(rng, tr)
			side := sides[rng.Intn(len(sides))]
			cells := rng.Intn(5) + 1
			n := tr.resizeTarget(id, side)
			if n == nil {
				assert.False(t, tr.Resize(id, side, cells, area))
				continue
			}
			g := tr.place(area).splits[n]
			delta := cells
			if side == Left || side == Up {
				delta = -cells
			}
			if !g.room || g.first+delta < 1 || g.first+delta > span(g.rect, n.dir)-2 {
				continue
			}
			before := tr.Layout(area)
			require.True(t, tr.Resize(id, side, cells, area))
			require.NotEqual(t, before, tr.Layout(area))
			require.True(t, tr.Resize(id, side, -cells, area))
			require.Equal(t, before, tr.Layout(area))
			tested++
		}
	}
	assert.Greater(t, tested, 100)
}

func TestPropertyNeighbor(t *testing.T) {
	for seed := range int64(200) {
		rng := rand.New(rand.NewSource(seed))
		tr := randTree(rng)
		area := randArea(rng)
		f := tr.Layout(area)
		for _, id := range tr.Panes() {
			for _, s := range sides {
				got, ok := tr.Neighbor(id, s, area)
				if !ok {
					continue
				}
				assert.NotEqual(t, id, got)
				r, _ := f.Pane(got)
				assert.False(t, r.Empty())
			}
		}
	}
}

func TestPropertyCloneIndependent(t *testing.T) {
	for seed := range int64(200) {
		rng := rand.New(rand.NewSource(seed))
		tr := randTree(rng)
		area := randArea(rng)
		ids := tr.Panes()
		f := tr.Layout(area)
		c := tr.Clone()
		require.Equal(t, f.Panes, c.Layout(area).Panes)
		for i := range 10 {
			switch rng.Intn(4) {
			case 0:
				c.Split(randPane(rng, c), Dir(rng.Intn(2)), ID(1000+i), false)
			case 1:
				c.Remove(randPane(rng, c))
			case 2:
				c.Swap(randPane(rng, c), randPane(rng, c))
			default:
				c.Equalize()
				big := Rect{0, 0, 100, 100}
				for _, d := range c.Layout(big).Dividers {
					c.Drag(d.Ref, rng.Intn(100), rng.Intn(100), big)
				}
			}
		}
		assert.Equal(t, ids, tr.Panes())
		assert.Equal(t, f, tr.Layout(area))
	}
}
