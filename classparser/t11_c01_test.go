package javaclassparser

import (
	"flag"
	"os"
	"reflect"
	"regexp"
	"testing"

	"github.com/yaklang/javajive/classparser/classes"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
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
	on, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, EnableShadowIR: true})
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
