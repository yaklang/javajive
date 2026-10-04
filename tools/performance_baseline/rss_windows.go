package performance_baseline

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// PROCESS_MEMORY_COUNTERS uses SIZE_T (uintptr) for each byte count.
// https://learn.microsoft.com/windows/win32/api/psapi/ns-psapi-process_memory_counters
// PeakWorkingSetSize is the process's resident working-set high-water mark;
// it is not the pagefile/commit-charge counter.
type processMemoryCounters struct {
	Size, PageFaultCount                               uint32
	PeakWorkingSetSize, WorkingSetSize                 uintptr
	QuotaPeakPagedPoolUsage, QuotaPagedPoolUsage       uintptr
	QuotaPeakNonPagedPoolUsage, QuotaNonPagedPoolUsage uintptr
	PagefileUsage, PeakPagefileUsage                   uintptr
}

var getProcessMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

func peakRSSBytes() (int64, string, error) {
	const unavailable = "unavailable_windows_peak_working_set"
	if err := getProcessMemoryInfo.Find(); err != nil {
		return 0, unavailable, err
	}
	var counters processMemoryCounters
	counters.Size = uint32(unsafe.Sizeof(counters))
	ok, _, err := getProcessMemoryInfo.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&counters)), uintptr(counters.Size))
	if ok == 0 {
		return 0, unavailable, fmt.Errorf("GetProcessMemoryInfo: %w", err)
	}
	if counters.PeakWorkingSetSize == 0 || uint64(counters.PeakWorkingSetSize) > uint64(^uint64(0)>>1) {
		return 0, unavailable, fmt.Errorf("invalid peak working set: %d", counters.PeakWorkingSetSize)
	}
	return int64(counters.PeakWorkingSetSize), "windows_process_peak_working_set_bytes", nil
}
