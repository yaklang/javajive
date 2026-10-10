package javaclassparser

import (
	"strconv"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

func TestNativeObjectRootFallbackIncludesExactNoThrowsConstructor(t *testing.T) {
	// These releases intentionally have no general declaration catalog. The
	// root constructor's language guarantee grants no overload, ancestry or
	// exception knowledge about an unrelated platform type.
	for _, release := range []int{7, 10, 12, 18, 22} {
		t.Run(strconv.Itoa(release), func(t *testing.T) {
			d := NewClassObjectDumper(&ClassObject{MajorVersion: 52})
			d.options.TargetSourceVersion = release
			provider := d.buildInvocationMetadata()
			exceptions, known := exactInvocationExceptions(provider, "java/lang/Object", "<init>", "()V")
			if !known || len(exceptions) != 0 {
				t.Fatal("root constructor lost its exact no-throws declaration")
			}
			for _, descriptor := range []string{"(I)V", "(Ljava/lang/Object;)V"} {
				if _, known := exactInvocationExceptions(provider, "java/lang/Object", "<init>", descriptor); known {
					t.Fatal("invented a root constructor overload")
				}
			}
			if _, known := provider("java/lang/StringBuilder"); known {
				t.Fatal("root guarantee substituted a different platform profile")
			}
			table, known := provider("java/lang/Object")
			if !known {
				t.Fatal("missing root namespace")
			}
			constructors := 0
			for _, method := range table.Methods {
				if method.Name == "<init>" {
					constructors++
					if method.Desc != "()V" || !method.Public || method.Static || method.Varargs || !method.ExceptionsKnown {
						t.Fatalf("wrong root constructor %+v", method)
					}
				}
			}
			if constructors != 1 {
				t.Fatal("duplicate or absent root constructor")
			}
			// exact invocation lookup must not treat the known declaration as
			// permission to skip the caller's symbolic descriptor witness.
			witness := callbinding.Witness{Owner: "java/lang/Object", Name: "<init>", Desc: "(I)V", Kind: callbinding.Special}
			family, err := callbinding.FamilyOf(witness, provider)
			if err == nil && family.Target != nil {
				t.Fatal("unknown constructor descriptor rebound to no-arg root")
			}
		})
	}
}
