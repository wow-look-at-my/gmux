// session.go holds the session and its windows.

package mux

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/wow-look-at-my/go-tmux/layout"
)

type size struct{ cols, rows int }

// Session is a set of windows a client attaches to.
type Session struct {
	ID      int
	Name    string
	Created time.Time

	windows []*Window
	// active and last index windows. last is the window shown before the active one, or negative.
	active, last int
	env          []string
	dir          string
	cols, rows   int
	sizes        map[int]size
	used         time.Time
	// gutter is the columns each pane keeps free at its right edge.
	gutter int
}

// Window is one screenful of panes.
type Window struct {
	ID   int
	Name string
	// named is set once the user names the window, which stops it following the running program.
	named   bool
	session *Session

	tree   *layout.Tree
	panes  map[layout.ID]*Pane
	active layout.ID
	// previous is the pane that had the focus before the active one.
	previous layout.ID
	zoomed   bool
	frame    layout.Frame

	// activity is set by output in a window nobody looks at.
	activity atomic.Bool
	bell     atomic.Bool
}

// WindowInfo describes a window for a status bar or a listing.
type WindowInfo struct {
	ID       int
	Index    int
	Name     string
	Active   bool
	Activity bool
	Bell     bool
	Zoomed   bool
	Panes    int
}

// SessionInfo describes a session for a status bar or a listing.
type SessionInfo struct {
	ID       int
	Name     string
	Created  time.Time
	Windows  []WindowInfo
	Attached int
	Cols     int
	Rows     int
}

// info describes the session. The caller holds the lock.
func (s *Session) info() SessionInfo {
	info := SessionInfo{
		ID: s.ID, Name: s.Name, Created: s.Created, Attached: len(s.sizes), Cols: s.cols, Rows: s.rows,
	}
	for i, w := range s.windows {
		info.Windows = append(info.Windows, WindowInfo{
			ID: w.ID, Index: i, Name: w.Name, Active: i == s.active,
			Activity: w.activity.Load(), Bell: w.bell.Load(), Zoomed: w.zoomed, Panes: len(w.panes),
		})
	}
	return info
}

// window is the session's active window. The caller holds the lock.
func (s *Session) window() *Window {
	if s.active < 0 || s.active >= len(s.windows) {
		return nil
	}
	return s.windows[s.active]
}

// addWindow starts a pane in a new window at the end of the session. The caller holds the lock.
func (m *Mux) addWindow(s *Session, spawn Spawn) (*Window, error) {
	m.nextID.window++
	w := &Window{ID: m.nextID.window, session: s, panes: map[layout.ID]*Pane{}}
	if spawn.Dir == "" {
		spawn.Dir = m.dirFor(s)
	}
	p, err := m.startPane(w, spawn, s.cols, s.rows)
	if err != nil {
		return nil, err
	}
	id := layout.ID(p.ID)
	w.tree = layout.New(id)
	w.panes[id] = p
	w.active, w.previous = id, id
	w.Name = p.commandName()
	s.windows = append(s.windows, w)
	m.show(s, len(s.windows)-1)
	return w, nil
}

// dirFor is the directory a new pane of the session starts in: where the active pane's program is, else where the
// session started. The caller holds the lock.
func (m *Mux) dirFor(s *Session) string {
	if w := s.window(); w != nil {
		if p := w.panes[w.active]; p != nil {
			if dir := p.Dir(); dir != "" {
				return dir
			}
		}
	}
	return s.dir
}

// show makes the window at index the session's active one. The caller holds the lock.
func (m *Mux) show(s *Session, index int) {
	if index < 0 || index >= len(s.windows) {
		return
	}
	if index != s.active && s.active >= 0 && s.active < len(s.windows) {
		s.last = s.active
	}
	s.active = index
	for i, w := range s.windows {
		w.setVisible(i == index)
	}
	w := s.windows[index]
	w.activity.Store(false)
	w.bell.Store(false)
	w.arrange()
}

// NewWindow adds a window to a session and shows it.
func (m *Mux) NewWindow(s *Session, spawn Spawn) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.alive(s) {
		return fmt.Errorf("the session has ended")
	}
	if _, err := m.addWindow(s, spawn); err != nil {
		return err
	}
	m.emit(Event{Kind: Changed, Session: s})
	return nil
}

// SelectWindow shows the window at index.
func (m *Mux) SelectWindow(s *Session, index int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if index < 0 || index >= len(s.windows) || index == s.active {
		return
	}
	m.show(s, index)
	m.emit(Event{Kind: Changed, Session: s})
}

// StepWindow shows the window step places after the active one, wrapping at both ends.
func (m *Mux) StepWindow(s *Session, step int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(s.windows)
	if n < 2 {
		return
	}
	m.show(s, ((s.active+step)%n+n)%n)
	m.emit(Event{Kind: Changed, Session: s})
}

// LastWindow shows the window that was active before this.
func (m *Mux) LastWindow(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.last < 0 || s.last >= len(s.windows) || s.last == s.active {
		return
	}
	m.show(s, s.last)
	m.emit(Event{Kind: Changed, Session: s})
}

// MoveWindow moves the window at index from to index to, and the windows between shift to make room.
func (m *Mux) MoveWindow(s *Session, from, to int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(s.windows)
	if from < 0 || from >= n || to < 0 || to >= n || from == to {
		return
	}
	active, last := s.window(), (*Window)(nil)
	if s.last >= 0 && s.last < n {
		last = s.windows[s.last]
	}
	w := s.windows[from]
	s.windows = append(s.windows[:from], s.windows[from+1:]...)
	s.windows = append(s.windows[:to], append([]*Window{w}, s.windows[to:]...)...)
	s.active, s.last = s.indexOf(active), s.indexOf(last)
	m.emit(Event{Kind: Changed, Session: s})
}

// indexOf is where a window sits in the session, or negative. The caller holds the lock.
func (s *Session) indexOf(w *Window) int {
	for i, other := range s.windows {
		if other == w {
			return i
		}
	}
	return -1
}

// RenameWindow names the window at index. An empty name lets it follow the running program again.
func (m *Mux) RenameWindow(s *Session, index int, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if index < 0 || index >= len(s.windows) {
		return
	}
	w := s.windows[index]
	w.named = name != ""
	if w.named {
		w.Name = name
	} else if p := w.panes[w.active]; p != nil {
		w.Name = p.commandName()
	}
	m.emit(Event{Kind: Changed, Session: s})
}

// KillWindow ends every program of the window at index.
func (m *Mux) KillWindow(s *Session, index int) {
	m.mu.Lock()
	if index < 0 || index >= len(s.windows) {
		m.mu.Unlock()
		return
	}
	var panes []*Pane
	for _, p := range s.windows[index].panes {
		panes = append(panes, p)
	}
	m.mu.Unlock()
	// Each pane leaves the window as its program ends, and the last one takes the window with it.
	for _, p := range panes {
		p.close()
	}
}

// removeWindow takes a window with no panes out of its session. It reports whether the session ended with it. The
// caller holds the lock.
func (m *Mux) removeWindow(w *Window) (ended bool) {
	s := w.session
	index := s.indexOf(w)
	if index < 0 {
		return false
	}
	active, last := s.window(), (*Window)(nil)
	if s.last >= 0 && s.last < len(s.windows) {
		last = s.windows[s.last]
	}
	s.windows = append(s.windows[:index], s.windows[index+1:]...)
	if len(s.windows) == 0 {
		m.dropSession(s)
		return true
	}
	s.last = s.indexOf(last)
	if active != w {
		s.active = s.indexOf(active)
		return false
	}
	// The window on screen went: the shown before it comes back, else its neighbour.
	next := s.last
	if next < 0 {
		next = min(index, len(s.windows)-1)
	}
	s.active, s.last = -1, -1
	m.show(s, next)
	return false
}

// setVisible tells the window's panes whether a client can see them.
func (w *Window) setVisible(visible bool) {
	for _, p := range w.panes {
		p.visible.Store(visible)
	}
}

// arrange lays the panes out in the session's area and sizes each pane's terminal to its place. The caller holds
// the lock.
func (w *Window) arrange() {
	area := w.session.area()
	if w.zoomed {
		w.frame = layout.Frame{Area: area, Panes: []layout.Placed{{ID: w.active, Rect: area}}}
	} else {
		w.frame = w.tree.Layout(area)
	}
	for _, placed := range w.frame.Panes {
		if p := w.panes[placed.ID]; p != nil && !placed.Rect.Empty() {
			p.resize(max(1, placed.Rect.W-w.session.gutter), placed.Rect.H)
		}
	}
}
