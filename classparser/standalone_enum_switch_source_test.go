package javaclassparser

import (
	"context"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeEnumStandaloneProjectionRequiresOriginalClosedProtocol(t *testing.T) {
	files := nativeCompileClasses(t, `enum ProjectionMode{A,B} class ProjectionRoot{static int run(ProjectionMode mode){switch(mode){case A:return 19;default:return 7;}}}`)
	for _, variant := range []string{"original", "no resolver", "no helper", "no enum", "ordinary helper", "mutable table", "ordinary table field", "disabled", "archive lookup", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(files["ProjectionRoot.class"])
			if err != nil {
				t.Fatal(err)
			}
			resolverFiles := make(map[string][]byte, len(files))
			for name, raw := range files {
				resolverFiles[name] = raw
			}
			d := NewClassObjectDumper(root)
			d.options = DecompileOptions{Mode: Precision, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}}
			d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := resolverFiles[name+".class"]; return raw, ok }
			switch variant {
			case "no resolver":
				d.foldSiblingResolver = nil
			case "no helper":
				delete(resolverFiles, "ProjectionRoot$1.class")
			case "no enum":
				delete(resolverFiles, "ProjectionMode.class")
			case "ordinary helper", "mutable table", "ordinary table field":
				helper, err := Parse(files["ProjectionRoot$1.class"])
				if err != nil {
					t.Fatal(err)
				}
				if variant == "ordinary helper" {
					helper.AccessFlags &^= 0x1000
				}
				if variant == "mutable table" {
					helper.Fields[0].AccessFlags &^= 16
				}
				if variant == "ordinary table field" {
					helper.Fields[0].AccessFlags &^= 0x1000
				}
				resolverFiles["ProjectionRoot$1.class"] = helper.Bytes()
			case "disabled":
				d.options.EnvSnapshot["JDEC_NO_ENUM_SWITCH_FOLD"] = "1"
			case "archive lookup":
				d.nativeMemberLookup = func(string) *nativeMemberClass { t.Fatal("standalone must not invoke archive lookup"); return nil }
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := DecompileResult{InputHash: "existing request", Diagnostics: []DecompileDiagnostic{{Code: "existing observation"}}}
			report := before
			d.report = &report
			source, committed, err := d.standaloneEnumSwitchSource()
			if committed != (variant == "original") {
				t.Fatalf("committed=%v source=%s error=%v", committed, source, err)
			}
			if variant == "budget" || variant == "canceled" {
				if !isRequestWorkError(err) {
					t.Fatalf("request limit lost: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !committed {
				if source != "" || d.nativeMemberRoot != nil || !reflect.DeepEqual(report, before) {
					t.Fatal("failed source proof published ownership or report state")
				}
			} else {
				for _, table := range d.nativeMemberRoot.enumSwitchTables {
					if table.usersClosedRoot != nil {
						t.Fatal("one class cannot certify archive closure")
					}
				}
			}
		})
	}
}
