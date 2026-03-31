package agent

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

// collectSysInfo gathers system metrics for the AGENT_INFO frame.
// Uses /proc on Linux for zero-dependency metrics collection.
func collectSysInfo() *protocol.AgentInfoPayload {
	info := &protocol.AgentInfoPayload{}

	switch runtime.GOOS {
	case "linux":
		collectLinuxInfo(info)
	case "darwin":
		collectDarwinInfo(info)
	}

	return info
}

// collectLinuxInfo reads system metrics from /proc on Linux.
func collectLinuxInfo(info *protocol.AgentInfoPayload) {
	// Memory from /proc/meminfo
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		parseMeminfo(data, info)
	}

	// Load averages from /proc/loadavg
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		parseLoadavg(data, info)
	}

	// Uptime from /proc/uptime
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		parseUptime(data, info)
	}

	// CPU from /proc/stat (simple snapshot — not a delta)
	if data, err := os.ReadFile("/proc/stat"); err == nil {
		parseCPUStat(data, info)
	}

	// Disk from statfs of root filesystem
	collectDiskUsage("/", info)
}

// collectDarwinInfo collects basic metrics on macOS.
// Uses the same /proc-like approach where possible, falls back gracefully.
func collectDarwinInfo(info *protocol.AgentInfoPayload) {
	// macOS doesn't have /proc, so we collect what we can portably
	// Uptime approximation: use Go's monotonic clock relative to process start
	info.Uptime = int64(time.Since(processStart).Seconds())

	// Disk usage of root
	collectDiskUsage("/", info)
}

// processStart is recorded at init for uptime approximation on non-Linux.
var processStart = time.Now()

// parseMeminfo extracts MemTotal and MemAvailable from /proc/meminfo.
func parseMeminfo(data []byte, info *protocol.AgentInfoPayload) {
	var memTotal, memAvailable uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		// Values in /proc/meminfo are in kB
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			memTotal = val * 1024
		case strings.HasPrefix(line, "MemAvailable:"):
			memAvailable = val * 1024
		}
	}
	info.MemTotal = memTotal
	if memTotal > 0 && memAvailable <= memTotal {
		info.MemUsed = memTotal - memAvailable
	}
}

// parseLoadavg extracts 1, 5, and 15 minute load averages.
func parseLoadavg(data []byte, info *protocol.AgentInfoPayload) {
	fields := strings.Fields(string(data))
	if len(fields) >= 3 {
		info.LoadAvg1, _ = strconv.ParseFloat(fields[0], 64)
		info.LoadAvg5, _ = strconv.ParseFloat(fields[1], 64)
		info.LoadAvg15, _ = strconv.ParseFloat(fields[2], 64)
	}
}

// parseUptime extracts system uptime in seconds.
func parseUptime(data []byte, info *protocol.AgentInfoPayload) {
	fields := strings.Fields(string(data))
	if len(fields) >= 1 {
		up, err := strconv.ParseFloat(fields[0], 64)
		if err == nil {
			info.Uptime = int64(up)
		}
	}
}

// parseCPUStat computes a rough CPU usage percentage from /proc/stat.
// This is a snapshot, not a delta — shows overall since boot.
func parseCPUStat(data []byte, info *protocol.AgentInfoPayload) {
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			break
		}

		// fields: cpu user nice system idle [iowait irq softirq steal]
		var total, idle uint64
		for i := 1; i < len(fields); i++ {
			v, err := strconv.ParseUint(fields[i], 10, 64)
			if err != nil {
				continue
			}
			total += v
			if i == 4 { // idle is the 4th value (0-indexed field 4)
				idle = v
			}
		}

		if total > 0 {
			info.CPUPercent = float64(total-idle) / float64(total) * 100
		}
		break
	}
}
