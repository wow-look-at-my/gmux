// Package server is the process that outlives the terminals: it owns the sessions, and draws each attached client
// on the terminal that client handed over.
package server

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/wow-look-at-my/go-tmux/keys"
	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/go-tmux/stats"
	"github.com/wow-look-at-my/go-tmux/ui"
	"github.com/wow-look-at-my/tml"
	"github.com/wow-look-at-my/tml/input"
)

// idleLimit is how long a server with no session and no client stays up.
const idleLimit = 10 * time.Second

// Options configure a server.
type Options struct {
	// Socket is the path the server listens on.
	Socket string
	// Mux configures the panes.
	Mux mux.Config
	// NoStats leaves the host's load off the bars.
	NoStats bool
	// Logf receives what the server has to say. Nil discards it.
	Logf func(format string, args ...any)
}

// Server owns the sessions and the attached clients.
type Server struct {
	opts  Options
	mux   *mux.Mux
	stats *stats.Sampler
	ln    *net.UnixListener

	mu       sync.Mutex
	attached map[int]*attachment
	nextID   int
	busy     int
	lastUsed time.Time
	hadWork  bool

	closeOnce sync.Once
	done      chan struct{}
}

// attachment is one client's program.
type attachment struct {
	program *tml.Program
	client  *ui.Client
}

// New prepares a server. Nothing listens until Listen.
func New(opts Options) *Server {
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	opts.Mux.Socket = opts.Socket
	opts.Mux.Gutter = ui.Gutter
	return &Server{
		opts: opts, mux: mux.New(opts.Mux), attached: map[int]*attachment{},
		done: make(chan struct{}), lastUsed: time.Now(),
	}
}

// Mux is the server's sessions.
func (s *Server) Mux() *mux.Mux { return s.mux }

// ErrRunning is what Listen reports when another server already answers on the socket.
var ErrRunning = errors.New("a server is already running on this socket")

// Listen takes the socket. A socket file nobody answers on is a dead server's, and is replaced.
func (s *Server) Listen() error {
	path := s.opts.Socket
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}
	if conn, err := net.DialTimeout("unix", path, time.Second); err == nil {
		conn.Close()
		return ErrRunning
	} else if errors.Is(err, syscall.ECONNREFUSED) {
		_ = os.Remove(path)
	}
	// The socket is the right to drive every session, so nobody else may open it.
	old := syscall.Umask(0o077)
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	syscall.Umask(old)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", path, err)
	}
	s.ln = ln
	if !s.opts.NoStats {
		s.stats = stats.Start(s.opts.Logf)
	}
	return nil
}

// Serve answers clients until the server closes: on a kill, or once the last session has ended.
func (s *Server) Serve() error {
	stopWatch := s.mux.Watch(func(event mux.Event) {
		if event.Kind == mux.Ended {
			// The watcher must not call back into the multiplexer, so the check runs on its own goroutine.
			go s.closeIfEmpty()
		}
	})
	defer stopWatch()
	go s.tick()
	for {
		conn, err := s.ln.AcceptUnix()
		if errors.Is(err, net.ErrClosed) {
			// The sessions end after the listener closes. The process must not leave before them.
			<-s.done
			return nil
		}
		if err != nil {
			return err
		}
		go s.handle(conn)
	}
}

// tick refreshes what the bars say about the running programs, and ends a server nobody uses.
func (s *Server) tick() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mux.Refresh()
			if s.idle() {
				s.opts.Logf("no session and no client: closing")
				s.Close()
			}
		}
	}
}

// idle reports whether the server has had nothing to do for longer than the limit.
func (s *Server) idle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.busy == 0 && s.mux.Empty() && time.Since(s.lastUsed) > idleLimit
}

// closeIfEmpty ends the server once its last session has ended.
func (s *Server) closeIfEmpty() {
	s.mu.Lock()
	had := s.hadWork
	s.mu.Unlock()
	if had && s.mux.Empty() {
		s.shutdown(ui.SessionEnded)
	}
}

// Close ends every session, detaches every client and stops listening.
func (s *Server) Close() { s.shutdown(ui.ServerClosed) }

// shutdown closes the server, and tells each client why its program ends.
func (s *Server) shutdown(why ui.Outcome) {
	s.closeOnce.Do(func() {
		// The listener closes first and the signal comes last. A server that says it closed answers nobody.
		if s.ln != nil {
			s.ln.Close()
		}
		defer close(s.done)
		s.mu.Lock()
		for _, a := range s.attached {
			a.client.Close(a.program, why)
		}
		s.mu.Unlock()
		s.mux.Close()
		if s.stats != nil {
			s.stats.Close()
		}
	})
}

// Done is closed once the server has closed.
func (s *Server) Done() <-chan struct{} { return s.done }

// handle answers one connection.
func (s *Server) handle(conn *net.UnixConn) {
	defer conn.Close()
	s.mu.Lock()
	s.busy++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.busy--
		s.lastUsed = time.Now()
		s.mu.Unlock()
	}()

	reader := NewReader(conn)
	var req Request
	if err := reader.Read(&req); err != nil {
		return
	}
	files := reader.Files()
	defer func() {
		for _, file := range files {
			file.Close()
		}
	}()
	if req.Op == OpAttach {
		s.attach(conn, reader, req, files)
		return
	}
	resp, err := s.answer(req)
	if err != nil {
		resp.Error = err.Error()
	}
	_ = WriteMessage(conn, resp)
	if req.Op == OpKillServer && err == nil {
		s.Close()
	}
}

// answer runs a request that needs no terminal.
func (s *Server) answer(req Request) (Response, error) {
	switch req.Op {
	case OpNew:
		session, err := s.create(req)
		if err != nil {
			return Response{}, err
		}
		return Response{Session: session.Name}, nil
	case OpList:
		var resp Response
		for _, info := range s.mux.List() {
			resp.Sessions = append(resp.Sessions, SessionLine{
				Name: info.Name, Windows: len(info.Windows), Attached: info.Attached,
				Created: info.Created.Unix(), Cols: info.Cols, Rows: info.Rows,
			})
		}
		return resp, nil
	case OpKillServer:
		return Response{}, nil
	}
	session, err := s.mux.Find(req.Target)
	if err != nil {
		return Response{}, err
	}
	resp := Response{Session: session.Name}
	switch req.Op {
	case OpKillSession:
		s.mux.KillSession(session)
	case OpRename:
		err = s.mux.RenameSession(session, req.Name)
	case OpCapture:
		if pane := s.mux.ActivePane(session); pane != nil {
			resp.Output = pane.Capture()
		}
	case OpSendKeys:
		err = s.sendKeys(session, req)
	default:
		err = fmt.Errorf("unknown request %q", req.Op)
	}
	return resp, err
}

// create starts a session for a request.
func (s *Server) create(req Request) (*mux.Session, error) {
	session, err := s.mux.NewSession(mux.SessionOptions{
		Name: req.Name, Env: req.Env, Cols: req.Cols, Rows: req.Rows,
		Spawn: mux.Spawn{Command: req.Command, Dir: req.Dir},
	})
	if err == nil {
		s.mu.Lock()
		s.hadWork = true
		s.mu.Unlock()
	}
	return session, err
}

// keyNames are the spellings of keys that tmux users type, mapped to this program's own.
var keyNames = map[string]string{
	"escape": "esc", "bspace": "backspace", "pageup": "pgup", "pagedown": "pgdown", "ppage": "pgup",
	"npage": "pgdown", "dc": "delete", "ic": "insert", "btab": "shift+tab",
}

// parseKey reads a key by name: this program's spelling, or tmux's. ok is false for anything else, which is then
// sent as text.
func parseKey(name string) (input.Key, bool) {
	base, mods := strings.ToLower(name), ""
	for cut := true; cut; {
		cut = false
		for from, to := range modNames {
			if rest, ok := strings.CutPrefix(base, from); ok && rest != "" {
				base, mods, cut = rest, mods+to, true
			}
		}
	}
	if known, ok := keyNames[base]; ok {
		base = known
	}
	key := input.ParseKey(mods + base)
	runes := len([]rune(base))
	// A name longer than one character is a key only when it is one of the keys that have a name.
	named := runes > 1 && (key.Code > unicode.MaxRune || key.Code <= ' ' || key.Code == input.KeyBackspace)
	if named || (mods != "" && runes == 1) {
		return key, true
	}
	return input.Key{}, false
}

// modNames are the spellings of a held modifier, tmux's and this program's, mapped to this program's.
var modNames = map[string]string{
	"c-": "ctrl+", "m-": "alt+", "s-": "shift+", "ctrl+": "ctrl+", "alt+": "alt+", "shift+": "shift+",
}

// sendKeys types keys into a session's active pane, as if a client had.
func (s *Server) sendKeys(session *mux.Session, req Request) error {
	pane := s.mux.ActivePane(session)
	if pane == nil {
		return errors.New("the session has no pane")
	}
	for _, name := range req.Keys {
		if key, ok := parseKey(name); ok && !req.Literal {
			pane.Key(key)
			continue
		}
		pane.Input(keys.Paste(name, keys.Modes{}))
	}
	return nil
}
