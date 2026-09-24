package jdecenv

import (
	"sync"
	"testing"
	"time"
)

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
