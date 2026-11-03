package javaclassparser

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestCoremailGroovyDeadCallsitePrefixKeepsBody(t *testing.T) {
	const root = "testdata/regression/coremail-groovy-prefix"
	const name = "groovy/util/CliBuilder$_expandArgumentFile_closure2"
	raw, err := os.ReadFile(filepath.Join(root, name+".class"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != "44476616eb81117a7a51d05a399e3bb37b4bc370ccda0553c5ca5f2ac37f2f20" {
		t.Fatalf("original Coremail class changed: %s", got)
	}
	result, err := DecompileWithOptions(raw, DecompileOptions{
		RetainUnprovedSource: true,
		Resolve: func(name string) ([]byte, bool) {
			data, err := os.ReadFile(filepath.Join(root, name+".class"))
			return data, err == nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.StubMethods) != 0 || strings.Contains(result.Source, DecompileStubMarker) {
		t.Fatalf("original call-site init must remain before the real super call, not a stub: %v\n%s", result.StubMethods, result.Source)
	}
	if !strings.Contains(result.Source, "super(jdec$ctor$") || !strings.Contains(result.Source, "$getCallSiteArray();") || !strings.Contains(result.Source, "doCall(") {
		t.Fatalf("missing constructor carrier, call-site side effect or method body:\n%s", result.Source)
	}
	for _, variant := range []string{"read after super", "used by super", "overwritten"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			var code *CodeAttribute
			for _, method := range obj.Methods {
				methodName, _ := obj.getUtf8(method.NameIndex)
				if methodName != "<init>" {
					continue
				}
				for _, attr := range method.Attributes {
					if body, ok := attr.(*CodeAttribute); ok {
						code = body
					}
				}
			}
			if code == nil || len(code.Code) != 11 || code.Code[3] != core.OP_ASTORE_3 || code.Code[7] != core.OP_INVOKESPECIAL {
				t.Fatal("unexpected original constructor shape")
			}
			switch variant {
			case "read after super":
				code.Code = append(code.Code[:10:10], core.OP_ALOAD_3, core.OP_POP, core.OP_RETURN)
			case "used by super":
				code.Code[5] = core.OP_ALOAD_3
			case "overwritten":
				code.Code = append(code.Code[:10:10], core.OP_ACONST_NULL, core.OP_ASTORE_3, core.OP_RETURN)
			}
			changed, err := DecompileWithOptions(obj.Bytes(), DecompileOptions{
				RetainUnprovedSource: true,
				Resolve: func(name string) ([]byte, bool) {
					data, err := os.ReadFile(filepath.Join(root, name+".class"))
					return data, err == nil
				},
			})
			if variant == "used by super" {
				if err != nil || len(changed.StubMethods) != 0 || !strings.Contains(changed.Source, "super($getCallSiteArray(),var2);") || strings.Contains(changed.Source, "jdec$ctor$") {
					t.Fatalf("super must retain the call result as its first argument: %v\n%s", err, changed.Source)
				}
			} else if err == nil && len(changed.StubMethods) == 0 {
				t.Fatalf("non-dead local received a discard proof:\n%s", changed.Source)
			}
		})
	}
}
