package javaclassparser

// The original superclass Signature maps N to the flattened subclass's N.
// Its constructor descriptor takes Object, so a source N view must preserve
// the same unchecked argument identity. Declaration-based binding supersedes
// JDEC_SUPER_CTOR_TYPEVAR_ARG_OFF; both settings must preserve the original tuple.

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestSuperCtorTypeVarArgCastIsLoadBearing(t *testing.T) {
	sub, err := os.ReadFile("testdata/regression/SuperCtorTypeVarSeed$1.class")
	if err != nil {
		t.Fatalf("read SuperCtorTypeVarSeed$1 seed: %v", err)
	}
	// Resolver for the sibling base unit so the super ctor Signature (`(String, N)`) and the super class's
	// formal type-param names are resolvable; the flattened anon unit alone cannot see them.
	resolver := func(internalName string) ([]byte, bool) {
		base := internalName[strings.LastIndexByte(internalName, '/')+1:]
		b, e := os.ReadFile("testdata/regression/" + base + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	// The constructor tuple and superclass type-argument relation prove the
	// unchecked N view. Its old spelling gate has been superseded.
	assertReviewedTypeVarMethod(t, sub, "<init>", "(LSuperCtorTypeVarSeed;Ljava/lang/String;Ljava/lang/Object;)V", "")
	assertReviewedClassSignature(t, sub, "LSuperCtorTypeVarBaseSeed<TN;>;")
	assertReviewedTypeVarInvoke(t, "testdata/regression/SuperCtorTypeVarSeed$1.class", "<init>", "(LSuperCtorTypeVarSeed;Ljava/lang/String;Ljava/lang/Object;)V", 8, core.OP_INVOKESPECIAL, "SuperCtorTypeVarBaseSeed", "<init>", "(Ljava/lang/String;Ljava/lang/Object;)V")

	base, _ := resolver("SuperCtorTypeVarBaseSeed")
	assertReviewedTypeVarMethod(t, base, "<init>", "(Ljava/lang/String;Ljava/lang/Object;)V", "(Ljava/lang/String;TN;)V")
	t.Setenv("JDEC_SUPER_CTOR_TYPEVAR_ARG_OFF", "")
	on, err := DecompileWithResolver(sub, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !regexp.MustCompile(`super\([^,]+,\(N\)\([A-Za-z_$][A-Za-z0-9_$]*\)\)`).MatchString(compactReviewedGenericSource(on)) {
		t.Errorf("expected the original super-constructor N argument view, got:\n%s", on)
	}

	t.Setenv("JDEC_SUPER_CTOR_TYPEVAR_ARG_OFF", "1")
	off, err := DecompileWithResolver(sub, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if off != on {
		t.Fatal("retired constructor spelling gate must not change proven superclass argument binding")
	}
}
