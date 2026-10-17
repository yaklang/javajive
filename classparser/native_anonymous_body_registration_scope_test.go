package javaclassparser

import (
	"strings"
	"testing"
)

func TestNativeAnonymousBodyRegistrationScopeIsOpaque(t *testing.T) {
	const body = `return new Outer(new Arg(capture) /*jdec-owned-anonymous-ordinal:1:Owner*/ {
 Object inner(){return capture;}
}) /*jdec-owned-anonymous-ordinal:2:Owner*/ {
 Object outer(){return capture;}
};`
	want := strings.Replace(body, "Arg(capture)", "Arg(changed)", 1)
	for _, space := range []string{" ", "\n\t", "\f", ""} {
		t.Run("separator="+space, func(t *testing.T) {
			source := strings.ReplaceAll(body, "*/ {", "*/"+space+"{")
			expected := strings.ReplaceAll(want, "*/ {", "*/"+space+"{")
			if got := nativeRewriteEnclosingScope(source, func(s string) string {
				return strings.ReplaceAll(s, "capture", "changed")
			}); got != expected {
				t.Fatalf("constructor arguments or lexical bodies changed:\n%s\nwant:\n%s", got, expected)
			}
		})
	}
	for _, variant := range []string{"lost", "duplicated"} {
		t.Run(variant, func(t *testing.T) {
			if got := nativeRewriteEnclosingScope(body, func(s string) string {
				if variant == "lost" {
					return strings.ReplaceAll(s, "/*jdec lexical members 0*/", "")
				}
				return s + s
			}); got != body {
				t.Fatal("accepted a lost or duplicated owned argument body")
			}
		})
	}
	ordinals, known := nativeAnonymousOrdinalsWithinOwner(body, "Owner")
	if !known || len(ordinals) != 2 || ordinals[0] != 1 || ordinals[1] != 2 {
		t.Fatalf("registration must visit argument declarations before enclosing body: %v %v", ordinals, known)
	}
}
