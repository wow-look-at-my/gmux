// Package ui is what one attached terminal shows and does. The tml view, the status bars, and the keys and the
// pointer that drive a session.
package ui

import (
	"embed"
	"io/fs"
	"sync"
	"time"

	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/go-tmux/stats"
	"github.com/wow-look-at-my/tml"
	"github.com/wow-look-at-my/tml/widget"
)

//go:embed assets
var assets embed.FS

// barRows is how many rows the status bars take below the window area.
const barRows = 2

// minFrame is the least time between frames that pane output asks for. Input and structure changes never wait.
const minFrame = 8 * time.Millisecond

// Stats is where a client reads the host's load from.
type Stats interface {
	Bar(detail bool) stats.Bar
}

// Options are what a client is built from.
type Options struct {
	Mux     *mux.Mux
	Session *mux.Session
	// ID tells this client's size from the other clients of its session.
	ID int
	// Stats is nil on a host that offers none.
	Stats Stats
	// Light draws the bars for a terminal with a light background.
	Light bool
	// Now is the clock, replaced in a test.
	Now func() time.Time
}

// Outcome is why a client's program ended.
type Outcome int

const (
	// Detached is the user leaving the session running.
	Detached Outcome = iota
	// SessionEnded is the last program of the session exiting.
	SessionEnded
	// ServerClosed is the server shutting down.
	ServerClosed
)

// Client is one attached terminal. Its methods run on its program's goroutine, except where noted.
type Client struct {
	mux     *mux.Mux
	session *mux.Session
	id      int
	stats   Stats
	now     func() time.Time
	view    *tml.View

	width, height int
	outcome       Outcome
	quitting      bool

	// prefix is set between the prefix key and the key it applies to.
	prefix      bool
	cancelArmed func()
	// hover is the id of the bar element under the pointer.
	hover string
	menu  *menu
	ask   *prompt
	help  bool
	note  string
	// noteUntil is when the note leaves the bar.
	noteUntil time.Time
	detail    bool
	longClock bool
	// clip is the text copied last, which the middle button pastes.
	clip  string
	drag  drag
	click clickTracker

	paceMu  sync.Mutex
	pending bool
	last    time.Time
	stop    func()
}

// New loads the view for a client of a session.
func New(opts Options) (*Client, error) {
	c := &Client{mux: opts.Mux, session: opts.Session, id: opts.ID, stats: opts.Stats, now: opts.Now}
	if c.now == nil {
		c.now = time.Now
	}
	files, err := fs.Sub(assets, "assets")
	if err != nil {
		return nil, err
	}
	registry := widget.NewRegistry().Bind("Panes", newPanes(c)).Bind("Hit", hit{})
	c.view, err = tml.Load(files, "screen.tml", tml.Options{Dark: !opts.Light, Widgets: registry})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// View is the document the client draws.
func (c *Client) View() *tml.View { return c.view }

// Outcome is why the program ended. It is valid once the program has.
func (c *Client) Outcome() Outcome { return c.outcome }

// Session is the session the client shows now.
func (c *Client) Session() *mux.Session { return c.session }

// The messages a client's program receives from outside its own terminal.
type (
	// frameMsg asks for a repaint, with nothing else to say.
	frameMsg struct{}
	// tickMsg is the second passing: the clock and the host's load move.
	tickMsg struct{}
	// endedMsg is a session ending.
	endedMsg struct{ session *mux.Session }
	// clipboardMsg is a program in a pane setting the clipboard.
	clipboardMsg struct{ text string }
	// disarmMsg ends the time a repeatable key may follow the prefix without it.
	disarmMsg struct{}
	// closeMsg is the server shutting down.
	closeMsg struct{}
)

// Start ties the client to its program: the session's events reach the program from here on. The function it
// returns unties them.
func (c *Client) Start(p *tml.Program) (stop func()) {
	unwatch := c.mux.Watch(func(event mux.Event) {
		switch event.Kind {
		case mux.Dirty:
			if event.Session == c.shown() {
				c.pace(p)
			}
		case mux.Changed:
			p.Send(frameMsg{})
		case mux.Ended:
			p.Send(endedMsg{session: event.Session})
		case mux.Clipboard:
			if event.Session == c.shown() {
				p.Send(clipboardMsg{text: event.Text})
			}
		}
	})
	cancelTick := p.Every(time.Second, tickMsg{})
	c.stop = func() {
		unwatch()
		cancelTick()
	}
	return c.stop
}

// Wake asks the program for a frame, which is also when it reads its terminal's size again.
func (c *Client) Wake(p *tml.Program) { p.Send(frameMsg{}) }

// Close tells the program the server is shutting down. It is safe from any goroutine.
func (c *Client) Close(p *tml.Program) { p.Send(closeMsg{}) }

// shown is the session on screen, read from a goroutine that is not the program's.
func (c *Client) shown() *mux.Session {
	c.paceMu.Lock()
	defer c.paceMu.Unlock()
	return c.session
}

// show moves the client to another session.
func (c *Client) show(s *mux.Session) {
	old := c.session
	c.paceMu.Lock()
	c.session = s
	c.paceMu.Unlock()
	c.mux.Detach(old, c.id)
	c.attach()
}

// attach tells the session how much room this client has for its windows.
func (c *Client) attach() {
	if c.width > 0 && c.height > barRows {
		c.mux.Attach(c.session, c.id, c.width, c.height-barRows)
	}
}

// pace asks the program for a frame, and holds a burst of pane output to one frame per minFrame. It runs on the
// goroutine of whatever pane drew.
func (c *Client) pace(p *tml.Program) {
	c.paceMu.Lock()
	if c.pending {
		c.paceMu.Unlock()
		return
	}
	c.pending = true
	wait := minFrame - time.Since(c.last)
	c.paceMu.Unlock()
	fire := func() {
		c.paceMu.Lock()
		c.pending = false
		c.last = time.Now()
		c.paceMu.Unlock()
		p.Send(frameMsg{})
	}
	if wait <= 0 {
		fire()
		return
	}
	time.AfterFunc(wait, fire)
}

// overlay names what is drawn over the window, which takes the keys: the menu, the prompt, the help, or nothing.
func (c *Client) overlay() string {
	switch {
	case c.ask != nil:
		return "prompt"
	case c.menu != nil:
		return "menu"
	case c.help:
		return "help"
	}
	return ""
}

// say puts a short note on the bar for a moment.
func (c *Client) say(p *tml.Program, text string) {
	c.note = text
	c.noteUntil = c.now().Add(noteTime)
	p.After(noteTime, frameMsg{})
}

// noteTime is how long a note stays on the bar.
const noteTime = 2 * time.Second

// quit ends the program with a reason.
func (c *Client) quit(p *tml.Program, why Outcome) {
	// The first reason stands: a server that closes because its last session ended says both things.
	if c.quitting {
		return
	}
	c.quitting = true
	if _, alive := c.mux.Info(c.session); !alive && why == ServerClosed {
		why = SessionEnded
	}
	c.outcome = why
	c.mux.Detach(c.session, c.id)
	if c.stop != nil {
		c.stop()
	}
	p.Quit()
}
