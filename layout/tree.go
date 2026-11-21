// tree.go holds the split tree and its structural edits.
package layout

// node is a leaf when a is nil. A split always has both children.
type node struct {
	parent *node
	id     ID
	dir    Dir
	ratio  float64
	a, b   *node
}

func (n *node) isLeaf() bool { return n.a == nil }

func (n *node) leaves(out []ID) []ID {
	if n.isLeaf() {
		return append(out, n.id)
	}
	return n.b.leaves(n.a.leaves(out))
}

func (n *node) find(id ID) *node {
	if n.isLeaf() {
		if n.id == id {
			return n
		}
		return nil
	}
	if f := n.a.find(id); f != nil {
		return f
	}
	return n.b.find(id)
}

func (n *node) clone(parent *node) *node {
	c := &node{parent: parent, id: n.id, dir: n.dir, ratio: n.ratio}
	if !n.isLeaf() {
		c.a = n.a.clone(c)
		c.b = n.b.clone(c)
	}
	return c
}

// Tree is the split layout of one window. It always holds at least one pane.
type Tree struct {
	root *node
	// gen changes on every structural edit, which voids older Refs.
	gen uint64
}

// New returns a tree of the pane first.
func New(first ID) *Tree {
	return &Tree{root: &node{id: first}}
}

// Len returns the number of panes.
func (t *Tree) Len() int { return len(t.Panes()) }

// Panes returns the pane IDs in tree order: depth first, first child first.
func (t *Tree) Panes() []ID { return t.root.leaves(nil) }

// Has reports whether the pane id is in the tree.
func (t *Tree) Has(id ID) bool { return t.root.find(id) != nil }

// replace puts n where old was.
func (t *Tree) replace(old, n *node) {
	p := old.parent
	n.parent = p
	switch {
	case p == nil:
		t.root = n
	case p.a == old:
		p.a = n
	default:
		p.b = n
	}
}

// Split replaces the leaf target with a split of direction dir that holds
// target and a new leaf added. added comes second unless before is true.
func (t *Tree) Split(target ID, dir Dir, added ID, before bool) bool {
	leaf := t.root.find(target)
	if leaf == nil || t.Has(added) {
		return false
	}
	s := &node{dir: dir, ratio: 0.5}
	t.replace(leaf, s)
	fresh := &node{id: added, parent: s}
	leaf.parent = s
	if before {
		s.a, s.b = fresh, leaf
	} else {
		s.a, s.b = leaf, fresh
	}
	t.gen++
	return true
}

// Remove deletes the leaf id. Its sibling takes the parent's place.
// The last pane cannot be removed.
func (t *Tree) Remove(id ID) bool {
	leaf := t.root.find(id)
	if leaf == nil || leaf.parent == nil {
		return false
	}
	p := leaf.parent
	sib := p.a
	if sib == leaf {
		sib = p.b
	}
	t.replace(p, sib)
	t.gen++
	return true
}

// Swap exchanges the positions of the panes a and b.
func (t *Tree) Swap(a, b ID) bool {
	na, nb := t.root.find(a), t.root.find(b)
	if na == nil || nb == nil {
		return false
	}
	na.id, nb.id = nb.id, na.id
	return true
}

// weight counts the leaves reachable from n through splits of direction d.
func weight(n *node, d Dir) int {
	if n.isLeaf() || n.dir != d {
		return 1
	}
	return weight(n.a, d) + weight(n.b, d)
}

func equalize(n *node) {
	if n.isLeaf() {
		return
	}
	wa, wb := weight(n.a, n.dir), weight(n.b, n.dir)
	n.ratio = float64(wa) / float64(wa+wb)
	equalize(n.a)
	equalize(n.b)
}

// Equalize gives every pane under a run of same-direction splits equal space.
func (t *Tree) Equalize() { equalize(t.root) }

// Next returns the pane step positions after id in tree order, with
// wrap-around. An unknown id gives the first pane.
func (t *Tree) Next(id ID, step int) ID {
	ids := t.Panes()
	n := len(ids)
	for i, v := range ids {
		if v == id {
			return ids[((i+step%n)%n+n)%n]
		}
	}
	return ids[0]
}

// Clone returns a deep copy. Refs of t name nothing in the copy.
func (t *Tree) Clone() *Tree {
	return &Tree{root: t.root.clone(nil), gen: t.gen}
}
