// pane.go holds the pane: one program on a terminal, and the screen it draws.

package mux

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wow-look-at-my/go-tmux/keys"
	"github.com/wow-look-at-my/go-tmux/pty"
	"github.com/wow-look-at-my/go-tmux/vt"
	"github.com/wow-look-at-my/tml/input"
)

// syncLimit is how long a program may hold synchronized output open before its frame is shown anyway.
const syncLimit = 200 * time.Millisecond

// drainGrace is how long output still in the terminal is read after the program ends.
const drainGrace = 50 * time.Millisecond

// Pane is one program on a terminal.
type Pane struct {
	ID     int
	mux    *Mux
	window *Window
	pty    *pty.Pty

	// visible is set while the pane's window is the one its session shows.
	visible atomic.Bool

	mu         sync.Mutex
	screen     *vt.Screen
	cols, rows int
	// offset is how many lines back in history the view starts. Zero is the live screen.
	offset    int
	sel       selection
	process   pty.Process
	command   string
	startDir  string
	syncSince time.Time

	inMu     sync.Mutex
	in       []byte
	inSignal chan struct{}
}

// startPane starts a program on a new terminal of the given size. The caller holds the multiplexer's lock.
func (m *Mux) startPane(w *Window, spawn Spawn, cols, rows int) (*Pane, error) {
	m.nextID.pane++
	p := &Pane{
		ID: m.nextID.pane, mux: m, window: w, cols: cols, rows: rows,
		startDir: spawn.Dir, inSignal: make(chan struct{}, 1),
	}
	cmd := pty.Command{Env: m.environ(w.session, p), Dir: spawn.Dir, Cols: cols, Rows: rows}
	if len(spawn.Command) > 0 {
		cmd.Path, cmd.Args = spawn.Command[0], spawn.Command[1:]
		p.command = filepath.Base(cmd.Path)
	} else {
		// A leading dash in the name is how a shell is told it is a login shell.
		cmd.Path = m.cfg.Shell
		p.command = filepath.Base(cmd.Path)
		cmd.Argv0 = "-" + p.command
	}
	if cmd.Dir != "" {
		if info, err := os.Stat(cmd.Dir); err != nil || !info.IsDir() {
			cmd.Dir = ""
		}
	}
	p.screen = vt.New(cols, rows, vt.Options{Scrollback: m.cfg.Scrollback})
	p.screen.OnReply(p.Input)
	p.screen.OnBell(p.bell)
	p.screen.OnTitle(func(string) { m.emit(Event{Kind: Changed, Session: w.session}) })
	p.screen.OnClipboard(func(data []byte) {
		m.emit(Event{Kind: Clipboard, Session: w.session, Text: string(data)})
	})
	started, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	p.pty = started
	go p.read()
	go p.write()
	return p, nil
}

// environ is the environment of a pane's program: the session's, with the terminal's own variables set.
func (m *Mux) environ(s *Session, p *Pane) []string {
	set := map[string]string{
		"TERM":      m.cfg.Term,
		"COLORTERM": "truecolor",
		"GMUX":      m.cfg.Socket,
		"GMUX_PANE": strconv.Itoa(p.ID),
	}
	env := make([]string, 0, len(s.env)+len(set))
	for _, entry := range s.env {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := set[name]; !replaced {
			env = append(env, entry)
		}
	}
	for _, name := range []string{"TERM", "COLORTERM", "GMUX", "GMUX_PANE"} {
		env = append(env, name+"="+set[name])
	}
	return env
}

// read feeds the program's output to the screen until the terminal closes, then removes the pane.
func (p *Pane) read() {
	go func() {
		<-p.pty.Done()
		// Output written before the exit is still in the terminal. Closing ends the read below.
		time.Sleep(drainGrace)
		p.pty.Close()
	}()
	buf := make([]byte, 64<<10)
	for {
		n, err := p.pty.Read(buf)
		if n > 0 {
			p.mu.Lock()
			p.screen.Write(buf[:n])
			held := p.holdsFrame()
			p.mu.Unlock()
			if !held {
				p.mux.output(p)
			}
		}
		if err != nil {
			break
		}
	}
	<-p.pty.Done()
	p.pty.Close()
	p.mux.paneExited(p)
}

// holdsFrame reports whether the program asked for its output to be shown as one frame, and that frame is still
// open. The caller holds the pane's lock.
func (p *Pane) holdsFrame() bool {
	if !p.screen.Modes().Sync {
		p.syncSince = time.Time{}
		return false
	}
	if p.syncSince.IsZero() {
		p.syncSince = time.Now()
	}
	return time.Since(p.syncSince) < syncLimit
}

// output tells the watchers a pane drew. Output nobody can see marks its window instead, one time.
func (m *Mux) output(p *Pane) {
	if p.visible.Load() {
		m.emit(Event{Kind: Dirty, Session: p.window.session})
		return
	}
	if !p.window.activity.Swap(true) {
		m.emit(Event{Kind: Changed, Session: p.window.session})
	}
}

// bell marks the pane's window when it rings out of sight.
func (p *Pane) bell() {
	if !p.visible.Load() && !p.window.bell.Swap(true) {
		p.mux.emit(Event{Kind: Changed, Session: p.window.session})
	}
}

// write sends queued input to the program. Input goes through a queue so that no caller waits on a program that
// does not read.
func (p *Pane) write() {
	for {
		select {
		case <-p.inSignal:
		case <-p.pty.Done():
			return
		}
		for {
			p.inMu.Lock()
			pending := p.in
			p.in = nil
			p.inMu.Unlock()
			if len(pending) == 0 {
				break
			}
			if _, err := p.pty.Write(pending); err != nil {
				return
			}
		}
	}
}

// Input queues bytes for the program to read.
func (p *Pane) Input(b []byte) {
	if len(b) == 0 {
		return
	}
	p.inMu.Lock()
	p.in = append(p.in, b...)
	p.inMu.Unlock()
	select {
	case p.inSignal <- struct{}{}:
	default:
	}
}

// keyModes is the part of the program's terminal modes that decides how input is encoded. The caller holds the
// pane's lock.
func (p *Pane) keyModes() keys.Modes {
	modes := p.screen.Modes()
	return keys.Modes{AppCursor: modes.AppCursor, BracketedPaste: modes.BracketedPaste, MouseSGR: modes.MouseSGR}
}

// Key sends a key press to the program, and returns the view to the live screen. It reports whether that changed
// what the pane shows: a view that was scrolled back, or a selection that was on.
func (p *Pane) Key(key input.Key) (changed bool) {
	p.mu.Lock()
	modes := p.keyModes()
	changed = p.offset != 0 || p.sel.on
	p.offset = 0
	p.sel = selection{}
	p.mu.Unlock()
	p.Input(keys.Encode(key, modes))
	return changed
}

// Paste sends pasted text to the program.
func (p *Pane) Paste(text string) {
	p.mu.Lock()
	modes := p.keyModes()
	p.offset = 0
	p.mu.Unlock()
	p.Input(keys.Paste(text, modes))
}

// Modes is the terminal modes the program has set.
func (p *Pane) Modes() vt.Modes {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.screen.Modes()
}

// Mouse sends a pointer event at a cell of the pane to the program, when the program asked for that kind of event.
// It reports whether the program takes the pointer at all.
func (p *Pane) Mouse(event input.Mouse, x, y int) bool {
	p.mu.Lock()
	modes := p.screen.Modes()
	encoding := p.keyModes()
	p.mu.Unlock()
	if modes.Mouse == vt.MouseOff {
		return false
	}
	wanted := false
	switch event.Action {
	case input.Press, input.Wheel:
		wanted = true
	case input.Release:
		wanted = modes.Mouse != vt.MouseX10
	case input.Motion:
		held := event.Button != input.ButtonNone
		wanted = modes.Mouse == vt.MouseAny || (modes.Mouse == vt.MouseButton && held)
	}
	if wanted {
		p.Input(keys.Mouse(event, x, y, encoding))
	}
	return true
}

// resize gives the terminal a new size. A pane that already has it is left alone.
func (p *Pane) resize(cols, rows int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if cols == p.cols && rows == p.rows {
		return
	}
	p.cols, p.rows = cols, rows
	p.screen.Resize(cols, rows)
	p.offset = min(p.offset, p.screen.HistoryLen())
	p.sel = selection{}
	_ = p.pty.Resize(cols, rows)
}

// close ends the pane's program. The pane leaves its window once the program is gone.
func (p *Pane) close() {
	p.pty.Close()
	// A program that ignores the hangup is not allowed to keep a pane the user closed.
	time.AfterFunc(time.Second, func() {
		select {
		case <-p.pty.Done():
		default:
			p.pty.Kill()
		}
	})
}

// Refresh reads what the pane's terminal runs in the foreground. It reports whether that changed.
func (p *Pane) Refresh() bool {
	process := pty.Process{}
	if pgrp, err := p.pty.Foreground(); err == nil {
		process, _ = pty.Describe(pgrp)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	changed := process != p.process
	p.process = process
	return changed
}

// commandName is the name of the program in the foreground, or of the one the pane was started with.
func (p *Pane) commandName() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.process.Name != "" {
		return p.process.Name
	}
	return p.command
}

// Dir is the working directory of the program in the foreground. A host that cannot say falls back to what the
// program reported about itself, then to where the pane started.
func (p *Pane) Dir() string {
	if pgrp, err := p.pty.Foreground(); err == nil {
		if process, err := pty.Describe(pgrp); err == nil && process.Dir != "" {
			return process.Dir
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if dir := p.screen.Cwd(); dir != "" {
		return dir
	}
	return p.startDir
}

// PaneInfo describes a pane for a status bar.
type PaneInfo struct {
	ID      int
	Command string
	Dir     string
	Title   string
	Cols    int
	Rows    int
	// Offset is how far back the view is scrolled, and History how far it could be.
	Offset  int
	History int
}

// Info describes the pane from what the last Refresh read.
func (p *Pane) Info() PaneInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	info := PaneInfo{
		ID: p.ID, Command: p.process.Name, Dir: p.process.Dir, Title: p.screen.Title(),
		Cols: p.cols, Rows: p.rows, Offset: p.offset, History: p.screen.HistoryLen(),
	}
	if info.Command == "" {
		info.Command = p.command
	}
	if info.Dir == "" {
		info.Dir = p.screen.Cwd()
	}
	if info.Dir == "" {
		info.Dir = p.startDir
	}
	return info
}

// Capture is the text now on the pane's screen.
func (p *Pane) Capture() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.screen.String()
}
