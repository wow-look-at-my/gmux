// proto.go holds what a client and the server say to each other: one JSON value per line, over a unix socket.

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// The operations a client asks for.
const (
	OpAttach      = "attach"
	OpNew         = "new"
	OpList        = "list"
	OpKillServer  = "kill-server"
	OpKillSession = "kill-session"
	OpSendKeys    = "send-keys"
	OpCapture     = "capture"
	OpRename      = "rename"
	// OpResize follows an attach, on the same connection: the client's terminal changed size.
	OpResize = "resize"
)

// Request is one thing a client asks the server to do.
type Request struct {
	Op string `json:"op"`
	// Target names a session. Empty is the one used most recently.
	Target string `json:"target,omitempty"`
	// Create makes an attach start a new session instead of joining one.
	Create bool `json:"create,omitempty"`
	// Name is a new session's name, or the new name of a renamed one.
	Name    string   `json:"name,omitempty"`
	Command []string `json:"command,omitempty"`
	Dir     string   `json:"dir,omitempty"`
	// Env is the client's environment, which a new session's programs start with.
	Env  []string `json:"env,omitempty"`
	Cols int      `json:"cols,omitempty"`
	Rows int      `json:"rows,omitempty"`
	// Keys are the keys to send: key names, or text when Literal is set.
	Keys    []string `json:"keys,omitempty"`
	Literal bool     `json:"literal,omitempty"`
	// Light says the client's terminal has a light background.
	Light bool `json:"light,omitempty"`
}

// SessionLine is one session in a listing.
type SessionLine struct {
	Name     string `json:"name"`
	Windows  int    `json:"windows"`
	Attached int    `json:"attached"`
	Created  int64  `json:"created"`
	Cols     int    `json:"cols"`
	Rows     int    `json:"rows"`
}

// Response is the server's answer.
type Response struct {
	// Error is why the request failed. Empty means it did not.
	Error string `json:"error,omitempty"`
	// Output is text for the client to print.
	Output string `json:"output,omitempty"`
	// Session is the name of the session the request acted on.
	Session  string        `json:"session,omitempty"`
	Sessions []SessionLine `json:"sessions,omitempty"`
	// Exit says why an attach ended: detached, exited, or closed.
	Exit string `json:"exit,omitempty"`
}

// The reasons an attach ends.
const (
	ExitDetached = "detached"
	ExitExited   = "exited"
	ExitClosed   = "closed"
)

// SocketEnv names the variable that overrides the socket path.
const SocketEnv = "GMUX_SOCKET"

// SocketPath is where the server with a name listens. An explicit path wins, then the environment, then a
// directory that only this user can read.
func SocketPath(explicit, name string) string {
	if explicit != "" {
		return explicit
	}
	if path := os.Getenv(SocketEnv); path != "" && name == "" {
		return path
	}
	if name == "" {
		name = "default"
	}
	dir := filepath.Join(os.TempDir(), "gmux-"+strconv.Itoa(os.Getuid()))
	if run := os.Getenv("XDG_RUNTIME_DIR"); run != "" {
		dir = filepath.Join(run, "gmux")
	}
	return filepath.Join(dir, name+".sock")
}

// WriteMessage sends one value as a line, with open files riding along when there are any.
func WriteMessage(conn *net.UnixConn, value any, files ...*os.File) error {
	line, err := json.Marshal(value)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	var rights []byte
	if len(files) > 0 {
		fds := make([]int, len(files))
		for i, file := range files {
			fds[i] = int(file.Fd())
		}
		rights = syscall.UnixRights(fds...)
	}
	_, _, err = conn.WriteMsgUnix(line, rights, nil)
	return err
}

// maxLine bounds one message. A request carries an environment, so it is not small.
const maxLine = 1 << 20

// Reader reads the lines of one connection, and the files that ride along with them.
type Reader struct {
	conn  *net.UnixConn
	buf   []byte
	files []*os.File
}

// NewReader reads messages from a connection.
func NewReader(conn *net.UnixConn) *Reader { return &Reader{conn: conn} }

// Read decodes the next line into value.
func (r *Reader) Read(value any) error {
	for {
		for i, b := range r.buf {
			if b == '\n' {
				line := r.buf[:i]
				err := json.Unmarshal(line, value)
				r.buf = append(r.buf[:0], r.buf[i+1:]...)
				return err
			}
		}
		if len(r.buf) > maxLine {
			return errors.New("the message is too long")
		}
		chunk := make([]byte, 64<<10)
		oob := make([]byte, syscall.CmsgSpace(4*8))
		n, oobn, _, _, err := r.conn.ReadMsgUnix(chunk, oob)
		if oobn > 0 {
			r.takeFiles(oob[:oobn])
		}
		if n > 0 {
			r.buf = append(r.buf, chunk[:n]...)
			continue
		}
		if err != nil {
			return err
		}
	}
}

// takeFiles keeps the files a message carried.
func (r *Reader) takeFiles(oob []byte) {
	messages, err := syscall.ParseSocketControlMessage(oob)
	if err != nil {
		return
	}
	for _, message := range messages {
		fds, err := syscall.ParseUnixRights(&message)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			syscall.CloseOnExec(fd)
			r.files = append(r.files, os.NewFile(uintptr(fd), "client"))
		}
	}
}

// Files hands over the files received so far.
func (r *Reader) Files() []*os.File {
	files := r.files
	r.files = nil
	return files
}

// Dial connects to the server at a socket path.
func Dial(path string) (*net.UnixConn, error) {
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("no server is running on %s: %w", path, err)
	}
	return conn, nil
}

// Ask sends one request and reads its answer. A failure the server reports comes back as an error.
func Ask(path string, req Request) (Response, error) {
	conn, err := Dial(path)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	if err := WriteMessage(conn, req); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := NewReader(conn).Read(&resp); err != nil {
		return Response{}, fmt.Errorf("the server closed the connection: %w", err)
	}
	if resp.Error != "" {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}
