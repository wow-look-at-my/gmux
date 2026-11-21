// hit.go holds the element that makes a piece of a bar answer the pointer.

package ui

import (
	"github.com/wow-look-at-my/tml/cell"
	"github.com/wow-look-at-my/tml/sema"
)

// hit is the element behind <Hit>. It draws nothing and keeps no room for itself: it wraps what is written inside it.
type hit struct{}

// Measure is only reached by an empty element, which takes no room.
func (hit) Measure(_, _ int) (int, int) { return 0, 0 }

func (hit) Draw(cell.Canvas) {}

func (hit) Insets() sema.Thickness { return sema.Thickness{} }
