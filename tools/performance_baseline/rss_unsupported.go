//go:build !darwin && !linux && !windows

package performance_baseline

import (
	"fmt"
	"runtime"
)

func peakRSSBytes() (int64, string, error) {
	return 0, "unavailable_" + runtime.GOOS, fmt.Errorf("peak RSS unsupported on %s", runtime.GOOS)
}
