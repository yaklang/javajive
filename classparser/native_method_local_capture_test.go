package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMethodLocalCaptureCannotDisappearFromEmittedSource(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, `class LocalDeadOwner{Object make(final long seed){class Entry{long get(){return seed;}}return new Entry();}}`, debug)
			local, e := Parse(files["LocalDeadOwner$1Entry.class"])
			if e != nil {
				t.Fatal(e)
			}
			replaced := false
			for _, method := range local.Methods {
				name, _ := sourceBridgeUTF8(local, method.NameIndex)
				if name != "get" {
					continue
				}
				for _, a := range method.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						if len(code.Code) != 5 || code.Code[0] != byte(core.OP_ALOAD_0) || code.Code[1] != byte(core.OP_GETFIELD) || code.Code[4] != byte(core.OP_LRETURN) {
							t.Fatalf("actual original capture read %v", code.Code)
						}
						code.Code = []byte{byte(core.OP_LCONST_0), byte(core.OP_NOP), byte(core.OP_NOP), byte(core.OP_NOP), byte(core.OP_LRETURN)}
						replaced = true
					}
				}
			}
			if !replaced {
				t.Fatal("original capture reader")
			}
			files["LocalDeadOwner$1Entry.class"] = local.Bytes()
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(files["LocalDeadOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			prepared := z.prepareNativeMemberFamily(root, snapshotJDECEnv())
			if prepared == nil {
				t.Fatal("original physical constructor remains independently valid")
			}
			entry := z.finishNativeMemberFamily(prepared, z.nativeMemberLookup, false)
			if entry != nil {
				t.Fatal("unused capture was erased from original constructor/field ABI")
			}
		})
	}
}
func TestNativeMethodLocalCaptureMarkerMustBeAnActualSourceComment(t *testing.T) {
	local := &nativeMethodLocalClass{object: &ClassObject{}, constructor: &nativeMethodLocalConstructor{captures: map[string]int{"val$seed": 0}}}
	// No class name is needed to exercise the lexer: an empty owner gives the
	// exact generated prefix, while executable/source ownership is proved above.
	for _, tc := range []struct {
		name, source string
		want         bool
	}{
		{"actual comment", "long get(){return (/*jdec-owned-local-capture::val$seed*/seed);}", true},
		{"quoted fake", "String fake=\"/*jdec-owned-local-capture::val$seed*/\";", false},
		{"line comment fake", "// /*jdec-owned-local-capture::val$seed*/\nlong get(){return 0;}", false},
		{"other owner", "/*jdec-owned-local-capture:Other:val$seed*/", false},
		{"other field", "/*jdec-owned-local-capture::val$other*/", false},
		{"unterminated string", "\"missing end", false},
		{"unterminated comment", "/*missing end", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := nativeMethodLocalCaptureSourceComplete(local, tc.source, nil); got != tc.want {
				t.Fatalf("actual source capture accepted %v", got)
			}
		})
	}
	work := workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
	if nativeMethodLocalCaptureSourceComplete(local, "long get(){return (/*jdec-owned-local-capture::val$seed*/seed);}", work) {
		t.Fatal("unbounded source lexer")
	}
}
