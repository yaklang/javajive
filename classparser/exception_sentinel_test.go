package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestExceptionSentinelDegradeIsLoadBearing pins the caught-throwable sentinel degradation
// (JDEC_EXCEPTION_SENTINEL_DEGRADE_OFF). A try/finally (or synchronized-region) whose handler stack
// value cannot be bound to a real local renders as a bare `varN = Exception;` -- accepted by the ANTLR
// syntax net but rejected by javac with "cannot find symbol" (guava Monitor.enterWhen /
// enterWhenUninterruptibly). The seed is guava's real Monitor.class. With the fix ON the leaked
// sentinel is promoted to a full degradation trigger, so the offending method degrades to an honest
// compiling stub and NO `= Exception;` survives into the output; with the kill-switch OFF the broken
// assignment reappears (proving the guard is load-bearing, not cosmetic).
func TestExceptionSentinelDegradeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ExceptionSentinelSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_EXCEPTION_SENTINEL_DEGRADE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if strings.Contains(on, "= Exception;") || strings.Contains(on, "= Exception\n") {
		t.Errorf("fix ON: broken caught-throwable sentinel `= Exception;` leaked into output:\n%s", on)
	}

	t.Setenv("JDEC_EXCEPTION_SENTINEL_DEGRADE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "= Exception;") {
		t.Errorf("fix OFF: expected the broken `= Exception;` sentinel to reappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
