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
		client.CPUPercent:                  {Value: 12.4, History: []float64{40, 5.25}},
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
	bar := Format(snapshot(), false)
	assert.Equal(t, Bar{
		CPU: "12%", CPUHistory: []string{"5.2", "40.0", "12.4"}, Mem: "3.2G/16G", Net: "↓1.2K ↑300B",
		Load: "0.52", GPU: "88% 6.0G",
	}, bar)
}

func TestFormatInDetail(t *testing.T) {
	bar := Format(snapshot(), true)
	assert.Equal(t, "12% of 16", bar.CPU)
	assert.Equal(t, "3.2G/16G swap 512M", bar.Mem)
	assert.Equal(t, "0.52 0.40 0.31", bar.Load)
	assert.Equal(t, "88% 6.0G/24G", bar.GPU)
}

func TestFormatLeavesOutWhatTheHostDoesNotOffer(t *testing.T) {
	assert.Equal(t, Bar{}, Format(nil, false))
	assert.Equal(t, Bar{}, Format(&client.Snapshot{Metrics: map[string]client.Metric{}}, true))

	partial := &client.Snapshot{Metrics: map[string]client.Metric{
		client.RAMUsed:                     {Value: 1},
		client.NetworkRxRate:               {Value: 1},
		client.GPUMetric(0, "utilization"): {Value: 3},
		client.SystemLoad1m:                {Value: 2},
	}}
	bar := Format(partial, true)
	assert.Empty(t, bar.Mem, "used without a total says nothing")
	assert.Empty(t, bar.Net)
	assert.Equal(t, "3%", bar.GPU)
	assert.Equal(t, "2.00", bar.Load)
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
	require.Eventually(t, func() bool { return s.Bar(false).CPU != "" }, 20*time.Second, 10*time.Millisecond)
	bar := s.Bar(false)
	assert.NotEmpty(t, bar.CPUHistory)
	assert.NotEmpty(t, bar.Mem)
	assert.NotEmpty(t, bar.Load)

	s.Close()
	assert.Nil(t, s.Read())
	assert.Equal(t, Bar{}, s.Bar(true))
}

func TestClosingASamplerBeforeItStarted(t *testing.T) {
	s := Start(func(string, ...any) {})
	s.Close()
	// The start that was under way finds the sampler closed and stops what it started.
	time.Sleep(50 * time.Millisecond)
	assert.Nil(t, s.Read())
}
