package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialEnumRolesComposeWithNestedAnonymousCaptureRoundTrip(t *testing.T) {
	enum := `public enum Kind{FIRST{public int code(){return 17;}},LAST{public int code(){return 31;}};public abstract int code();}`
	source := strings.Replace(nativeAnonymousNestedFixture, "class NestedOwner{", "class NestedOwner{"+enum, 1)
	check := `if(NestedOwner.Kind.FIRST.code()!=17||NestedOwner.Kind.LAST.code()!=31||!NestedOwner.Kind.FIRST.getClass().isAnonymousClass()||NestedOwner.Kind.FIRST.getClass().getSuperclass()!=NestedOwner.Kind.class)throw new AssertionError("enum role");`
	source = strings.Replace(source, `System.out.println(rows+":"+NestedEffects.trace);`, check+`System.out.println(rows+":"+NestedEffects.trace);`, 1)
	for _, owner := range []string{"NestedOwner", "RenamedNestedOwner"} {
		t.Run(owner, func(t *testing.T) {
			fixture := strings.ReplaceAll(source, "NestedOwner", owner)
			testNativePrivateSetterCompiledFixture(t, owner, "NestedDriver", "58:P\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
			})
		})
	}
}
