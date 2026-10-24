package javaclassparser

import (
	"strings"
	"testing"
)

// The source cycle publishes atomically. Original drivers and producers check
// that a named method-local body is emitted only inside its original method,
// with its captured value, enclosing declaration and generic Signature intact.
func TestAdversarialSourceTransactionMethodLocalsKeepOriginalMethodOwnership(t *testing.T) {
	for _, scope := range []string{"instance method", "static method", "named member method"} {
		t.Run(scope, func(t *testing.T) {
			method := `Object local(final Object token){class Entry implements java.util.function.Supplier<Object>{public Object get(){return token;}}return new Entry();}`
			needle, replacement := "class CycleScopeA{", "class CycleScopeA{"+method
			call, owner := "new CycleScopeA().local(a)", "CycleScopeA"
			if scope == "static method" {
				replacement = "class CycleScopeA{static " + method
			}
			if scope == "named member method" {
				needle = "class CycleScopeA{static class Value{"
				replacement = needle + method
				call = "left.local(a)"
				owner = "CycleScopeA.Value"
			}
			f := strings.Replace(nativeCyclicDependencyFixture, needle, replacement, 1)
			check := `Object local=` + call + `;if(((java.util.function.Supplier<?>)local).get()!=a||local.getClass().getEnclosingMethod()==null||local.getClass().getEnclosingMethod().getDeclaringClass()!=` + owner + `.class||!local.getClass().getEnclosingMethod().getName().equals("local"))throw new AssertionError("local identity/original method");rows++;`
			f = strings.Replace(f, `rows++;`, check, 1)
			for _, rename := range []bool{false, true} {
				name := "original"
				fixture := f
				owners := []string{"CycleScopeA", "CycleScopeB"}
				if rename {
					name = "renamed"
					fixture = strings.ReplaceAll(strings.ReplaceAll(fixture, "CycleScopeA", "PrimaryOwner"), "CycleScopeB", "SecondaryOwner")
					owners = []string{"PrimaryOwner", "SecondaryOwner"}
				}
				t.Run(name, func(t *testing.T) {
					testNativeIndependentFamilyFixture(t, fixture, owners, "CyclicDependencyDriver", "4:cyclic:declarations:independent:owners\n", nativeLexicalExactSignatures)
				})
			}
		})
	}
}
