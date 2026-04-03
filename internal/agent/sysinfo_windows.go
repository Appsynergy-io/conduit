//go:build windows

package agent

import (
	"syscall"
	"time"
	"unsafe"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procGlobalMemoryEx    = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetDiskFreeSpaceW = kernel32.NewProc("GetDiskFreeSpaceExW")
	procGetTickCount64    = kernel32.NewProc("GetTickCount64")

	pdh               = syscall.NewLazyDLL("pdh.dll")
	procPdhOpenQuery   = pdh.NewProc("PdhOpenQueryW")
	procPdhAddCounter  = pdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollectData = pdh.NewProc("PdhCollectQueryData")
	procPdhGetDouble   = pdh.NewProc("PdhGetFormattedCounterValue")
)

// memoryStatusEx matches the Windows MEMORYSTATUSEX structure.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// collectWindowsInfo gathers system metrics on Windows using kernel32 syscalls.
func collectWindowsInfo(info *protocol.AgentInfoPayload) {
	collectWindowsMemory(info)
	collectWindowsUptime(info)
	collectWindowsCPU(info)
	collectDiskUsage(`C:\`, info)
}

// collectWindowsMemory reads memory stats via GlobalMemoryStatusEx.
func collectWindowsMemory(info *protocol.AgentInfoPayload) {
	var mem memoryStatusEx
	mem.Length = uint32(unsafe.Sizeof(mem))

	ret, _, _ := procGlobalMemoryEx.Call(uintptr(unsafe.Pointer(&mem)))
	if ret == 0 {
		return
	}

	info.MemTotal = mem.TotalPhys
	info.MemUsed = mem.TotalPhys - mem.AvailPhys
}

// collectWindowsUptime reads system uptime via GetTickCount64.
func collectWindowsUptime(info *protocol.AgentInfoPayload) {
	ret, _, _ := procGetTickCount64.Call()
	if ret == 0 {
		// Fallback to process-relative uptime
		info.Uptime = int64(time.Since(processStart).Seconds())
		return
	}
	info.Uptime = int64(ret) / 1000 // milliseconds to seconds
}

// collectWindowsCPU attempts to read CPU usage via PDH performance counters.
// Falls back gracefully if PDH is unavailable.
func collectWindowsCPU(info *protocol.AgentInfoPayload) {
	if pdh.Load() != nil {
		return
	}

	var query uintptr
	ret, _, _ := procPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&query)))
	if ret != 0 {
		return
	}

	counterPath, _ := syscall.UTF16PtrFromString(`\Processor(_Total)\% Processor Time`)
	var counter uintptr
	ret, _, _ = procPdhAddCounter.Call(query, uintptr(unsafe.Pointer(counterPath)), 0, uintptr(unsafe.Pointer(&counter)))
	if ret != 0 {
		return
	}

	// Collect twice with a short delay for a delta-based reading
	procPdhCollectData.Call(query)
	time.Sleep(100 * time.Millisecond)
	ret, _, _ = procPdhCollectData.Call(query)
	if ret != 0 {
		return
	}

	// PDH_FMT_COUNTERVALUE: dwStatus (4 bytes) + padding (4 bytes) + doubleValue (8 bytes)
	type pdhValue struct {
		Status uint32
		_      uint32
		Value  float64
	}
	var value pdhValue
	const pdhFmtDouble = 0x00000200
	ret, _, _ = procPdhGetDouble.Call(counter, uintptr(pdhFmtDouble), 0, uintptr(unsafe.Pointer(&value)))
	if ret == 0 && value.Status == 0 {
		info.CPUPercent = value.Value
	}
}

// collectDiskUsage reads disk space via GetDiskFreeSpaceExW.
func collectDiskUsage(path string, info *protocol.AgentInfoPayload) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	ret, _, _ := procGetDiskFreeSpaceW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if ret == 0 {
		return
	}

	info.DiskTotal = totalBytes
	info.DiskUsed = totalBytes - totalFreeBytes
}
