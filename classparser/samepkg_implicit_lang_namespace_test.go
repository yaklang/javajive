package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"os"
	"testing"
)

func TestImplicitLangNameNeedsOriginalSamePackageDeclarationIdentity(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/SamePkgFQSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"default package", "named package", "missing declaration", "wrong namespace", "no resolver"} {
		t.Run(scenario, func(t *testing.T) {
			pkg := "scope"
			if scenario == "default package" {
				pkg = ""
			}
			wanted := "scope/String"
			if pkg == "" {
				wanted = "String"
			}
			d := &ClassObjectDumper{obj: obj, PackageName: pkg, options: DecompileOptions{EnvSnapshot: map[string]string{}}, FuncCtx: &class_context.ClassContext{}}
			d.FuncCtx.InvocationMetadata = func(name string) (callbinding.Class, bool) {
				if name != wanted {
					return callbinding.Class{}, false
				}
				if scenario == "missing declaration" {
					return callbinding.Class{}, false
				}
				if scenario == "wrong namespace" {
					return callbinding.Class{Name: "other/String"}, true
				}
				return callbinding.Class{Name: name}, true
			}
			if scenario == "no resolver" {
				d.FuncCtx.InvocationMetadata = nil
			}
			got := d.computeSamePkgFQNames()["String"]
			want := scenario == "default package" || scenario == "named package"
			if got != want {
				t.Fatalf("qualified=%v want=%v", got, want)
			}
		})
	}
}
