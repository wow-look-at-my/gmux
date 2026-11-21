// Package stats reads the host's load for the status bar. The readings come from simple-stats-api's sources, run
// inside this process.
package stats

import (
	"fmt"
	"os/exec"
	"strconv"
	"sync"

	"github.com/wow-look-at-my/simple-stats-api/client"
	"github.com/wow-look-at-my/simple-stats-api/hoststats"
)

// historyLen is how many samples the load graph draws.
const historyLen = 12

// Sampler samples the host until Close.
type Sampler struct {
	mu      sync.Mutex
	sampler *hoststats.Sampler
	closed  bool
}

// Start begins sampling on a goroutine of its own, because looking for a GPU can take seconds. Readings are empty
// until the first sample is in. logf receives each source failure.
func Start(logf func(format string, args ...any)) *Sampler {
	s := &Sampler{}
	go func() {
		_, err := exec.LookPath("nvidia-smi")
		sampler, startErr := hoststats.Start(hoststats.Options{HistorySize: historyLen, GPU: err == nil, Logf: logf})
		if startErr != nil {
			logf("host stats: %v", startErr)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed {
			sampler.Close()
			return
		}
		s.sampler = sampler
	}()
	return s
}

// Read is the newest sample. It is nil until the sampler has one.
func (s *Sampler) Read() *client.Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sampler == nil {
		return nil
	}
	return s.sampler.Snapshot(historyLen)
}

// Bar is the newest sample as the status bar writes it.
func (s *Sampler) Bar(detail bool) Bar { return Format(s.Read(), detail) }

// Close stops sampling.
func (s *Sampler) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.sampler != nil {
		s.sampler.Close()
		s.sampler = nil
	}
}

// Bar is the host's load as the status bar writes it. An empty field is a reading the host does not offer, and the
// bar leaves that reading out.
type Bar struct {
	CPU string
	// CPUHistory is the recent CPU load in percent, oldest first.
	CPUHistory []string
	Mem        string
	Net        string
	Load       string
	GPU        string
}

// Format writes a sample for the bar. detail asks for the longer form of each reading.
func Format(snap *client.Snapshot, detail bool) Bar {
	var bar Bar
	if snap == nil {
		return bar
	}
	if cpu, ok := snap.Metrics[client.CPUPercent]; ok {
		bar.CPU = percent(cpu.Value)
		if count, ok := snap.Value(client.CPUCount); ok && detail {
			bar.CPU += " of " + strconv.Itoa(int(count))
		}
		// The history arrives newest first and leaves out the current value.
		for i := len(cpu.History) - 1; i >= 0; i-- {
			bar.CPUHistory = append(bar.CPUHistory, strconv.FormatFloat(cpu.History[i], 'f', 1, 64))
		}
		bar.CPUHistory = append(bar.CPUHistory, strconv.FormatFloat(cpu.Value, 'f', 1, 64))
	}
	bar.Mem = memory(snap, detail)
	bar.Net = network(snap)
	bar.Load = load(snap, detail)
	bar.GPU = gpu(snap, detail)
	return bar
}

func percent(v float64) string { return strconv.Itoa(int(v+0.5)) + "%" }

func memory(snap *client.Snapshot, detail bool) string {
	used, ok := snap.Value(client.RAMUsed)
	total, hasTotal := snap.Value(client.RAMTotal)
	if !ok || !hasTotal || total <= 0 {
		return ""
	}
	text := Bytes(used) + "/" + Bytes(total)
	if !detail {
		return text
	}
	if swap, ok := snap.Value(client.RAMSwapUsed); ok && swap > 0 {
		text += " swap " + Bytes(swap)
	}
	return text
}

func network(snap *client.Snapshot) string {
	rx, ok := snap.Value(client.NetworkRxRate)
	tx, hasTx := snap.Value(client.NetworkTxRate)
	if !ok || !hasTx {
		return ""
	}
	return "↓" + Bytes(rx) + " ↑" + Bytes(tx)
}

func load(snap *client.Snapshot, detail bool) string {
	one, ok := snap.Value(client.SystemLoad1m)
	if !ok {
		return ""
	}
	text := strconv.FormatFloat(one, 'f', 2, 64)
	if !detail {
		return text
	}
	for _, path := range []string{client.SystemLoad5m, client.SystemLoad15m} {
		if v, ok := snap.Value(path); ok {
			text += " " + strconv.FormatFloat(v, 'f', 2, 64)
		}
	}
	return text
}

// gpu writes the first GPU's use. More than one device is rare on a machine somebody types at.
func gpu(snap *client.Snapshot, detail bool) string {
	util, ok := snap.Value(client.GPUMetric(0, "utilization"))
	if !ok {
		return ""
	}
	text := percent(util)
	vram, ok := snap.Metrics[client.GPUMetric(0, "vram")]
	if !ok {
		return text
	}
	text += " " + Bytes(vram.Value)
	if detail && vram.RangePossible != nil {
		text += "/" + Bytes(vram.RangePossible[1])
	}
	return text
}

// units are the binary multiples a byte count is written in.
var units = []string{"B", "K", "M", "G", "T", "P"}

// Bytes writes a byte count in a few characters: a whole number below units of a size, one decimal otherwise
// only where it fits.
func Bytes(v float64) string {
	if v < 0 {
		v = 0
	}
	unit := 0
	for v >= 1024 && unit < len(units)-1 {
		v /= 1024
		unit++
	}
	if unit == 0 || v >= 100 {
		return fmt.Sprintf("%.0f%s", v, units[unit])
	}
	return fmt.Sprintf("%.1f%s", v, units[unit])
}
