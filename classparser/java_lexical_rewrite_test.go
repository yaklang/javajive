package javaclassparser

import (
	"strings"
	"testing"
)

func TestJavaCodeRewriteLiteralBoundaries(t *testing.T) {
	cases := []string{
		`"Integer::intValue"`,
		`"escaped \\\" Integer::intValue this.getMatchers()"`,
		"\"\"\"\nInteger::intValue\nthis.getMatchers()\n\"\"\"",
		"// Integer::intValue this.getMatchers()\r\n",
		"/* Integer::intValue\nthis.getMatchers() */",
		`'\''`,
		`"__JDEC_PROTECTED_0__ Integer::intValue"`,
	}
	for _, data := range cases {
		t.Run(data, func(t *testing.T) {
			src := "class X { Object f = Integer::intValue; Object v = " + data + "; }"
			got := applyMockitoShapes(src)
			if !strings.Contains(got, data) {
				t.Fatalf("protected data changed:\n%s", got)
			}
			if !strings.Contains(got, "Object f = x -> ((Integer)x).intValue();") {
				t.Fatalf("code was not rewritten:\n%s", got)
			}
			if twice := applyMockitoShapes(got); twice != got {
				t.Fatalf("rule not idempotent:\n%s", twice)
			}
		})
	}
}
func TestMockitoPackageGateUsesCode(t *testing.T) {
	for _, source := range []string{
		`package org.mockitofake; class X {Object x = Integer::intValue;}`,
		`package ordinary; class X {String x="package org.mockito";Object y=Integer::intValue;}`,
		"package ordinary; class X {String x=\"\"\"\npackage org.mockito;\n\"\"\";Object y=Integer::intValue;}",
	} {
		if got := fixHardjarShapes(source); got != source {
			t.Fatalf("false package gate:\n%s", got)
		}
	}
}
