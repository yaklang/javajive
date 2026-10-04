//go:build darwin || linux

package performance_baseline

import (
	"runtime"
	"syscall"
)

func peakRSSBytes() (int64, string, error) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0, "unavailable_getrusage", err
	}
	if runtime.GOOS == "linux" {
		return ru.Maxrss * 1024, "rusage_maxrss_linux_kilobytes_to_bytes", nil
	}
	return ru.Maxrss, "rusage_maxrss_darwin_bytes", nil
}
