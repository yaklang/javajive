package javaclassparser

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialEnumMemberLedgerRequiresWholeOriginalRegenerationProtocol(t *testing.T) {
	compiler, _ := t04Tools(t)
	files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, `enum LedgerChoice{LEFT,RIGHT;static int $values(int value){return value;}}`, "LedgerChoice", "none", false)
	for _, variant := range []string{"original", "changed helper Code", "changed backing role", "missing values factory", "duplicate initializer", "ordinary helper role", "named metadata unproved", "native compiler", "older class", "other source level", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			object, err := Parse(bytes.Clone(files["LedgerChoice.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(object)
			d.options = DecompileOptions{SourceCompiler: ModernJavac, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}}
			d.report = &DecompileResult{Members: newMemberLedger(object)}
			source, err := d.DumpClass()
			if err != nil {
				t.Fatal("original typed enum", err)
			}
			for i := range d.report.Members {
				row := &d.report.Members[i]
				if row.Name == "$values" && row.Descriptor == "(I)I" && row.State != "preserved" {
					t.Fatal("ordinary helper-name overload was consumed as compiler code", *row)
				}
				row.State, row.Evidence = "unsupported", "original declaration not yet accounted"
			}
			switch variant {
			case "changed helper Code", "ordinary helper role":
				found := false
				for _, method := range object.Methods {
					if method.AccessFlags != 0x100a {
						continue
					}
					found = true
					if variant == "ordinary helper role" {
						method.AccessFlags &^= 0x1000
					} else {
						for _, attribute := range method.Attributes {
							if code, ok := attribute.(*CodeAttribute); ok {
								code.Code[0] = core.OP_ICONST_0
							}
						}
					}
				}
				if !found {
					t.Fatal("original collision-renamed synthetic helper absent")
				}
			case "changed backing role":
				for _, field := range object.Fields {
					if field.AccessFlags == 0x101a {
						field.AccessFlags &^= 0x1000
					}
				}
			case "missing values factory":
				object.Methods = slices.DeleteFunc(slices.Clone(object.Methods), func(method *MemberInfo) bool {
					name, _ := sourceBridgeUTF8(object, method.NameIndex)
					return name == "values"
				})
			case "duplicate initializer":
				for _, method := range object.Methods {
					name, _ := sourceBridgeUTF8(object, method.NameIndex)
					if name == "<clinit>" {
						object.Methods = append(slices.Clone(object.Methods), method)
						break
					}
				}
			case "named metadata unproved":
				d.enumParameterMetadataUnsupported = true
			case "native compiler":
				d.options.SourceCompiler = NativeJavac8
			case "older class":
				object.MajorVersion = 51
			case "other source level":
				d.options.TargetSourceVersion = 11
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := object.Bytes()
			d.recordEnumRegeneratedMembers(source, nil)
			count := 0
			for _, row := range d.report.Members {
				if row.State == "regenerated" {
					count++
					if !strings.Contains(row.Evidence, "protocol") || row.Name == "$values" && row.Descriptor == "(I)I" {
						t.Fatal("unproved compiler role", row)
					}
				}
			}
			want := 0
			if variant == "original" {
				want = 4 // values, valueOf, the renamed array helper and backing array.
			}
			if count != want {
				t.Fatalf("certified regenerated members=%d, want %d", count, want)
			}
			if !bytes.Equal(before, object.Bytes()) {
				t.Fatal("member accounting rewrote the original class")
			}
		})
	}
}
