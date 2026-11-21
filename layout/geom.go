// geom.go holds the geometry: layout, focus movement and divider moves.
package layout

import "math"

// splitGeom is where one split landed in a layout pass.
type splitGeom struct {
	rect  Rect
	first int
	room  bool
}

type pass struct {
	f      Frame
	splits map[*node]splitGeom
}

func span(r Rect, d Dir) int {
	if d == Columns {
		return r.W
	}
	return r.H
}

func cross(r Rect, d Dir) int {
	if d == Columns {
		return r.H
	}
	return r.W
}

func origin(r Rect, d Dir) int {
	if d == Columns {
		return r.X
	}
	return r.Y
}

func clampInt(v, lo, hi int) int {
	return max(lo, min(v, hi))
}

// cut splits r along d: the first child gets first cells, then the divider.
func cut(r Rect, d Dir, first int) (a, div, b Rect) {
	if d == Columns {
		return Rect{r.X, r.Y, first, r.H},
			Rect{r.X + first, r.Y, 1, r.H},
			Rect{r.X + first + 1, r.Y, r.W - first - 1, r.H}
	}
	return Rect{r.X, r.Y, r.W, first},
		Rect{r.X, r.Y + first, r.W, 1},
		Rect{r.X, r.Y + first + 1, r.W, r.H - first - 1}
}

// after is the empty rect past the far edge of r along d.
func after(r Rect, d Dir) Rect {
	if d == Columns {
		return Rect{r.X + r.W, r.Y, 0, r.H}
	}
	return Rect{r.X, r.Y + r.H, r.W, 0}
}

func (p *pass) lay(t *Tree, n *node, r Rect) {
	if n.isLeaf() {
		p.f.Panes = append(p.f.Panes, Placed{ID: n.id, Rect: r})
		return
	}
	total := span(r, n.dir)
	if total < 3 || cross(r, n.dir) <= 0 {
		p.splits[n] = splitGeom{rect: r, first: total}
		p.lay(t, n.a, r)
		p.lay(t, n.b, after(r, n.dir))
		return
	}
	avail := total - 1
	first := clampInt(int(math.Round(n.ratio*float64(avail))), 1, avail-1)
	a, div, b := cut(r, n.dir, first)
	p.splits[n] = splitGeom{rect: r, first: first, room: true}
	p.f.Dividers = append(p.f.Dividers, Divider{Rect: div, Dir: n.dir, Ref: Ref{t: t, n: n, gen: t.gen}})
	p.lay(t, n.a, a)
	p.lay(t, n.b, b)
}

func (t *Tree) place(area Rect) pass {
	area.W = max(area.W, 0)
	area.H = max(area.H, 0)
	p := pass{f: Frame{Area: area}, splits: map[*node]splitGeom{}}
	p.lay(t, t.root, area)
	return p
}

// Layout places every pane and divider of the tree in area.
func (t *Tree) Layout(area Rect) Frame { return t.place(area).f }

// setFirst moves the divider of n so the first child gets first cells.
// It returns false when the split has no room or the divider stays put.
func (t *Tree) setFirst(p pass, n *node, first int) bool {
	g := p.splits[n]
	if !g.room {
		return false
	}
	avail := span(g.rect, n.dir) - 1
	first = clampInt(first, 1, avail-1)
	if first == g.first {
		return false
	}
	n.ratio = float64(first) / float64(avail)
	return true
}

// Drag moves the divider of the split ref under the pointer cell.
func (t *Tree) Drag(ref Ref, x, y int, area Rect) bool {
	if ref.t != t || ref.gen != t.gen || ref.n == nil {
		return false
	}
	p := t.place(area)
	g := p.splits[ref.n]
	if !g.room {
		return false
	}
	pos := x
	if ref.n.dir == Rows {
		pos = y
	}
	t.setFirst(p, ref.n, pos-origin(g.rect, ref.n.dir))
	return true
}

func sideDir(s Side) Dir {
	if s == Left || s == Right {
		return Columns
	}
	return Rows
}

// resizeTarget returns the split whose divider Resize moves for id and side.
func (t *Tree) resizeTarget(id ID, side Side) *node {
	n := t.root.find(id)
	if n == nil {
		return nil
	}
	dir := sideDir(side)
	wantFirst := side == Right || side == Down
	var fallback *node
	for c := n; c.parent != nil; c = c.parent {
		p := c.parent
		if p.dir != dir {
			continue
		}
		if fallback == nil {
			fallback = p
		}
		if (p.a == c) == wantFirst {
			return p
		}
	}
	return fallback
}

// Resize moves the divider that bounds pane id on side by cells toward side.
func (t *Tree) Resize(id ID, side Side, cells int, area Rect) bool {
	n := t.resizeTarget(id, side)
	if n == nil {
		return false
	}
	if side == Left || side == Up {
		cells = -cells
	}
	p := t.place(area)
	return t.setFirst(p, n, p.splits[n].first+cells)
}

// adjacent reports whether q starts past the divider beyond r's edge on side.
func adjacent(r, q Rect, side Side) bool {
	switch side {
	case Left:
		return q.X+q.W+1 == r.X
	case Right:
		return q.X == r.X+r.W+1
	case Up:
		return q.Y+q.H+1 == r.Y
	default:
		return q.Y == r.Y+r.H+1
	}
}

// overlap returns how many cells r and q share on the axis across side.
func overlap(r, q Rect, side Side) int {
	if sideDir(side) == Columns {
		return min(r.Y+r.H, q.Y+q.H) - max(r.Y, q.Y)
	}
	return min(r.X+r.W, q.X+q.W) - max(r.X, q.X)
}

// Neighbor returns the pane that focus moves to from id toward side.
func (t *Tree) Neighbor(id ID, side Side, area Rect) (ID, bool) {
	f := t.Layout(area)
	r, ok := f.Pane(id)
	if !ok || r.Empty() {
		return 0, false
	}
	var best Placed
	bestOv := 0
	for _, q := range f.Panes {
		if q.ID == id || q.Rect.Empty() || !adjacent(r, q.Rect, side) {
			continue
		}
		ov := overlap(r, q.Rect, side)
		if ov <= 0 {
			continue
		}
		key, bestKey := q.Rect.Y, best.Rect.Y
		if sideDir(side) == Rows {
			key, bestKey = q.Rect.X, best.Rect.X
		}
		if ov > bestOv || (ov == bestOv && key < bestKey) {
			best, bestOv = q, ov
		}
	}
	return best.ID, bestOv > 0
}
