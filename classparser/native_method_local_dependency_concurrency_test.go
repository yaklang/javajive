package javaclassparser

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// Start fresh root, local and foreign-member reads together. A dependency found
// only in the local body must select the same atomic transaction before any
// ordinary family can publish or wait recursively on the other family.
func TestNativeSourceTransactionMethodLocalColdReadersCommitOneScope(t *testing.T) {
	files := nativeCompileClasses(t, nativeLocalOnlyDependencyCycleFixture)
	nativeLocalOnlyDependencyRemoveOuterHints(t, files, "LocalOnlyAlpha", "LocalOnlyBeta")
	names := []string{"LocalOnlyAlpha", "LocalOnlyBeta", "LocalOnlyAlpha$1Entry", "LocalOnlyBeta$Value"}
	for _, first := range names {
		t.Run(first, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			start := make(chan struct{})
			failures := make(chan string, 16)
			var wg sync.WaitGroup
			for i := 0; i < 16; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					n := names[i%len(names)]
					if i == 0 {
						n = first
					}
					source, e := z.ReadFile(n + ".class")
					if e != nil || strings.Contains(string(source), DecompileStubMarker) {
						failures <- fmt.Sprintf("%s: %v", n, e)
						return
					}
					if n == "LocalOnlyAlpha" && !strings.Contains(string(source), "class Entry") {
						failures <- "root lost its local declaration"
					}
					if n == "LocalOnlyAlpha$1Entry" && !strings.Contains(string(source), "original method-local declaration regenerated") {
						failures <- "local emitted independently"
					}
				}(i)
			}
			close(start)
			done := make(chan struct{})
			go func() { wg.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(15 * time.Second):
				t.Fatal("cold local source dependency cycle did not complete")
			}
			close(failures)
			for e := range failures {
				t.Error(e)
			}
			root, e := Parse(append([]byte(nil), files["LocalOnlyAlpha.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			local, e := Parse(append([]byte(nil), files["LocalOnlyAlpha$1Entry.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			a, b := z.nativeMemberTransactionEntry(root), z.nativeMemberTransactionEntry(local)
			if a == nil || a != b || a.family == nil || a.family.methodLocals["LocalOnlyAlpha$1Entry"] == nil {
				t.Fatal("cold readers published distinct ownership receipts")
			}
		})
	}
}
