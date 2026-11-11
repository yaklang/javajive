package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestJoinRuleChainExactOutput(t *testing.T) {
	if got, want := joinRuleChain([]string{"first", "second", "third"}), "first->second->third"; got != want {
		t.Fatalf("joinRuleChain() = %q, want %q", got, want)
	}
	if got := joinRuleChain(nil); got != "" {
		t.Fatalf("joinRuleChain(nil) = %q", got)
	}
}

func BenchmarkJoinRuleChainBuilder(b *testing.B) {
	chain := make([]string, 4096)
	for i := range chain {
		chain[i] = "rewriteRule"
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if got := joinRuleChain(chain); len(got) == 0 {
			b.Fatal("empty output")
		}
	}
}

func TestSourceRewritePrecisionOuterGateSemantics(t *testing.T) {
	called := false
	d := &ClassObjectDumper{options: DecompileOptions{Mode: Precision}}
	got := d.sourceRewrite("test", "class_source", "source", func(string) string {
		called = true
		return "changed"
	})
	if got != "source" || called {
		t.Fatalf("Precision sourceRewrite = %q, rule called=%v", got, called)
	}
}

func TestDumpMethodsPointerFastPathKeepsDescriptor(t *testing.T) {
	raw, err := os.ReadFile("../test/cross/testdata/irreducible/original/DeadSecondaryEntry.class")
	if err != nil {
		t.Fatal(err)
	}
	result, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.StubMethods) != 0 {
		t.Fatalf("pointer fast path stubbed methods: %v diagnostics=%+v", result.StubMethods, result.Diagnostics)
	}
	if result.Status != "complete" {
		t.Fatalf("status=%q diagnostics=%+v", result.Status, result.Diagnostics)
	}
	if !strings.Contains(result.Source, "int run(int") {
		t.Fatalf("run(II)I descriptor was not rendered:\n%s", result.Source)
	}
}
