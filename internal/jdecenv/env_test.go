package jdecenv

import (
	"sync"
	"testing"
	"time"
)

func TestCaptureProcessSeesSetenv(t *testing.T) {
	t.Setenv("JDEC_CAPTURE_PROCESS", "once")
	snap := CaptureProcess()
	if snap["JDEC_CAPTURE_PROCESS"] != "once" {
		t.Fatalf("snapshot missed host env: %q", snap["JDEC_CAPTURE_PROCESS"])
	}
	t.Setenv("JDEC_CAPTURE_PROCESS", "later")
	if snap["JDEC_CAPTURE_PROCESS"] != "once" {
		t.Fatal("snapshot followed a later Setenv")
	}
	if Get("JDEC_CAPTURE_PROCESS") != "later" {
		t.Fatal("unbound Get stopped reading the live environment")
	}
}

func TestLookupPreservesExplicitSnapshotAndLivePolicy(t *testing.T) {
	t.Setenv("JDEC_LOOKUP", "host")
	live := Lookup()
	_ = Run(map[string]string{"JDEC_LOOKUP": "outer"}, func() error {
		outer := Lookup()
		if outer("JDEC_LOOKUP") != "outer" || outer("JDEC_MISSING") != "" {
			t.Fatal("captured snapshot must be closed and keep the outer value")
		}
		_ = Run(map[string]string{"JDEC_LOOKUP": "inner"}, func() error {
			if Lookup()("JDEC_LOOKUP") != "inner" || outer("JDEC_LOOKUP") != "outer" || live("JDEC_LOOKUP") != "host" {
				t.Fatal("nested request changed a previously captured policy")
			}
			return nil
		})
		_ = RunLive(func() error {
			lookup := Lookup()
			t.Setenv("JDEC_LOOKUP", "changed")
			if lookup("JDEC_LOOKUP") != "changed" || outer("JDEC_LOOKUP") != "outer" {
				t.Fatal("live sentinel was frozen or snapshot leaked into live lookup")
			}
			return nil
		})
		if outer("JDEC_LOOKUP") != Get("JDEC_LOOKUP") {
			t.Fatal("outer request was not restored")
		}
		return nil
	})
	if live("JDEC_LOOKUP") != "changed" || Lookup()("JDEC_LOOKUP") != "changed" {
		t.Fatal("unbound lookups must remain live")
	}
}

func TestNestedRunRestoresOuter(t *testing.T) {
	t.Setenv("JDEC_NESTED", "host")
	if Get("JDEC_NESTED") != "host" {
		t.Fatal("unbound Get should read host")
	}
	err := Run(map[string]string{"JDEC_NESTED": "outer"}, func() error {
		if Get("JDEC_NESTED") != "outer" {
			t.Fatalf("before inner: got %q", Get("JDEC_NESTED"))
		}
		innerErr := Run(map[string]string{"JDEC_NESTED": "inner"}, func() error {
			if Get("JDEC_NESTED") != "inner" {
				t.Fatalf("inner: got %q", Get("JDEC_NESTED"))
			}
			return nil
		})
		if innerErr != nil {
			return innerErr
		}
		if Get("JDEC_NESTED") != "outer" {
			t.Fatalf("after inner: got %q want outer (host leak)", Get("JDEC_NESTED"))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if Get("JDEC_NESTED") != "host" {
		t.Fatalf("after outer: got %q want host", Get("JDEC_NESTED"))
	}
}

func TestNestedRunPanicRestoresOuter(t *testing.T) {
	t.Setenv("JDEC_NESTED", "host")
	err := Run(map[string]string{"JDEC_NESTED": "outer"}, func() error {
		func() {
			defer func() { _ = recover() }()
			_ = Run(map[string]string{"JDEC_NESTED": "inner"}, func() error {
				panic("inner boom")
			})
		}()
		if Get("JDEC_NESTED") != "outer" {
			t.Fatalf("after inner panic: got %q want outer", Get("JDEC_NESTED"))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGoInheritsSnapshot(t *testing.T) {
	t.Setenv("JDEC_NESTED", "host")
	ch := make(chan string, 1)
	err := Run(map[string]string{"JDEC_NESTED": "outer"}, func() error {
		Go(func() { ch <- Get("JDEC_NESTED") })
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := <-ch
	if got != "outer" {
		t.Fatalf("child goroutine Get=%q want outer (not host)", got)
	}
}

func TestRawGoroutineDoesNotInherit(t *testing.T) {
	t.Setenv("JDEC_NESTED", "host")
	ch := make(chan string, 1)
	err := Run(map[string]string{"JDEC_NESTED": "outer"}, func() error {
		go func() { ch <- Get("JDEC_NESTED") }()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := <-ch
	if got != "host" {
		t.Fatalf("raw goroutine should read host, got %q", got)
	}
}

func TestNestedRunRace(t *testing.T) {
	t.Setenv("JDEC_NESTED", "host")
	var wg sync.WaitGroup
	errCh := make(chan string, 200)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = Run(map[string]string{"JDEC_NESTED": "outer"}, func() error {
				if Get("JDEC_NESTED") != "outer" {
					errCh <- "outer missing"
				}
				_ = Run(map[string]string{"JDEC_NESTED": "inner"}, func() error {
					if Get("JDEC_NESTED") != "inner" {
						errCh <- "inner missing"
					}
					return nil
				})
				if Get("JDEC_NESTED") != "outer" {
					errCh <- "outer not restored: " + Get("JDEC_NESTED")
				}
				return nil
			})
		}()
	}
	wg.Wait()
	close(errCh)
	for m := range errCh {
		t.Error(m)
	}
}

func TestUnboundGetDoesNotRequireBind(t *testing.T) {
	t.Setenv("JDEC_X", "host")
	if Get("JDEC_X") != "host" {
		t.Fatal("unbound Get must read process env")
	}
	const n = 200000
	start := time.Now()
	for i := 0; i < n; i++ {
		if Get("JDEC_X") != "host" {
			t.Fatal("unbound Get lost host")
		}
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("unbound Get too slow (%s); must not parse runtime.Stack", time.Since(start))
	}
}

func TestGetFastPathPreservesClosedAndLiveSemantics(t *testing.T) {
	const key = "JDEC_FASTPATH_SEMANTICS"
	t.Setenv(key, "host")

	// A bound snapshot that does not override the key is closed.
	if err := Run(map[string]string{}, func() error {
		if got := Get(key); got != "" {
			t.Fatalf("closed snapshot leaked host value %q", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// A snapshot that does override it wins.
	if err := Run(map[string]string{key: "snapshot"}, func() error {
		if got := Get(key); got != "snapshot" {
			t.Fatalf("snapshot value = %q", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// A live sentinel reads the process environment.
	if err := RunLive(func() error {
		if got := Get(key); got != "host" {
			t.Fatalf("live sentinel = %q, want host", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Nested snapshots must restore the outer closed/live choice.
	if err := Run(map[string]string{key: "outer"}, func() error {
		if err := Run(map[string]string{}, func() error {
			if got := Get(key); got != "" {
				t.Fatalf("inner closed snapshot = %q", got)
			}
			return nil
		}); err != nil {
			return err
		}
		if got := Get(key); got != "outer" {
			t.Fatalf("outer snapshot was not restored: %q", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

var benchmarkGetSink string

func BenchmarkGetWithoutActiveOverride(b *testing.B) {
	const key = "JDEC_BENCH_ABSENT_KEY"
	b.Setenv(key, "")
	if err := Run(map[string]string{"PATH": "/usr/bin"}, func() error {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchmarkGetSink = Get(key)
		}
		return nil
	}); err != nil {
		b.Fatal(err)
	}
}

func BenchmarkGetWithActiveOverride(b *testing.B) {
	const key = "JDEC_BENCH_OVERRIDE_KEY"
	if err := Run(map[string]string{key: "1"}, func() error {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchmarkGetSink = Get(key)
		}
		return nil
	}); err != nil {
		b.Fatal(err)
	}
}

func TestNilRunDoesNotPush(t *testing.T) {
	t.Setenv("JDEC_NESTED", "host")
	_ = Run(map[string]string{"JDEC_NESTED": "outer"}, func() error {
		_ = Run(nil, func() error {
			if Get("JDEC_NESTED") != "outer" {
				t.Fatalf("nil Run cleared outer: %q", Get("JDEC_NESTED"))
			}
			return nil
		})
		return nil
	})
}

func TestRunLiveUsesHostThenRestoresOuter(t *testing.T) {
	t.Setenv("JDEC_NESTED", "host")
	err := Run(map[string]string{"JDEC_NESTED": "outer"}, func() error {
		if Get("JDEC_NESTED") != "outer" {
			t.Fatalf("before live: %q", Get("JDEC_NESTED"))
		}
		if err := RunLive(func() error {
			if Get("JDEC_NESTED") != "host" {
				t.Fatalf("live inner: got %q want host", Get("JDEC_NESTED"))
			}
			return nil
		}); err != nil {
			return err
		}
		if Get("JDEC_NESTED") != "outer" {
			t.Fatalf("after live: got %q want outer", Get("JDEC_NESTED"))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunLivePanicRestoresOuter(t *testing.T) {
	t.Setenv("JDEC_NESTED", "host")
	err := Run(map[string]string{"JDEC_NESTED": "outer"}, func() error {
		func() {
			defer func() { _ = recover() }()
			_ = RunLive(func() error {
				panic("live boom")
			})
		}()
		if Get("JDEC_NESTED") != "outer" {
			t.Fatalf("after live panic: got %q want outer", Get("JDEC_NESTED"))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
