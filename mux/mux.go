// Package mux is the multiplexer's model: the sessions, the windows in them, and the panes that run programs.
//
// Mux.mu guards the structure. Each Pane guards its own screen. A goroutine takes Mux.mu before a pane's lock and
// never the other way round.
package mux

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/wow-look-at-my/go-tmux/layout"
)

// Config is what every pane the multiplexer starts shares.
type Config struct {
	// Shell is the program a pane runs when no command is given.
	Shell string
	// Scrollback is how many lines of history a pane keeps.
	Scrollback int
	// Term is the TERM a pane's program sees.
	Term string
	// Socket is the server's address, exported to a pane's program as GMUX.
	Socket string
}

// DefaultTerm is the terminal type a pane's program is told it runs on.
const DefaultTerm = "xterm-256color"

// Default sizes of a session nobody is attached to.
const (
	DefaultCols = 80
	DefaultRows = 24
)

// EventKind says what changed.
type EventKind int

const (
	// Dirty is a visible pane drawing something new.
	Dirty EventKind = iota
	// Changed is the structure moving: a window, a pane, a name, the focus.
	Changed
	// Ended is a session losing its last window.
	Ended
	// Clipboard is a program in a pane setting the clipboard. Text carries what it set.
	Clipboard
)

// Event is one change, sent to every watcher.
type Event struct {
	Kind    EventKind
	Session *Session
	Text    string
}

// Mux holds every session of one server.
type Mux struct {
	cfg Config

	mu       sync.Mutex
	sessions []*Session
	nextID   struct{ session, window, pane int }

	watchMu  sync.RWMutex
	watchers map[int]func(Event)
	watchID  int
}

// New returns an empty multiplexer.
func New(cfg Config) *Mux {
	if cfg.Shell == "" {
		cfg.Shell = defaultShell()
	}
	if cfg.Term == "" {
		cfg.Term = DefaultTerm
	}
	return &Mux{cfg: cfg, watchers: map[int]func(Event){}}
}

// defaultShell is the user's shell, or the one every host has.
func defaultShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/sh"
}

// Watch calls fn for every event, from whatever goroutine caused it. fn must not block and must not call back into
// the multiplexer. The function Watch returns stops the calls.
func (m *Mux) Watch(fn func(Event)) (stop func()) {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()
	m.watchID++
	id := m.watchID
	m.watchers[id] = fn
	return func() {
		m.watchMu.Lock()
		defer m.watchMu.Unlock()
		delete(m.watchers, id)
	}
}

func (m *Mux) emit(event Event) {
	m.watchMu.RLock()
	defer m.watchMu.RUnlock()
	for _, fn := range m.watchers {
		fn(event)
	}
}

// Spawn says what a new pane runs and where.
type Spawn struct {
	// Command is the program and its arguments. Empty runs the shell.
	Command []string
	// Dir is the working directory. Empty takes the active pane's, or the session's.
	Dir string
}

// SessionOptions describe a session to create.
type SessionOptions struct {
	// Name is the session's name. Empty picks the next free number.
	Name string
	// Env is the environment its panes start with.
	Env []string
	// Cols and Rows are the size of the window area before a client attaches.
	Cols, Rows int
	Spawn
}

// NewSession creates a session with one window and one pane.
func (m *Mux) NewSession(opts SessionOptions) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := opts.Name
	if name == "" {
		name = m.freeName()
	} else if m.find(name) != nil {
		return nil, fmt.Errorf("a session named %q already exists", name)
	}
	cols, rows := opts.Cols, opts.Rows
	if cols <= 0 || rows <= 0 {
		cols, rows = DefaultCols, DefaultRows
	}
	dir := opts.Dir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	m.nextID.session++
	s := &Session{
		ID: m.nextID.session, Name: name, Created: time.Now(),
		env: opts.Env, dir: dir, cols: cols, rows: rows,
		sizes: map[int]size{}, last: -1,
	}
	if _, err := m.addWindow(s, opts.Spawn); err != nil {
		return nil, err
	}
	m.sessions = append(m.sessions, s)
	m.emit(Event{Kind: Changed, Session: s})
	return s, nil
}

// freeName is the lowest number no session is named. The caller holds the lock.
func (m *Mux) freeName() string {
	for n := 0; ; n++ {
		name := strconv.Itoa(n)
		if m.find(name) == nil {
			return name
		}
	}
}

// find is the session with the given name. The caller holds the lock.
func (m *Mux) find(name string) *Session {
	for _, s := range m.sessions {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// Find is the session a target names: an exact name first, then a unique prefix. An empty target is the session
// used most recently.
func (m *Mux) Find(target string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sessions) == 0 {
		return nil, fmt.Errorf("no sessions")
	}
	if target == "" {
		recent := m.sessions[0]
		for _, s := range m.sessions[1:] {
			if s.used.After(recent.used) {
				recent = s
			}
		}
		return recent, nil
	}
	if s := m.find(target); s != nil {
		return s, nil
	}
	var matches []*Session
	for _, s := range m.sessions {
		if len(s.Name) >= len(target) && s.Name[:len(target)] == target {
			matches = append(matches, s)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no session named %q", target)
	case 1:
		return matches[0], nil
	}
	return nil, fmt.Errorf("%q names more than one session", target)
}

// Empty reports whether no session is left.
func (m *Mux) Empty() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions) == 0
}

// List describes every session, oldest first.
func (m *Mux) List() []SessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	infos := make([]SessionInfo, 0, len(m.sessions))
	for _, s := range m.sessions {
		infos = append(infos, s.info())
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].ID < infos[j].ID })
	return infos
}

// Info describes one session. ok is false once the session has ended.
func (m *Mux) Info(s *Session) (info SessionInfo, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.alive(s) {
		return SessionInfo{}, false
	}
	return s.info(), true
}

// alive reports whether the session is still one of the multiplexer's. The caller holds the lock.
func (m *Mux) alive(s *Session) bool {
	for _, other := range m.sessions {
		if other == s {
			return true
		}
	}
	return false
}

// RenameSession gives a session a new name.
func (m *Mux) RenameSession(s *Session, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if name == "" {
		return fmt.Errorf("a session name cannot be empty")
	}
	if other := m.find(name); other != nil && other != s {
		return fmt.Errorf("a session named %q already exists", name)
	}
	s.Name = name
	m.emit(Event{Kind: Changed, Session: s})
	return nil
}

// KillSession ends every program of a session and removes it.
func (m *Mux) KillSession(s *Session) {
	m.mu.Lock()
	var panes []*Pane
	for _, w := range s.windows {
		for _, p := range w.panes {
			panes = append(panes, p)
		}
	}
	m.dropSession(s)
	m.mu.Unlock()
	for _, p := range panes {
		p.close()
	}
	m.emit(Event{Kind: Ended, Session: s})
}

// dropSession removes a session from the list. The caller holds the lock.
func (m *Mux) dropSession(s *Session) {
	for i, other := range m.sessions {
		if other == s {
			m.sessions = append(m.sessions[:i], m.sessions[i+1:]...)
			return
		}
	}
}

// Close ends every session.
func (m *Mux) Close() {
	m.mu.Lock()
	sessions := append([]*Session(nil), m.sessions...)
	m.mu.Unlock()
	for _, s := range sessions {
		m.KillSession(s)
	}
}

// Touch marks a session as the one used now, which is what an attach with no target picks.
func (m *Mux) Touch(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.used = time.Now()
}

// Attach records a client's window area and resizes the session to the smallest area any client shows.
func (m *Mux) Attach(s *Session, client, cols, rows int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.sizes[client] = size{cols: cols, rows: rows}
	s.used = time.Now()
	m.fit(s)
}

// Detach forgets a client's window area.
func (m *Mux) Detach(s *Session, client int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(s.sizes, client)
	m.fit(s)
}

// fit sizes the session to the smallest client. A session with no client keeps its size. The caller holds the lock.
func (m *Mux) fit(s *Session) {
	cols, rows := 0, 0
	for _, sz := range s.sizes {
		if cols == 0 || sz.cols < cols {
			cols = sz.cols
		}
		if rows == 0 || sz.rows < rows {
			rows = sz.rows
		}
	}
	if cols <= 0 || rows <= 0 || (cols == s.cols && rows == s.rows) {
		return
	}
	s.cols, s.rows = cols, rows
	for _, w := range s.windows {
		w.arrange()
	}
	m.emit(Event{Kind: Changed, Session: s})
}

// area is the cells a session's windows lay their panes out in.
func (s *Session) area() layout.Rect { return layout.Rect{W: s.cols, H: s.rows} }
