package javaclassparser

import (
	"context"
	"flag"
	"os"
	"reflect"
	"regexp"
	"testing"

	"github.com/yaklang/javajive/classparser/classes"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestMain(m *testing.M) {
	flag.Parse()
	rewriteIRCaseRunFlag()
	os.Exit(m.Run())
}

func rewriteIRCaseRunFlag() {
	f := flag.Lookup("test.run")
	if f == nil {
		return
	}
	v := f.Value.String()
	if v == "" {
		return
	}
	dashC := regexp.MustCompile(`T(\d+)-C(\d+)`)
	dash := regexp.MustCompile(`T(\d+)-`)
	nv := dashC.ReplaceAllString(v, "T${1}_C${2}")
	nv = dash.ReplaceAllString(nv, "T${1}_")
	if nv != v {
		_ = flag.Set("test.run", nv)
	}
}

func TestT11(t *testing.T) {
	t.Run("T11-C01", TestT11_C01_ShadowDoesNotInterfere)
}

func TestT11_C01_ShadowDoesNotInterfere(t *testing.T) {
	raw, err := os.ReadFile("testdata/invisible_anno.class")
	if err != nil {
		raw, err = classes.FS.ReadFile("LongTest.class")
		if err != nil {
			t.Fatal(err)
		}
	}
	off, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
	if err != nil {
		t.Fatal(err)
	}
	work := workbudget.New(context.Background(), Limits{})
	on, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, EnableShadowIR: true, Work: work})
	if err != nil {
		t.Fatal(err)
	}
	if off.Source != on.Source {
		t.Fatal("EnableShadowIR changed source")
	}
	if off.Status != on.Status {
		t.Fatalf("status %s vs %s", off.Status, on.Status)
	}
	if !reflect.DeepEqual(off.RulesApplied, on.RulesApplied) {
		t.Fatalf("rules %v vs %v", off.RulesApplied, on.RulesApplied)
	}
	if off.ShadowIRHash != "" || off.ShadowIRVersion != 0 {
		t.Fatalf("shadow records present when disabled: %+v", off)
	}
	if on.ShadowIRHash == "" || on.ShadowIRVersion != methodir.SnapshotVersion {
		t.Fatalf("missing shadow records when enabled: hash=%q ver=%d status=%s", on.ShadowIRHash, on.ShadowIRVersion, on.Status)
	}
	if on.InputHash != off.InputHash {
		t.Fatal("input hash diverged")
	}
	if work.Used(workbudget.CounterAnalysisUpdates) == 0 {
		t.Fatal("shadow SSA/lowering work did not use the request analysis counter")
	}
}

func TestEnableShadowIR_DefaultOff(t *testing.T) {
	raw, err := os.ReadFile("testdata/invisible_anno.class")
	if err != nil {
		t.Fatal(err)
	}
	r, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
	if err != nil {
		t.Fatal(err)
	}
	if r.ShadowIRHash != "" || r.ShadowIRVersion != 0 {
		t.Fatalf("default path leaked shadow IR: %+v", r)
	}
}

func TestShadowIRRunsFrameSSAAndLowering(t *testing.T) {
	_, classes := t04CompileRun(t, "8", "ShadowPipelineMain", map[string]string{
		"ShadowPipelineMain.java": `public final class ShadowPipelineMain {
			static int choose(int value) { return value < 0 ? -value : value; }
			public static void main(String[] args) { System.out.println(choose(args.length)); }
		}`,
	})
	raw := classes["ShadowPipelineMain"]
	off, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
	if err != nil {
		t.Fatal(err)
	}
	work := workbudget.New(context.Background(), Limits{})
	on, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, EnableShadowIR: true, Work: work})
	if err != nil {
		t.Fatal(err)
	}
	if on.Source != off.Source || on.Status != off.Status {
		t.Fatalf("shadow pipeline changed printer output/status: source=%t status=%q/%q", on.Source != off.Source, on.Status, off.Status)
	}
	if on.ShadowIRHash == "" || on.ShadowIRVersion != methodir.SnapshotVersion {
		t.Fatalf("missing pipeline observation: hash=%q version=%d shadow=%+v", on.ShadowIRHash, on.ShadowIRVersion, on.Shadow)
	}
	for _, observation := range on.Shadow {
		if observation.Status == "ok" && observation.Hash != "" {
			return
		}
	}
	t.Fatalf("no method completed frame transfer, SSA construction, and phi lowering: %+v", on.Shadow)
}
