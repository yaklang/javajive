package performance_baseline

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestPeakRSSPlatformMeasurement(t *testing.T) {
	rss, source, err := peakRSSBytes()
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		if err != nil || rss <= 0 || strings.HasPrefix(source, "unavailable") {
			t.Fatalf("rss=%d source=%q err=%v", rss, source, err)
		}
	default:
		if err == nil || !strings.HasPrefix(source, "unavailable") {
			t.Fatalf("unsupported platform fabricated RSS: %d %q %v", rss, source, err)
		}
	}
}

func TestUnavailableRSSIsNull(t *testing.T) {
	raw, err := json.Marshal(childOut{RSSSource: "unavailable", OK: false, Error: "RSS unavailable"})
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["peak_rss_bytes"] != nil || out["ok"] != false {
		t.Fatalf("unavailable measurement represented as success: %s", raw)
	}
}
