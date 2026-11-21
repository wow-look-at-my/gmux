package ui

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wow-look-at-my/go-tmux/layout"
	"github.com/wow-look-at-my/go-tmux/mux"
	"github.com/wow-look-at-my/tml"
)

// BenchmarkFrame is one whole frame of a large terminal. BenchmarkFrame panes full of coloured text, the lines
// between them and both bars, from the properties to the painted cells.
func BenchmarkFrame(b *testing.B) {
	const width, height = 200, 60
	m := mux.New(mux.Config{Shell: "/bin/sh", Scrollback: 500})
	defer m.Close()
	fill := `i=0; while [ $i -lt 80 ]; do printf '\033[3%dm%s\033[0m\n' $((i%8)) "` + strings.Repeat("colour text ", 16) + `"; i=$((i+1)); done; sleep 600`
	spawn := mux.Spawn{Command: []string{"/bin/sh", "-c", fill}}
	session, err := m.NewSession(mux.SessionOptions{Env: os.Environ(), Cols: width, Rows: height - barRows, Spawn: spawn})
	require.NoError(b, err)
	require.NoError(b, m.Split(session, layout.Columns, spawn))
	require.NoError(b, m.Split(session, layout.Rows, spawn))

	client, err := New(Options{Mux: m, Session: session, ID: 1, Stats: fakeStats{}})
	require.NoError(b, err)
	program, err := tml.NewProgram(client.View(), client, tml.ProgramOptions{
		Input: strings.NewReader(""), Output: io.Discard, Width: width, Height: height,
	})
	require.NoError(b, err)
	client.Update(program, tml.Resize{Width: width, Height: height})
	require.Eventually(b, func() bool {
		for _, pane := range m.View(session).Panes {
			if !strings.Contains(pane.Pane.Capture(), "colour text") {
				return false
			}
		}
		return true
	}, wait, 10*time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, err := client.View().Draw(client.Props(), width, height)
		require.NoError(b, err)

	}
}
