package mux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// BenchmarkPaneOutput is a program writing a large file to its terminal: the pty, the read loop, the emulator and
// the watchers. The count is the bytes of the file per run.
func BenchmarkPaneOutput(b *testing.B) {
	const size = 16 << 20
	line := strings.Repeat("the quick brown fox jumps over the lazy dog ", 2) + "\n"
	path := filepath.Join(b.TempDir(), "big.txt")
	require.NoError(b, os.WriteFile(path, []byte(strings.Repeat(line, size/len(line))), 0o600))

	m := New(Config{Shell: "/bin/sh", Scrollback: 1000})
	defer m.Close()
	frames := 0
	defer m.Watch(func(e Event) {
		if e.Kind == Dirty {
			frames++
		}
	})()

	b.SetBytes(size)
	b.ResetTimer()
	for range b.N {
		_, err := m.NewSession(SessionOptions{Env: os.Environ(), Spawn: Spawn{Command: []string{"/bin/cat", path}}})
		require.NoError(b, err)
		deadline := time.Now().Add(time.Minute)
		for !m.Empty() {
			if time.Now().After(deadline) {
				b.Fatal("the program never finished")
			}
			time.Sleep(time.Millisecond)
		}
	}
}
