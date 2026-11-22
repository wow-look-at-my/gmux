// quiet_test.go covers the input that costs a client no frame.

package ui

import (
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wow-look-at-my/tml"
)

// counted is a client whose frames a test can count. The program reads the props one time for each frame.
type counted struct {
	*Client
	frames atomic.Int64
}

func (a *counted) Props() tml.Props {
	a.frames.Add(1)
	return a.Client.Props()
}

// running is a client on a running program, which reads what the test types.
type running struct {
	app  *counted
	keys *io.PipeWriter
	rig  *rig
}

// run starts a program for the rig's client, with a reader in its pane that prints nothing until a line ends.
// The rig must not draw after this: the program's loop owns the view.
func run(t *testing.T) *running {
	t.Helper()
	r := newRig(t, 60, 12)
	r.reader("")
	reader, writer := io.Pipe()
	app := &counted{Client: r.client}
	p, err := tml.NewProgram(r.client.View(), app, tml.ProgramOptions{
		Input: reader, Output: io.Discard, Width: r.width, Height: r.height,
	})
	require.NoError(t, err)
	stop := r.client.Start(p)
	go func() { _ = p.Run() }()
	t.Cleanup(func() {
		p.Quit()
		<-p.Done()
		stop()
		_ = writer.Close()
	})
	require.Eventually(t, func() bool { return app.frames.Load() > 0 }, wait, time.Millisecond, "no first frame")
	return &running{app: app, keys: writer, rig: r}
}

// types writes one input value at a time, so that each is a batch of its own for the program.
func (r *running) types(t *testing.T, value string, times int) {
	t.Helper()
	for range times {
		_, err := r.keys.Write([]byte(value))
		require.NoError(t, err)
		time.Sleep(2 * time.Millisecond)
	}
}

// arrived ends the line, and waits until the reader has printed what was typed.
func (r *running) arrived(t *testing.T, want string) {
	t.Helper()
	r.types(t, "\r", 1)
	pane := r.rig.mux.ActivePane(r.rig.session)
	require.Eventually(t, func() bool { return strings.Contains(pane.Capture(), want) }, wait, 5*time.Millisecond,
		"the reader never printed %q", want)
}

// The clock and the reader's answer paint a few frames. A frame for each value would paint more than half of count.
func assertFewFrames(t *testing.T, painted int64, count int, what string) {
	t.Helper()
	assert.Less(t, painted, int64(count/2), "%s painted %d frames for %d values", what, painted, count)
}

func TestATypedKeyCostsNoFrame(t *testing.T) {
	r := run(t)
	const count = 60
	before := r.app.frames.Load()
	r.types(t, "a", count)
	r.arrived(t, strings.Repeat("a", count))
	assertFewFrames(t, r.app.frames.Load()-before, count, "typing")
}

func TestThePointerMovingOverTheWindowCostsNoFrame(t *testing.T) {
	r := run(t)
	const count = 60
	before := r.app.frames.Load()
	// A report of the pointer moving with no button held, at a cell of the window.
	r.types(t, "\x1b[<35;10;4M", count)
	r.types(t, "z", 4)
	r.arrived(t, "zzzz")
	assertFewFrames(t, r.app.frames.Load()-before, count, "the pointer")
}
