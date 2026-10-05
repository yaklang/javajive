package statements

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"testing"
)

func TestOriginalMonitorSourceWitnessInvalidatesEditedReceiver(t *testing.T) {
	for _, scenario := range []string{"original", "kind", "data", "unknown", "negative", "owner differs", "source receiver", "nil"} {
		t.Run(scenario, func(t *testing.T) {
			value := values.JavaNull
			entry := NewOriginalMonitorStatement("monitor_enter", value, 3, 3)
			switch scenario {
			case "kind":
				entry.Flag = "monitor_exit"
			case "data":
				entry.Data = nil
			case "unknown":
				entry = NewMiddleStatement("monitor_enter", value)
			case "negative":
				entry = NewOriginalMonitorStatement("monitor_enter", value, -1, -1)
			case "owner differs":
				entry = NewOriginalMonitorStatement("monitor_enter", value, 3, 4)
			case "nil":
				entry = nil
			}
			source := NewSynchronizedStatementFromMonitor(entry, nil)
			if scenario == "source receiver" {
				source.Argument = nil
			}
			pc, known := source.OriginalMonitorEnterPC()
			if known != (scenario == "original") || (known && pc != 3) {
				t.Fatalf("pc=%d known=%v", pc, known)
			}
		})
	}
	exit := NewOriginalMonitorStatement("monitor_exit", nil, 5, 3)
	if pc, owner, known := exit.OriginalMonitor(); !known || pc != 5 || owner != 3 {
		t.Fatal("release witness lost")
	}
	exit.Data = values.JavaNull
	if _, _, known := exit.OriginalMonitor(); known {
		t.Fatal("edited exit retained witness")
	}
	var missing *SynchronizedStatement
	if _, known := missing.OriginalMonitorEnterPC(); known {
		t.Fatal("nil source admitted")
	}
}
