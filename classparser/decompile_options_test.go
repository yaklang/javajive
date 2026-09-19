package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/classes"
	"os"
	"strings"
	"testing"
)

func TestDecompilePolicyIsolation(t *testing.T) {
	raw, err := classes.FS.ReadFile("LongTest.class")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		mode := mode
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			first, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode})
			if err != nil {
				t.Fatal(err)
			}
			if first.Status != "complete" {
				t.Fatalf("%+v", first)
			}
			for i := 0; i < 4; i++ {
				next, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode})
				if err != nil || next.Source != first.Source || next.InputHash != first.InputHash || next.Mode != mode {
					t.Fatalf("request leaked: %+v %v", next, err)
				}
			}
			if mode == Precision && len(first.RulesApplied) != 0 {
				t.Fatal("precision applied compatibility rules")
			}
		})
	}
}
func TestDecompilePolicyFailures(t *testing.T) {
	for _, options := range []DecompileOptions{{Mode: "invalid"}, {MaxAnalysisUpdates: -1}, {}} {
		r, err := DecompileWithOptions([]byte{0xca, 0xfe}, options)
		if err == nil || r.Status != "invalid_input" {
			t.Fatalf("invalid input accepted: %+v %v", r, err)
		}
	}
	raw, err := classes.FS.ReadFile("LongTest.class")
	if err != nil {
		t.Fatal(err)
	}
	r, err := DecompileWithOptions(raw, DecompileOptions{MaxAnalysisUpdates: 1})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "partial" || len(r.StubMethods) == 0 {
		t.Fatalf("budget accepted as complete: %+v", r)
	}
	found := false
	for _, d := range r.Diagnostics {
		if strings.Contains(d.Message, "analysis_budget_exceeded") {
			found = true
		}
	}
	if !found {
		t.Fatalf("budget reason lost: %+v", r.Diagnostics)
	}
}
func TestSourceRewriteCycle(t *testing.T) {
	report := &DecompileResult{}
	d := &ClassObjectDumper{options: DecompileOptions{Mode: Compatibility}, report: report}
	first := d.sourceRewrite("a", "class_source", "A", func(string) string { return "B" })
	next := d.sourceRewrite("b", "class_source", first, func(string) string { return "A" })
	if next != "B" || len(report.RulesApplied) != 1 || report.Diagnostics[len(report.Diagnostics)-1].Code != "rewrite_cycle" {
		t.Fatalf("cycle accepted: %s %+v", next, report)
	}
}

// Retirement checks keep positive behavior-bearing structure on both sides of
// a legacy source switch; equality alone cannot make an empty/stub output pass.
func assertDecompileBothPreserve(t *testing.T, path, flag string, needles ...string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "1"} {
		t.Setenv(flag, value)
		source, err := Decompile(raw)
		if err != nil {
			t.Fatal(err)
		}
		for _, needle := range needles {
			if !strings.Contains(source, needle) {
				t.Errorf("%s=%q missing %q", flag, value, needle)
			}
		}
	}
}

func TestSyntheticCatchNamesAreRequestLocal(t *testing.T) {
	for _, file := range []string{"BuilderCallExpression.class", "EAN13Writer.class"} {
		raw, err := os.ReadFile("testdata/regression/" + file)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 4; i++ {
			t.Run(file, func(t *testing.T) {
				t.Parallel()
				next, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
				if err != nil || next.Source != expected.Source {
					t.Errorf("synthetic name depends on request order: %v", err)
				}
			})
		}
	}
}
