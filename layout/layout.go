// Package layout holds the split-pane layout of one window. A binary tree of
// splits whose leaves are panes, and the geometry that goes with it.
package layout

// ID names a pane. The caller assigns it.
type ID int

// Dir is how a split arranges its children.
type Dir uint8

const (
	// Columns puts the children side by side, with a vertical divider line between them.
	Columns Dir = iota
	// Rows stacks the children, with a horizontal divider line between them.
	Rows
)

// Side is a direction on screen.
type Side uint8

const (
	Left Side = iota
	Right
	Up
	Down
)

// Rect is a block of cells. W or H at or below zero makes it empty.
type Rect struct{ X, Y, W, H int }

// Contains reports whether the cell lies inside the rect.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// Empty reports whether the rect covers no cells.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Placed is a pane and the cells it owns.
type Placed struct {
	ID   ID
	Rect Rect
}

// Divider is the one-cell-thick line between the children of one split.
type Divider struct {
	// Rect is the cells the line covers: W == 1 for a Columns split, H == 1 for a Rows split.
	Rect Rect
	Dir  Dir
	// Ref names the split for Drag. It stays valid until the tree's structure changes.
	Ref Ref
}

// Ref names one split of a tree. The zero Ref names nothing.
type Ref struct {
	t   *Tree
	n   *node
	gen uint64
}

// Frame is one layout of a tree in an area.
type Frame struct {
	Area     Rect
	Panes    []Placed
	Dividers []Divider
}

// Pane returns the rect of the pane id.
func (f Frame) Pane(id ID) (Rect, bool) {
	for _, p := range f.Panes {
		if p.ID == id {
			return p.Rect, true
		}
	}
	return Rect{}, false
}

// PaneAt returns the pane whose rect holds the cell.
func (f Frame) PaneAt(x, y int) (ID, bool) {
	for _, p := range f.Panes {
		if p.Rect.Contains(x, y) {
			return p.ID, true
		}
	}
	return 0, false
}

// DividerAt returns the divider whose rect holds the cell.
func (f Frame) DividerAt(x, y int) (Divider, bool) {
	for _, d := range f.Dividers {
		if d.Rect.Contains(x, y) {
			return d, true
		}
	}
	return Divider{}, false
}
