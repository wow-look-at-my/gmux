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

// historyLen is how many samples a graph of the detail view draws.
const historyLen = 30

// barHistory is how many samples the graph on the bar draws.
const barHistory = 12

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
func (s *Sampler) Bar() Bar { return Format(s.Read()) }

// Detail is the newest sample as the detail view writes it.
func (s *Sampler) Detail() []Row { return Detail(s.Read()) }

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

// Bar is the host's load as the status bar writes it. An empty field is a
// reading the host does not offer, and the bar leaves that reading out.
type Bar struct {
	CPU string
	// CPUHistory is the recent CPU load in percent, oldest first.
	CPUHistory []string
	Mem        string
	Net        string
	GPU        string
}

// Format writes a sample for the bar.
func Format(snap *client.Snapshot) Bar {
	var bar Bar
	if snap == nil {
		return bar
	}
	if cpu, count, ok := processor(snap); ok {
		bar.CPU = percent(cpu.Value / count)
		bar.CPUHistory = full(history(cpu, count), barHistory)
	}
	if used, total, ok := pair(snap, client.RAMUsed, client.RAMTotal); ok && total > 0 {
		bar.Mem = Fixed(used) + "/" + Bytes(total)
	}
	if rx, tx, ok := pair(snap, client.NetworkRxRate, client.NetworkTxRate); ok {
		bar.Net = "↓" + Fixed(rx) + " ↑" + Fixed(tx)
	}
	if util, ok := snap.Value(client.GPUMetric(0, "utilization")); ok {
		// The first GPU stands for all. More than one device is rare on a machine somebody types at.
		bar.GPU = percent(util)
		if vram, ok := snap.Value(client.GPUMetric(0, "vram")); ok {
			bar.GPU += " " + Fixed(vram)
		}
	}
	return bar
}

// Row is one reading of the detail view: a label, a graph of the recent values, and the value in words.
type Row struct {
	Label string
	// History is the recent values, oldest first. A row with none draws no graph.
	History []string
	// Max is the value the top of the graph stands for. Zero lets the largest value of History be the top.
	Max  float64
	Text string
}

// Detail writes a sample for the detail view. It leaves out each reading the host does not offer.
func Detail(snap *client.Snapshot) []Row {
	if snap == nil {
		return nil
	}
	var rows []Row
	if cpu, count, ok := processor(snap); ok {
		rows = append(rows, Row{
			Label: "CPU", History: history(cpu, count), Max: 100,
			Text: percent(cpu.Value/count) + " of " + strconv.Itoa(int(count)) + " cores",
		})
	}
	rows = append(rows, filled(snap, "MEM", client.RAMUsed, client.RAMTotal)...)
	rows = append(rows, filled(snap, "SWAP", client.RAMSwapUsed, client.RAMSwapTotal)...)
	if util, ok := snap.Metrics[client.GPUMetric(0, "utilization")]; ok {
		rows = append(rows, Row{Label: "GPU", History: history(util, 1), Max: 100, Text: percent(util.Value)})
	}
	if vram, ok := snap.Metrics[client.GPUMetric(0, "vram")]; ok {
		row := Row{Label: "VRAM", History: history(vram, 1), Text: Fixed(vram.Value)}
		if vram.RangePossible != nil && vram.RangePossible[1] > 0 {
			row.Max = vram.RangePossible[1]
			row.Text = of(vram.Value, row.Max)
		}
		rows = append(rows, row)
	}
	rows = append(rows, rate(snap, "NET ↓", client.NetworkRxRate)...)
	rows = append(rows, rate(snap, "NET ↑", client.NetworkTxRate)...)
	rows = append(rows, filled(snap, "DISK", client.DiskUsed, client.DiskTotal)...)
	rows = append(rows, rate(snap, "READ", client.DiskIOReadRate)...)
	rows = append(rows, rate(snap, "WRITE", client.DiskIOWriteRate)...)
	if one, ok := snap.Metrics[client.SystemLoad1m]; ok {
		text := strconv.FormatFloat(one.Value, 'f', 2, 64)
		for _, path := range []string{client.SystemLoad5m, client.SystemLoad15m} {
			if v, ok := snap.Value(path); ok {
				text += " " + strconv.FormatFloat(v, 'f', 2, 64)
			}
		}
		rows = append(rows, Row{Label: "LOAD", History: history(one, 1), Text: text})
	}
	if up, ok := snap.Value(client.SystemUptime); ok {
		rows = append(rows, Row{Label: "UP", Text: Uptime(up)})
	}
	for i := range rows {
		if len(rows[i].History) > 0 {
			rows[i].History = full(rows[i].History, historyLen)
		}
	}
	return rows
}

// processor is the processor load and the count of processors it is a sum over. A load with no count has no scale.
func processor(snap *client.Snapshot) (cpu client.Metric, count float64, ok bool) {
	cpu, ok = snap.Metrics[client.CPUPercent]
	count, counted := snap.Value(client.CPUCount)
	return cpu, count, ok && counted && count >= 1
}

// pair is a pair of readings that only say something together.
func pair(snap *client.Snapshot, first, second string) (a, b float64, ok bool) {
	a, ok = snap.Value(first)
	b, hasSecond := snap.Value(second)
	return a, b, ok && hasSecond
}

// filled is the row of a store that has a size: memory, swap, a disk. A store of no size has no row.
func filled(snap *client.Snapshot, label, usedPath, totalPath string) []Row {
	used, ok := snap.Metrics[usedPath]
	total, hasTotal := snap.Value(totalPath)
	if !ok || !hasTotal || total <= 0 {
		return nil
	}
	return []Row{{Label: label, History: history(used, 1), Max: total, Text: of(used.Value, total)}}
}

// rate is the row of a count of bytes that move each second.
func rate(snap *client.Snapshot, label, path string) []Row {
	m, ok := snap.Metrics[path]
	if !ok {
		return nil
	}
	return []Row{{Label: label, History: history(m, 1), Text: Fixed(m.Value) + "/s"}}
}

// of writes how much of a store is in use.
func of(used, total float64) string {
	return Fixed(used) + " of " + Bytes(total) + " " + percent(100*used/total)
}

// history is a metric's values, oldest first, each divided by scale. The metric carries them newest first and
// leaves out the current value.
func history(m client.Metric, scale float64) []string {
	values := make([]string, 0, len(m.History)+1)
	for i := len(m.History) - 1; i >= 0; i-- {
		values = append(values, strconv.FormatFloat(m.History[i]/scale, 'f', 1, 64))
	}
	return append(values, strconv.FormatFloat(m.Value/scale, 'f', 1, 64))
}

// full is the newest n of a list of values. A list with fewer gets zeros ahead of it. A graph of it has its
// whole width from the first sample on and the newest value at its right end.
func full(values []string, n int) []string {
	if len(values) >= n {
		return values[len(values)-n:]
	}
	padded := make([]string, n-len(values), n)
	for i := range padded {
		padded[i] = "0"
	}
	return append(padded, values...)
}

// percent writes a share in cells, whatever its value.
func percent(v float64) string { return fmt.Sprintf("%3d%%", int(v+0.5)) }

// units are the binary multiples a byte count is written in.
var units = []string{"B", "K", "M", "G", "T", "P"}

// Bytes writes a byte count in a few characters: with a decimal while a single digit leads, and whole past that.
func Bytes(v float64) string {
	if v < 0 {
		v = 0
	}
	unit := 0
	for v >= 1024 && unit < len(units)-1 {
		v /= 1024
		unit++
	}
	if unit == 0 || v >= 9.95 {
		return fmt.Sprintf("%.0f%s", v, units[unit])
	}
	return fmt.Sprintf("%.1f%s", v, units[unit])
}

// Fixed writes a byte count in cells, whatever its value, so a count that changes does not move what is
// beside it. A count that needs a fourth digit goes to the next unit.
func Fixed(v float64) string {
	if v < 0 {
		v = 0
	}
	unit := 0
	for v >= 999.5 && unit < len(units)-1 {
		v /= 1024
		unit++
	}
	if unit > 0 && v < 9.95 {
		return fmt.Sprintf("%.1f%s", v, units[unit])
	}
	return fmt.Sprintf("%3.0f%s", v, units[unit])
}

// Uptime writes a count of seconds in its largest pair of units.
func Uptime(seconds float64) string {
	minutes := int(seconds) / 60
	days, hours, minutes := minutes/(24*60), minutes/60%24, minutes%60
	switch {
	case days > 0:
		return strconv.Itoa(days) + "d " + strconv.Itoa(hours) + "h"
	case hours > 0:
		return strconv.Itoa(hours) + "h " + strconv.Itoa(minutes) + "m"
	}
	return strconv.Itoa(minutes) + "m"
}
