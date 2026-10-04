package core

import (
	"testing"

	"github.com/yaklang/javajive/internal/jdecenv"
)

func TestRewriteTraceKeepsRequestGateAndFilters(t *testing.T) {
	t.Setenv("JDEC_TRACE_REWRITE_VAR", "1")
	for _, scenario := range []string{"closed unset", "unrelated trace", "enabled", "class miss", "method miss", "matching filters"} {
		t.Run(scenario, func(t *testing.T) {
			snap := map[string]string{}
			if scenario != "closed unset" && scenario != "unrelated trace" {
				snap["JDEC_TRACE_REWRITE_VAR"] = "1"
			}
			if scenario == "unrelated trace" {
				snap["JDEC_TRACE_VAR_TABLE"] = "1"
			}
			if scenario == "class miss" {
				snap["JDEC_TRACE_CLASS"] = "missing"
			}
			if scenario == "method miss" {
				snap["JDEC_TRACE_METHOD"] = "missing"
			}
			if scenario == "matching filters" {
				snap["JDEC_TRACE_CLASS"] = "Example"
				snap["JDEC_TRACE_METHOD"] = "run"
			}
			_ = jdecenv.Run(snap, func() error {
				want := scenario == "enabled" || scenario == "matching filters"
				if got := TraceRewriteVarEnabled("probe.Example", "run"); got != want {
					t.Fatalf("got=%v want=%v", got, want)
				}
				_ = jdecenv.Run(map[string]string{}, func() error {
					if TraceRewriteVarEnabled("probe.Example", "run") {
						t.Fatal("inner request inherited outer trace")
					}
					return nil
				})
				if got := TraceRewriteVarEnabled("probe.Example", "run"); got != want {
					t.Fatal("inner request changed outer filter")
				}
				return nil
			})
		})
	}
}
