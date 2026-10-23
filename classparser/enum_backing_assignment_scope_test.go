package javaclassparser

import (
	"strings"
	"testing"
)

func TestEnumBackingAssignmentDoesNotRemoveLaterLocal(t *testing.T) {
	source := "enum E {\n\tONE;\n\tstatic {\n\t\tk = var0;\n\t}\n\tstatic int keep(int k) {\n\t\tk = 17;\n\t\treturn k;\n\t}\n}\n"
	got := stripEnumBackingAssignment(source, "k")
	if strings.Contains(got, "k = var0;") {
		t.Fatalf("backing assignment survived:\n%s", got)
	}
	if !strings.Contains(got, "k = 17;") {
		t.Fatalf("later method local was erased:\n%s", got)
	}
}
