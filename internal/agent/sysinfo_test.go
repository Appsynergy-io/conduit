package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCollectSysInfo(t *testing.T) {
	info := collectSysInfo()
	assert.NotNil(t, info)

	// On Linux CI, we should have real values
	// On other platforms, values may be zero but shouldn't panic
	t.Logf("CPU: %.1f%%", info.CPUPercent)
	t.Logf("Memory: %d / %d", info.MemUsed, info.MemTotal)
	t.Logf("Disk: %d / %d", info.DiskUsed, info.DiskTotal)
	t.Logf("Uptime: %ds", info.Uptime)
	t.Logf("Load: %.2f / %.2f / %.2f", info.LoadAvg1, info.LoadAvg5, info.LoadAvg15)
}

func TestParseMeminfo(t *testing.T) {
	data := []byte(`MemTotal:       16384000 kB
MemFree:         2048000 kB
MemAvailable:    8192000 kB
Buffers:          512000 kB
Cached:          4096000 kB
`)

	info := &struct {
		MemTotal uint64
		MemUsed  uint64
	}{}

	// Use the actual protocol type
	pInfo := collectSysInfo()
	parseMeminfo(data, pInfo)

	_ = info // unused, using pInfo directly
	assert.Equal(t, uint64(16384000*1024), pInfo.MemTotal)
	assert.Equal(t, uint64((16384000-8192000)*1024), pInfo.MemUsed)
}

func TestParseLoadavg(t *testing.T) {
	data := []byte("1.25 0.75 0.50 3/456 12345\n")

	pInfo := collectSysInfo()
	parseLoadavg(data, pInfo)

	assert.InDelta(t, 1.25, pInfo.LoadAvg1, 0.001)
	assert.InDelta(t, 0.75, pInfo.LoadAvg5, 0.001)
	assert.InDelta(t, 0.50, pInfo.LoadAvg15, 0.001)
}

func TestParseUptime(t *testing.T) {
	data := []byte("12345.67 98765.43\n")

	pInfo := collectSysInfo()
	parseUptime(data, pInfo)

	assert.Equal(t, int64(12345), pInfo.Uptime)
}

func TestParseCPUStat(t *testing.T) {
	// cpu  user nice system idle iowait irq softirq steal
	data := []byte(`cpu  100 10 50 840 0 0 0 0 0 0
cpu0 50 5 25 420 0 0 0 0 0 0
`)

	pInfo := collectSysInfo()
	parseCPUStat(data, pInfo)

	// total = 100+10+50+840 = 1000, idle = 840
	// CPU% = (1000-840)/1000 * 100 = 16%
	assert.InDelta(t, 16.0, pInfo.CPUPercent, 0.1)
}

func TestParseCPUStat_Empty(t *testing.T) {
	pInfo := collectSysInfo()
	parseCPUStat([]byte(""), pInfo)
	// Should not panic, CPUPercent stays at whatever collectSysInfo set it to
}
