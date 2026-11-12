package javaclassparser

import (
	"os"
	"testing"
)

// The loop-analysis cache is an internal reuse of a body/exit query that is
// otherwise recomputed for every container node. It must never change the
// rendered source: the kill switch makes the rewriter take the original
// recompute path, so both runs must agree byte for byte.
//
// These fixtures have loops and no exception table, which is exactly the
// configuration where the cache is enabled (protected retry boundaries mutate
// node state and keep it disabled).
func TestLoopAnalysisCacheMatchesUncachedDecompile(t *testing.T) {
	fixtures := []string{
		"testdata/regression/BareIfAdv.class",
		"testdata/regression/BoolArrayElementStoreSeed.class",
		"testdata/regression/AbstractLinkedList.class",
	}
	for _, path := range fixtures {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		t.Setenv("JDEC_LOOP_ANALYSIS_CACHE_OFF", "")
		cached, err := Decompile(data)
		if err != nil {
			t.Fatalf("%s cached: %v", path, err)
		}
		t.Setenv("JDEC_LOOP_ANALYSIS_CACHE_OFF", "1")
		uncached, err := Decompile(data)
		if err != nil {
			t.Fatalf("%s uncached: %v", path, err)
		}
		if cached != uncached {
			t.Fatalf("%s: cache changed the rendered source (cached=%d bytes, uncached=%d bytes)", path, len(cached), len(uncached))
		}
	}
}
