package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func compilePackFixture(t *testing.T, srcPath, className string) []byte {
	t.Helper()
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read fixture %s: %v", srcPath, err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, className+".java"), src, 0644); err != nil {
		t.Fatal(err)
	}
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Fatalf("javac required for seed contract: %v", err)
	}
	cmd := exec.Command(javac, "-encoding", "UTF-8", "--release", "8", className+".java")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("javac %s: %v\n%s", className, err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, className+".class"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestT24C06ExceptionBehaviorRegression(t *testing.T) {
	fixtures := filepath.Join("testdata", "task_contracts")
	for _, name := range []string{"FinallyOverride", "MonitorRelease"} {
		src := filepath.Join(fixtures, name+".java")
		raw := compilePackFixture(t, src, name)
		off, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
		if err != nil {
			t.Fatalf("%s decompile: %v", name, err)
		}
		obj, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range obj.Methods {
			var code *CodeAttribute
			for _, attr := range m.Attributes {
				if ca, ok := attr.(*CodeAttribute); ok {
					code = ca
					break
				}
			}
			if code == nil {
				continue
			}
			d := core.NewDecompiler(code.Code, nil)
			if err := d.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			d.ExceptionTable = make([]*core.ExceptionTableEntry, 0, len(code.ExceptionTable))
			for _, e := range code.ExceptionTable {
				d.ExceptionTable = append(d.ExceptionTable, &core.ExceptionTableEntry{
					StartPc: e.StartPc, EndPc: e.EndPc, HandlerPc: e.HandlerPc, CatchType: e.CatchType,
				})
			}
			g, err := d.BuildSemanticCFG()
			if err != nil {
				t.Fatalf("%s CFG: %v", name, err)
			}
			prod := core.ProductionExceptionEdges(g)
			indexed, _, err := core.IndexedExceptionEdges(core.MayThrowPCs(g), core.ExceptionTableAsRanges(d.ExceptionTable), nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(prod) != len(indexed) {
				t.Fatalf("T24-C06 %s production edges %d indexed %d", name, len(prod), len(indexed))
			}
			for i := range prod {
				if prod[i].ThrowPc != indexed[i].ThrowPc || prod[i].HandlerPc != indexed[i].HandlerPc || prod[i].HandlerOrder != indexed[i].HandlerOrder {
					t.Fatalf("T24-C06 %s edge %d %+v vs %+v", name, i, prod[i], indexed[i])
				}
			}
		}
		on, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
		if err != nil {
			t.Fatal(err)
		}
		if off.Status != on.Status || off.Source != on.Source {
			t.Fatalf("T24-C06 %s source/status changed without wiring a production constructor swap", name)
		}
	}
}
