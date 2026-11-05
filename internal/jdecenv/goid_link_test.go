package jdecenv

import (
	"sync"
	"testing"
)

func TestGoidStableAndUnique(t *testing.T) {
	const workers = 8
	var wg sync.WaitGroup
	ids := make(chan uint64, workers)
	release := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := gid()
			ids <- id
			<-release
			if gid() != id {
				t.Errorf("goroutine identity changed")
			}
		}()
	}
	seen := make(map[uint64]struct{}, workers)
	for i := 0; i < workers; i++ {
		id := <-ids
		if id == 0 {
			t.Fatal("goroutine identity is zero")
		}
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate live goroutine identity %d", id)
		}
		seen[id] = struct{}{}
	}
	close(release)
	wg.Wait()
}
