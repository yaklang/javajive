package javaclassparser

import "testing"

func TestNativeDiagnosticSourceBoundaryDistinguishesLiteralAndSignal(t *testing.T) {
	for _, row := range []struct {
		name, source string
		failure      bool
	}{
		{"ordinary", "class C { int f(){return 7;} }", false},
		{"string marker", `class C {String f(){return "yak-decompiler:";}}`, false},
		{"string placeholder", `class C {String f(){return "empty slot value";}}`, false},
		{"escaped quote", `class C {String f(){return "before \" yak-decompiler: \" after";}}`, false},
		{"escaped slash", `class C {String f(){return "\\yak-decompiler:";}}`, false},
		{"comment shaped data", `class C {String f(){return "/* yak-decompiler: failed */";}}`, false},
		{"line shaped data", `class C {String f(){return "// yak-decompiler: failed";}}`, false},
		{"text block", "class C {String s=\"\"\"\n yak-decompiler: empty slot value\n\"\"\";}", false},
		{"single quote before data", `class C {char c='\'';String s="empty slot value";}`, false},
		{"bare placeholder", `class C {} empty slot value`, true},
		{"failure comment", `class C {} /* yak-decompiler: failed */`, true},
		{"failure line", `class C {} // yak-decompiler: failed`, true},
		{"quoted message in comment", `class C {} /* "yak-decompiler:" */`, true},
		{"signal after literal", `class C {String s="yak-decompiler:";} /* yak-decompiler: failed */`, true},
		{"placeholder after char", `class C {char c='"';} empty slot value`, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			if got := legacySourceHasFailureMarker(row.source); got != row.failure {
				t.Fatalf("failure=%v for %s", got, row.source)
			}
		})
	}
}

func TestNativeDiagnosticMethodBoundaryRetainsStructuredAndLegacyFailures(t *testing.T) {
	for _, row := range []struct {
		name    string
		body    *dumpedMethods
		failure bool
	}{
		{"missing record", nil, true},
		{"complete record", &dumpedMethods{code: "return 7;", bodyCode: "return 7;"}, false},
		{"marker data", &dumpedMethods{code: `return "yak-decompiler:";`}, false},
		{"placeholder data", &dumpedMethods{code: `return "empty slot value";`}, false},
		{"stub data", &dumpedMethods{code: `return "stub";`, bodyCode: `return "stub";`}, false},
		{"typed stub", &dumpedMethods{isStub: true}, true},
		{"typed incomplete value", &dumpedMethods{incompleteValue: true}, true},
		{"typed malformed try", &dumpedMethods{malformedTry: true}, true},
		{"legacy stub", &dumpedMethods{bodyCode: "stub"}, true},
		{"legacy embedded diagnostic", &dumpedMethods{code: "return 7; /* yak-decompiler: failed */"}, true},
		{"legacy placeholder", &dumpedMethods{code: "return empty slot value;"}, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			if got := row.body.sourceReconstructionFailed(); got != row.failure {
				t.Fatalf("reconstruction failed=%v, want %v", got, row.failure)
			}
		})
	}
}
