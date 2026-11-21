// attach.go holds the server's half of an attached client: a program drawn on the terminal the client handed over.

package server

import (
	"errors"
	"net"
	"os"
	"strings"

	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/go-tmux/ui"
	"github.com/wow-look-at-my/tml"
)

// attach runs a client's program until it detaches, its session ends, or its terminal goes away.
func (s *Server) attach(conn *net.UnixConn, reader *Reader, req Request, files []*os.File) {
	fail := func(err error) { _ = WriteMessage(conn, Response{Error: err.Error()}) }
	if len(files) < 2 {
		fail(errors.New("an attach hands over the terminal's input and output"))
		return
	}
	session, err := s.sessionFor(req)
	if err != nil {
		fail(err)
		return
	}
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.mu.Unlock()

	options := ui.Options{Mux: s.mux, Session: session, ID: id, Light: req.Light}
	if s.stats != nil {
		options.Stats = s.stats
	}
	client, err := ui.New(options)
	if err != nil {
		fail(err)
		return
	}
	// The terminal is the client's, so what it can show is read from the client's environment.
	program, err := tml.NewProgram(client.View(), client, tml.ProgramOptions{
		Input: files[0], Output: files[1], Env: lookup(req.Env), HostSignals: true,
	})
	if err != nil {
		fail(err)
		return
	}
	stop := client.Start(program)
	defer stop()

	s.mu.Lock()
	s.attached[id] = &attachment{program: program, client: client}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.attached, id)
		s.mu.Unlock()
	}()

	go func() {
		// Every line after the request says the terminal changed size.
		for {
			var msg Request
			if err := reader.Read(&msg); err != nil {
				client.Close(program)
				return
			}
			client.Wake(program)
		}
	}()

	s.mux.Touch(session)
	runErr := program.Run()
	resp := Response{Session: session.Name, Exit: exitName(client.Outcome())}
	if runErr != nil && !errors.Is(runErr, tml.ErrInterrupted) {
		resp.Error = runErr.Error()
	}
	_ = WriteMessage(conn, resp)
}

// sessionFor finds the session an attach names, or creates the one it asks for.
func (s *Server) sessionFor(req Request) (*mux.Session, error) {
	if req.Create {
		return s.create(req)
	}
	return s.mux.Find(req.Target)
}

func exitName(outcome ui.Outcome) string {
	switch outcome {
	case ui.SessionEnded:
		return ExitExited
	case ui.ServerClosed:
		return ExitClosed
	}
	return ExitDetached
}

// lookup reads a variable from an environment given as a list.
func lookup(env []string) func(string) string {
	return func(name string) string {
		for _, entry := range env {
			if value, ok := strings.CutPrefix(entry, name+"="); ok {
				return value
			}
		}
		return ""
	}
}
