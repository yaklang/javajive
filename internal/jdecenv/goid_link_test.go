package jdecenv

import (
	"sync"
	"testing"
)

func TestGoidFastMatchesStack(t *testing.T) {
	goidOnce.Do(calibrateGoidOffset)
	if !goidFastOK {
		t.Fatal("goid offset was not found")
	}
	var wg sync.WaitGroup
	errc := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if gid() != gidFromStack() {
				errc <- "mismatch"
			}
		}()
	}
	wg.Wait()
	close(errc)
	if _, ok := <-errc; ok {
		t.Fatal("fast goid does not match runtime.Stack")
	}
}
