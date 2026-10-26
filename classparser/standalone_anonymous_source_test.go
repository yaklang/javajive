package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"reflect"
	"testing"
)

func TestNativeAnonymousStandalonePublicationRequiresCompleteOriginalFamily(t *testing.T) {
	files := nativeCompileClasses(t, `interface SingleRead{boolean read();} class StandaloneOwner{SingleRead make(final boolean value){return new SingleRead(){public boolean read(){return value;}};}}`)
	for _, variant := range []string{"original", "no resolver", "missing child", "mutable capture", "missing ownership", "named self row", "invalid constructor", "invalid body", "disabled", "archive lookup", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(files["StandaloneOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			child, err := Parse(files["StandaloneOwner$1.class"])
			if err != nil {
				t.Fatal(err)
			}
			inputs := map[string][]byte{}
			for n, b := range files {
				inputs[n] = append([]byte(nil), b...)
			}
			d := NewClassObjectDumper(root)
			d.options = DecompileOptions{Mode: Precision, SourceCompiler: ModernJavac, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}}
			d.foldSiblingResolver = func(n string) ([]byte, bool) { b, ok := inputs[n+".class"]; return b, ok }
			switch variant {
			case "no resolver":
				d.foldSiblingResolver = nil
			case "missing child":
				delete(inputs, "StandaloneOwner$1.class")
			case "mutable capture":
				for _, f := range child.Fields {
					f.AccessFlags &^= 16
				}
				inputs["StandaloneOwner$1.class"] = child.Bytes()
			case "missing ownership":
				for i, a := range child.Attributes {
					if a, ok := a.(*UnparsedAttribute); ok && a.Name == "EnclosingMethod" {
						child.Attributes = append(child.Attributes[:i], child.Attributes[i+1:]...)
						break
					}
				}
				inputs["StandaloneOwner$1.class"] = child.Bytes()
			case "named self row":
				for _, a := range child.Attributes {
					if a, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range a.Classes {
							row.InnerNameIndex = child.Fields[0].NameIndex
						}
					}
				}
				inputs["StandaloneOwner$1.class"] = child.Bytes()
			case "invalid constructor", "invalid body":
				for _, m := range child.Methods {
					n, _ := sourceBridgeUTF8(child, m.NameIndex)
					if variant == "invalid constructor" && n == "<init>" || variant == "invalid body" && n == "read" {
						for _, a := range m.Attributes {
							if a, ok := a.(*CodeAttribute); ok {
								a.Code = []byte{0xbf}
							}
						}
					}
				}
				inputs["StandaloneOwner$1.class"] = child.Bytes()
			case "disabled":
				d.options.EnvSnapshot["JDEC_NATIVE_ANONYMOUS_OFF"] = "1"
			case "archive lookup":
				d.nativeMemberLookup = func(string) *nativeMemberClass { t.Fatal("standalone called archive lookup"); return nil }
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := DecompileResult{InputHash: "existing", Members: []MemberRecord{}, Diagnostics: []DecompileDiagnostic{{Code: "existing"}}}
			report := before
			d.report = &report
			source, committed, err := d.standaloneAnonymousSource()
			if committed != (variant == "original") {
				t.Fatalf("committed=%v err=%v source=%s", committed, err, source)
			}
			if variant == "budget" || variant == "canceled" {
				if !isRequestWorkError(err) {
					t.Fatalf("request limit lost: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !committed && (source != "" || d.nativeAnonymousRoot != nil || d.nativeMemberRoot != nil || !reflect.DeepEqual(before, report)) {
				t.Fatal("failed publication leaked source, ownership or ledger")
			}
			if committed && (d.nativeAnonymousRoot == nil || d.nativeAnonymousRoot.forest != nil || d.nativeMemberLookup != nil) {
				t.Fatal("unexpected archive permission")
			}
		})
	}
}
