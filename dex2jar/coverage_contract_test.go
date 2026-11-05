package dex2jar_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

type coverageBaseline struct {
	SchemaVersion    int `json:"schema_version"`
	Module           string
	MeasuredRevision string `json:"measured_revision"`
	Command          string
	Total            coverageCount
	Packages         map[string]coverageCount
	ZeroCovered      []string `json:"zero_covered_functions"`
}

type coverageCount struct {
	Covered    int
	Statements int
}

func TestCoverageBaselineContract(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repository := filepath.Dir(filepath.Dir(filename))
	data, err := os.ReadFile(filepath.Join(repository, "dex2jar", "coverage_baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	var baseline coverageBaseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		t.Fatal(err)
	}
	if baseline.SchemaVersion != 1 || baseline.Module != "github.com/yaklang/javajive" || len(baseline.MeasuredRevision) != 40 || baseline.Command == "" {
		t.Fatalf("invalid baseline metadata: %#v", baseline)
	}
	wantPackages := []string{
		"github.com/yaklang/javajive/dex2jar",
		"github.com/yaklang/javajive/dex2jar/input",
		"github.com/yaklang/javajive/dex2jar/ir",
		"github.com/yaklang/javajive/dex2jar/lower",
		"github.com/yaklang/javajive/dex2jar/report",
		"github.com/yaklang/javajive/internal/dexparser",
	}
	gotPackages := make([]string, 0, len(baseline.Packages))
	total := coverageCount{}
	for name, count := range baseline.Packages {
		gotPackages = append(gotPackages, name)
		if count.Covered < 0 || count.Statements <= 0 || count.Covered > count.Statements {
			t.Fatalf("invalid package baseline %q: %#v", name, count)
		}
		total.Covered += count.Covered
		total.Statements += count.Statements
	}
	sort.Strings(gotPackages)
	if !equalStrings(gotPackages, wantPackages) {
		t.Fatalf("package baselines = %q, want %q", gotPackages, wantPackages)
	}
	if total != baseline.Total {
		t.Fatalf("package sum = %#v, total = %#v", total, baseline.Total)
	}
	if !sort.StringsAreSorted(baseline.ZeroCovered) {
		t.Fatal("zero_covered_functions must be sorted")
	}
	seen := make(map[string]bool, len(baseline.ZeroCovered))
	for _, function := range baseline.ZeroCovered {
		if function == "" || seen[function] {
			t.Fatalf("invalid zero-covered function %q", function)
		}
		seen[function] = true
	}

	command := exec.Command("python3", "-I", filepath.Join(repository, "scripts", "check_dex_coverage.py"), "--self-test")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("coverage checker self-test: %v\n%s", err, output)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
