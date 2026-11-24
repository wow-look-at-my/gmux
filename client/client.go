// Package client is the short-lived half of the multiplexer: it finds or starts the server, hands its terminal
// over, and waits.
package client

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/wow-look-at-my/go-tmux/server"
	"github.com/wow-look-at-my/tml/term"
	"golang.org/x/sys/unix"
)

// startWait is how long a server gets to answer after it is started.
const startWait = 5 * time.Second

// NestEnv is the variable a pane's program carries, which is how a client knows it runs inside a session.
const NestEnv = "GMUX"

// Start makes sure a server answers on the socket, starting one when none does. exe is this program.
func Start(socket, exe string) error {
	if conn, err := server.Dial(socket); err == nil {
		conn.Close()
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(socket), 0o700); err != nil {
		return err
	}
	log, err := os.OpenFile(socket+".log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	// The program goes through a shell, because a portable executable is one a shell can start on every host.
	cmd := exec.Command("/bin/sh", "-c", `exec "$0" "$@"`, exe, "server", "run", "--socket", socket)
	cmd.Stdout, cmd.Stderr = log, log
	cmd.Dir = "/"
	// A session of its own keeps the server alive when the terminal that started it closes.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start the server: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	deadline := time.After(startWait)
	for {
		if conn, err := server.Dial(socket); err == nil {
			conn.Close()
			return nil
		}
		select {
		case err := <-exited:
			return fmt.Errorf("the server did not start (%v); see %s.log", err, socket)
		case <-deadline:
			return fmt.Errorf("the server did not answer within %s; see %s.log", startWait, socket)
		case <-time.After(time.Millisecond):
		}
	}
}

// Terminal is the terminal a client hands to the server.
type Terminal struct {
	In, Out *os.File
}

// restore is what puts a terminal back when the server could not: out of the alternate screen, cursor shown.
const restore = "\x1b[?1000l\x1b[?1003l\x1b[?1006l\x1b[?2004l\x1b[?25h\x1b[?1049l"

// Attach hands the terminal to the server and waits until the server gives it back. It reports why the attach
// ended.
func Attach(socket string, req server.Request, tty Terminal) (server.Response, error) {
	if !term.IsTerminal(tty.In) || !term.IsTerminal(tty.Out) {
		return server.Response{}, errors.New("attaching needs a terminal for input and output")
	}
	conn, err := server.Dial(socket)
	if err != nil {
		return server.Response{}, err
	}
	defer conn.Close()

	// The server puts the terminal in raw mode and takes it back out. This copy is for a server that dies first.
	saved, savedErr := unix.IoctlGetTermios(int(tty.In.Fd()), unix.TCGETS)
	req.Op = server.OpAttach
	if cols, rows, err := term.Size(tty.Out); err == nil {
		req.Cols, req.Rows = cols, rows
	}
	if err := server.WriteMessage(conn, req, tty.In, tty.Out); err != nil {
		return server.Response{}, err
	}

	signals := make(chan os.Signal, 4)
	signal.Notify(signals, syscall.SIGWINCH, syscall.SIGHUP, syscall.SIGTERM)
	defer signal.Stop(signals)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case sig := <-signals:
				if sig != syscall.SIGWINCH {
					// The terminal is gone, or the user asked the client to end: the server sees the connection close.
					conn.Close()
					return
				}
				_ = server.WriteMessage(conn, server.Request{Op: server.OpResize})
			case <-stop:
				return
			}
		}
	}()

	var resp server.Response
	if err := server.NewReader(conn).Read(&resp); err != nil {
		if savedErr == nil {
			_ = unix.IoctlSetTermios(int(tty.In.Fd()), unix.TCSETS, saved)
		}
		_, _ = io.WriteString(tty.Out, restore)
		return server.Response{}, fmt.Errorf("lost the server: %w", err)
	}
	if resp.Error != "" {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}
