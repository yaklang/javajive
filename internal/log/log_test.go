package log

import (
	"bytes"
	stdlog "log"
	"strings"
	"sync"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want Level
	}{
		{"debug", DebugLevel},
		{" DEBUG ", DebugLevel},
		{"info", InfoLevel},
		{"warn", WarnLevel},
		{"warning", WarnLevel},
		{"error", ErrorLevel},
		{"quiet", QuietLevel},
		{"off", QuietLevel},
		{"none", QuietLevel},
	}
	for _, tc := range cases {
		got, err := ParseLevel(tc.in)
		if err != nil {
			t.Fatalf("ParseLevel(%q) unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseLevel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	for _, bad := range []string{"", "   ", "verbose", "trace"} {
		if _, err := ParseLevel(bad); err == nil {
			t.Fatalf("ParseLevel(%q) expected error, got nil", bad)
		}
	}
}

func TestEnabledAndOutputSuppression(t *testing.T) {
	prev := GetLevel()
	prevOut := stdlog.Writer()
	defer func() {
		SetLevel(prev)
		stdlog.SetOutput(prevOut)
	}()

	var buf bytes.Buffer
	stdlog.SetOutput(&buf)

	SetLevel(WarnLevel)
	if Enabled(DebugLevel) || Enabled(InfoLevel) {
		t.Fatal("debug/info should be disabled at WarnLevel")
	}
	if !Enabled(WarnLevel) || !Enabled(ErrorLevel) {
		t.Fatal("warn/error should be enabled at WarnLevel")
	}

	Debugf("dbg %s", "ignored")
	Infof("info %s", "ignored")
	if buf.Len() != 0 {
		t.Fatalf("expected no output below WarnLevel, got %q", buf.String())
	}

	Warnf("warn %s", "shown")
	Errorf("err %s", "shown")
	got := buf.String()
	if !strings.Contains(got, "[WARN] warn shown") || !strings.Contains(got, "[ERROR] err shown") {
		t.Fatalf("unexpected log output: %q", got)
	}

	buf.Reset()
	SetLevel(QuietLevel)
	if Enabled(ErrorLevel) {
		t.Fatal("error should be disabled at QuietLevel")
	}
	Error("suppressed")
	Errorf("suppressed %d", 1)
	if buf.Len() != 0 {
		t.Fatalf("expected empty output at QuietLevel, got %q", buf.String())
	}
}

func TestConcurrentSetLevelAndLog(t *testing.T) {
	prev := GetLevel()
	prevOut := stdlog.Writer()
	defer func() {
		SetLevel(prev)
		stdlog.SetOutput(prevOut)
	}()

	var buf bytes.Buffer
	stdlog.SetOutput(&buf)

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				if id%2 == 0 {
					SetLevel(QuietLevel)
				} else {
					SetLevel(ErrorLevel)
				}
				Debugf("d %d", i)
				Infof("i %d", i)
				_ = Enabled(InfoLevel)
				_ = GetLevel()
			}
		}(g)
	}
	wg.Wait()
}
