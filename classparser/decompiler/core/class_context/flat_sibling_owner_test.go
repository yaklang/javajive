package class_context

import (
	"strings"
	"testing"
)

// A flat sibling unit is emitted as its own top-level class literally named
// `Outer$Inner`, and the declaration authority reports exactly that binary
// spelling. Its package root can be a value name in the referencing class
// (wmsvr's `a.ci` declares field `a`), and JLS 6.5.5.1 then makes every
// package-qualified spelling illegal. Real javac 21 and javac 8 reject it:
//
//	a/ci.java:13: error: non-static variable a cannot be referenced from a static context
//	        return a.ci$a.a(value);
//	a/ci.java:13: error: cannot find symbol
//	  symbol:   variable ci$a
//	  location: variable a of type Map<String,ci$a>
//
// while the same-package flat simple name compiles on both:
//
//	return ci$a.a(value);
//
// So a flat declaration source name must be shortened like any other
// same-package unit instead of being returned verbatim.
func TestFlatSiblingOwnerUsesSpellableSamePackageSimpleName(t *testing.T) {
	f := &ClassContext{
		ClassName:    "a.ci",
		PackageName:  "a",
		FunctionName: "a",
		// The member-family authority reports the flat binary spelling.
		DeclarationSourceName: func(string) (string, bool) { return "a.ci$a", true },
		// Field `a` obscures the package root; nothing obscures `ci$a`.
		SourceValueNameShadow: func(name string) bool { return name == "a" },
		StaticMethodImports:   NewStaticMethodImports(),
	}
	if got := f.ShortTypeName("a.ci$a"); got != "ci$a" {
		t.Fatalf("ShortTypeName = %q, want the flat simple name %q", got, "ci$a")
	}
	got := f.StaticClassCallPrefix("a.ci$a", "a", "(La/ci$a;)Ljava/lang/String;")
	if got != "ci$a." {
		t.Fatalf("prefix = %q, want %q", got, "ci$a.")
	}
	if err := f.StaticMethodImports.Error(); err != nil {
		t.Fatalf("proved spelling latched a refusal: %v", err)
	}
	if strings.HasPrefix(got, "a.") {
		t.Fatalf("package-qualified spelling is illegal under this field: %q", got)
	}
}
