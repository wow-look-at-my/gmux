package stats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wow-look-at-my/simple-stats-api/client"
)

func snapshot() *client.Snapshot {
	return &client.Snapshot{Metrics: map[string]client.Metric{
		client.CPUPercent:                  {Value: 16 * 12.4, History: []float64{16 * 40, 16 * 5.25}},
		client.CPUCount:                    {Value: 16},
		client.RAMUsed:                     {Value: 3.2 * (1 << 30)},
		client.RAMTotal:                    {Value: 16 * (1 << 30)},
		client.RAMSwapUsed:                 {Value: 512 * (1 << 20)},
		client.NetworkRxRate:               {Value: 1234},
		client.NetworkTxRate:               {Value: 300},
		client.SystemLoad1m:                {Value: 0.519},
		client.SystemLoad5m:                {Value: 0.4},
		client.SystemLoad15m:               {Value: 0.31},
		client.GPUMetric(0, "utilization"): {Value: 87.6},
		client.GPUMetric(0, "vram"):        {Value: 6 * (1 << 30), RangePossible: &client.Range{0, 24 * (1 << 30)}},
	}}
}

func TestFormatWritesEachReadingShort(t *testing.T) {
	bar := Format(snapshot())
	assert.Equal(t, Bar{
		CPU: " 12%", CPUHistory: []string{"5.2", "40.0", "12.4"}, Mem: "3.2G/16G", Net: "↓1.2K ↑300B",
		GPU: " 88% 6.0G",
	}, bar)
}

func TestAReadingKeepsItsWidthWhileItsValueChanges(t *testing.T) {
	width := func(rx, tx, used, cpu float64) [3]int {
		bar := Format(&client.Snapshot{Metrics: map[string]client.Metric{
			client.CPUPercent:    {Value: cpu},
			client.CPUCount:      {Value: 1},
			client.RAMUsed:       {Value: used},
			client.RAMTotal:      {Value: 16 * (1 << 30)},
			client.NetworkRxRate: {Value: rx},
			client.NetworkTxRate: {Value: tx},
		}})
		return [3]int{len([]rune(bar.CPU)), len([]rune(bar.Mem)), len([]rune(bar.Net))}
	}
	want := width(0, 0, 0, 0)
	for _, v := range []float64{7, 42, 999, 1000, 1023, 1024, 9.96 * 1024, 150 * 1024, 1023 * 1024, 5 << 20, 3 << 30} {
		assert.Equal(t, want, width(v, v/3, v*4, min(100, v/10)), "%v", v)
	}
}

func TestFixed(t *testing.T) {
	cases := map[float64]string{
		-5: "  0B", 0: "  0B", 7: "  7B", 42: " 42B", 999: "999B", 1000: "1.0K", 1024: "1.0K", 1536: "1.5K",
		10200: " 10K", 150 * 1024: "150K", 1023 * 1024: "1.0M", 5 * (1 << 20): "5.0M", 3.25 * (1 << 30): "3.2G",
	}
	for value, want := range cases {
		assert.Equal(t, want, Fixed(value), "%v", value)
	}
}

func TestDetailWritesEachReadingWithItsHistory(t *testing.T) {
	snap := snapshot()
	snap.Metrics[client.RAMSwapTotal] = client.Metric{Value: 2 * (1 << 30)}
	snap.Metrics[client.DiskUsed] = client.Metric{Value: 120 * (1 << 30)}
	snap.Metrics[client.DiskTotal] = client.Metric{Value: 480 * (1 << 30)}
	snap.Metrics[client.SystemUptime] = client.Metric{Value: 3*86400 + 4*3600 + 120}
	rows := map[string]Row{}
	var labels []string
	for _, row := range Detail(snap) {
		rows[row.Label] = row
		labels = append(labels, row.Label)
	}
	assert.Equal(t, []string{"CPU", "MEM", "SWAP", "GPU", "VRAM", "NET ↓", "NET ↑", "DISK", "LOAD", "UP"}, labels)
	assert.Equal(t, Row{Label: "CPU", History: []string{"5.2", "40.0", "12.4"}, Max: 100, Text: " 12% of 16 cores"}, rows["CPU"])
	assert.Equal(t, "3.2G of 16G  20%", rows["MEM"].Text)
	assert.Equal(t, float64(16*(1<<30)), rows["MEM"].Max)
	assert.Equal(t, "512M of 2.0G  25%", rows["SWAP"].Text)
	assert.Equal(t, " 88%", rows["GPU"].Text)
	assert.Equal(t, "6.0G of 24G  25%", rows["VRAM"].Text)
	assert.Equal(t, "1.2K/s", rows["NET ↓"].Text)
	assert.Equal(t, "300B/s", rows["NET ↑"].Text)
	assert.Equal(t, "120G of 480G  25%", rows["DISK"].Text)
	assert.Equal(t, "0.52 0.40 0.31", rows["LOAD"].Text)
	assert.Equal(t, Row{Label: "UP", Text: "3d 4h"}, rows["UP"])
}

func TestUptime(t *testing.T) {
	assert.Equal(t, "0m", Uptime(59))
	assert.Equal(t, "12m", Uptime(12*60+5))
	assert.Equal(t, "4h 12m", Uptime(4*3600+12*60))
	assert.Equal(t, "3d 0h", Uptime(3*86400+59*60))
}

func TestFormatLeavesOutWhatTheHostDoesNotOffer(t *testing.T) {
	assert.Equal(t, Bar{}, Format(nil))
	assert.Equal(t, Bar{}, Format(&client.Snapshot{Metrics: map[string]client.Metric{}}))
	assert.Empty(t, Detail(nil))
	assert.Empty(t, Detail(&client.Snapshot{Metrics: map[string]client.Metric{}}))

	partial := &client.Snapshot{Metrics: map[string]client.Metric{
		client.RAMUsed:                     {Value: 1},
		client.NetworkRxRate:               {Value: 1},
		client.GPUMetric(0, "utilization"): {Value: 3},
		client.SystemLoad1m:                {Value: 2},
	}}
	bar := Format(partial)
	assert.Empty(t, bar.Mem, "used without a total says nothing")
	assert.Empty(t, bar.Net)
	assert.Equal(t, "  3%", bar.GPU)
	var labels []string
	for _, row := range Detail(partial) {
		labels = append(labels, row.Label+" "+row.Text)
	}
	assert.Equal(t, []string{"GPU   3%", "NET ↓   1B/s", "LOAD 2.00"}, labels)
}

func TestTheProcessorLoadIsAShareOfTheWholeHost(t *testing.T) {
	busy := &client.Snapshot{Metrics: map[string]client.Metric{
		client.CPUPercent: {Value: 1500, History: []float64{1600, 0}},
		client.CPUCount:   {Value: 16},
	}}
	bar := Format(busy)
	assert.Equal(t, " 94%", bar.CPU)
	assert.Equal(t, []string{"0.0", "100.0", "93.8"}, bar.CPUHistory, "the graph's scale ends at a hundred")

	uncounted := &client.Snapshot{Metrics: map[string]client.Metric{client.CPUPercent: {Value: 1500}}}
	assert.Empty(t, Format(uncounted).CPU, "a load with no processor count has no scale")
}

func TestBytes(t *testing.T) {
	cases := map[float64]string{
		-5: "0B", 0: "0B", 999: "999B", 1024: "1.0K", 1536: "1.5K", 150 * 1024: "150K", 10200: "10K",
		5 * (1 << 20): "5.0M", 3.25 * (1 << 30): "3.2G", 2 * (1 << 40): "2.0T", 1 << 60: "1024P",
	}
	for value, want := range cases {
		assert.Equal(t, want, Bytes(value), "%v", value)
	}
}

func TestTheSamplerReadsThisHost(t *testing.T) {
	var lines []string
	s := Start(func(format string, args ...any) { lines = append(lines, format) })
	defer s.Close()
	// The load needs a second sample to measure against the first.
	require.Eventually(t, func() bool { return s.Bar().CPU != "" }, 20*time.Second, 10*time.Millisecond)
	bar := s.Bar()
	assert.NotEmpty(t, bar.CPUHistory)
	assert.NotEmpty(t, bar.Mem)
	assert.NotEmpty(t, s.Detail())

	s.Close()
	assert.Nil(t, s.Read())
	assert.Equal(t, Bar{}, s.Bar())
	assert.Empty(t, s.Detail())
}

func TestClosingASamplerBeforeItStarted(t *testing.T) {
	s := Start(func(string, ...any) {})
	s.Close()
	// The start that was under way finds the sampler closed and stops what it started.
	time.Sleep(50 * time.Millisecond)
	assert.Nil(t, s.Read())
}
