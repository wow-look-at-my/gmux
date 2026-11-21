// window.go holds what a session does to the panes of its active window.

package mux

import (
	"fmt"

	"github.com/wow-look-at-my/go-tmux/layout"
)

// PaneView is one pane of the window on screen and the cells it owns.
type PaneView struct {
	Pane   *Pane
	Rect   layout.Rect
	Active bool
}

// WindowView is the active window of a session as a client draws it.
type WindowView struct {
	Area     layout.Rect
	Panes    []PaneView
	Dividers []layout.Divider
	Zoomed   bool
}

// View is the session's active window. A session that has ended gives an empty view.
func (m *Mux) View(s *Session) WindowView {
	m.mu.Lock()
	defer m.mu.Unlock()
	w := s.window()
	if w == nil {
		return WindowView{}
	}
	view := WindowView{Area: w.frame.Area, Dividers: w.frame.Dividers, Zoomed: w.zoomed}
	for _, placed := range w.frame.Panes {
		if p := w.panes[placed.ID]; p != nil && !placed.Rect.Empty() {
			view.Panes = append(view.Panes, PaneView{Pane: p, Rect: placed.Rect, Active: placed.ID == w.active})
		}
	}
	return view
}

// ActivePane is the pane of the session that has the focus, or nil once the session has ended.
func (m *Mux) ActivePane(s *Session) *Pane {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w := s.window(); w != nil {
		return w.panes[w.active]
	}
	return nil
}

// Split divides the active pane and starts a program in the new half, which takes the focus.
func (m *Mux) Split(s *Session, dir layout.Dir, spawn Spawn) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	w := s.window()
	if w == nil {
		return fmt.Errorf("the session has ended")
	}
	rect, _ := w.frame.Pane(w.active)
	if (dir == layout.Columns && rect.W < minSplit) || (dir == layout.Rows && rect.H < minSplit) {
		return fmt.Errorf("the pane is too small to split")
	}
	if spawn.Dir == "" {
		spawn.Dir = m.dirFor(s)
	}
	p, err := m.startPane(w, spawn, max(1, rect.W/2), max(1, rect.H/2))
	if err != nil {
		return err
	}
	id := layout.ID(p.ID)
	w.zoomed = false
	w.tree.Split(w.active, dir, id, false)
	w.panes[id] = p
	p.visible.Store(true)
	w.focus(id)
	w.arrange()
	m.emit(Event{Kind: Changed, Session: s})
	return nil
}

// minSplit is the smallest extent that holds a pair of panes and the line between them.
const minSplit = 3

// focus moves the window's focus to a pane.
func (w *Window) focus(id layout.ID) {
	if id == w.active || w.panes[id] == nil {
		return
	}
	w.previous, w.active = w.active, id
}

// change runs fn on the session's active window and tells the watchers. fn reports whether anything changed.
func (m *Mux) change(s *Session, fn func(w *Window) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w := s.window()
	if w == nil || !fn(w) {
		return
	}
	m.emit(Event{Kind: Changed, Session: s})
}

// SelectPane gives a pane of the active window the focus.
func (m *Mux) SelectPane(s *Session, p *Pane) {
	m.change(s, func(w *Window) bool {
		id := layout.ID(p.ID)
		if w.panes[id] != p || id == w.active {
			return false
		}
		w.focus(id)
		if w.zoomed {
			w.arrange()
		}
		return true
	})
}

// FocusSide moves the focus to the pane beside the active one.
func (m *Mux) FocusSide(s *Session, side layout.Side) {
	m.change(s, func(w *Window) bool {
		if w.zoomed {
			return false
		}
		id, ok := w.tree.Neighbor(w.active, side, w.frame.Area)
		if !ok {
			return false
		}
		w.focus(id)
		return true
	})
}

// StepPane moves the focus step panes on, in layout order.
func (m *Mux) StepPane(s *Session, step int) {
	m.change(s, func(w *Window) bool {
		if w.tree.Len() < 2 {
			return false
		}
		w.focus(w.tree.Next(w.active, step))
		if w.zoomed {
			w.arrange()
		}
		return true
	})
}

// LastPane moves the focus back to the pane that had it before.
func (m *Mux) LastPane(s *Session) {
	m.change(s, func(w *Window) bool {
		if w.previous == w.active || w.panes[w.previous] == nil {
			return false
		}
		w.focus(w.previous)
		if w.zoomed {
			w.arrange()
		}
		return true
	})
}

// ToggleZoom gives the active pane the whole window, or gives the window back to all of its panes.
func (m *Mux) ToggleZoom(s *Session) {
	m.change(s, func(w *Window) bool {
		if !w.zoomed && w.tree.Len() < 2 {
			return false
		}
		w.zoomed = !w.zoomed
		w.arrange()
		return true
	})
}

// ResizePane moves the edge of the active pane on the given side by a number of cells.
func (m *Mux) ResizePane(s *Session, side layout.Side, cells int) {
	m.change(s, func(w *Window) bool {
		if w.zoomed || !w.tree.Resize(w.active, side, cells, w.frame.Area) {
			return false
		}
		w.arrange()
		return true
	})
}

// DragDivider moves the line between a pair of panes under the pointer.
func (m *Mux) DragDivider(s *Session, ref layout.Ref, x, y int) {
	m.change(s, func(w *Window) bool {
		if w.zoomed || !w.tree.Drag(ref, x, y, w.frame.Area) {
			return false
		}
		w.arrange()
		return true
	})
}

// Equalize gives every pane of the window the same share.
func (m *Mux) Equalize(s *Session) {
	m.change(s, func(w *Window) bool {
		if w.zoomed {
			return false
		}
		w.tree.Equalize()
		w.arrange()
		return true
	})
}

// SwapPane exchanges the active pane with the step places on. The focus stays
// with the program.
func (m *Mux) SwapPane(s *Session, step int) {
	m.change(s, func(w *Window) bool {
		other := w.tree.Next(w.active, step)
		if w.zoomed || other == w.active || !w.tree.Swap(w.active, other) {
			return false
		}
		w.arrange()
		return true
	})
}

// KillPane ends the program of the active pane. The pane leaves once the program is gone.
func (m *Mux) KillPane(s *Session) {
	if p := m.ActivePane(s); p != nil {
		p.close()
	}
}

// paneExited takes a pane whose program ended out of its window. A window left empty goes too, and so does a
// session left with no window.
func (m *Mux) paneExited(p *Pane) {
	m.mu.Lock()
	w := p.window
	s := w.session
	id := layout.ID(p.ID)
	if w.panes[id] != p {
		m.mu.Unlock()
		return
	}
	delete(w.panes, id)
	ended := false
	if len(w.panes) == 0 {
		ended = m.removeWindow(w)
	} else {
		next := w.previous
		if next == id || w.panes[next] == nil {
			next = w.tree.Next(id, 1)
		}
		w.tree.Remove(id)
		if w.active == id {
			w.active, w.previous = next, next
		}
		if w.previous == id {
			w.previous = w.active
		}
		if w.tree.Len() < 2 {
			w.zoomed = false
		}
		w.arrange()
	}
	m.mu.Unlock()
	if ended {
		m.emit(Event{Kind: Ended, Session: s})
		return
	}
	m.emit(Event{Kind: Changed, Session: s})
}
