package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

const privateSymbolDisjointFixture = `class DisjointOwner {
 private static java.util.Map unmodifiableMap(java.util.Map input){return input;}
 static class Child{static java.util.Map wrap(java.util.Map input){return java.util.Collections.unmodifiableMap(input);}}
}
class DisjointDriver{public static void main(String[]args){Object value=new Object();java.util.Map input=new java.util.HashMap();input.put("key",value);java.util.Map view=DisjointOwner.Child.wrap(input);if(view.get("key")!=value)throw new AssertionError("identity");input.put("key","changed");if(!"changed".equals(view.get("key")))throw new AssertionError("backed view");try{view.put("forbidden",value);throw new AssertionError("write admitted");}catch(UnsupportedOperationException expected){}if(input.containsKey("forbidden"))throw new AssertionError("mutation");try{DisjointOwner.Child.wrap(null);throw new AssertionError("null admitted");}catch(NullPointerException expected){}if(DisjointOwner.Child.class.getDeclaringClass()!=DisjointOwner.class||!DisjointOwner.Child.class.isMemberClass())throw new AssertionError("declaration scope");System.out.println("private-symbol:disjoint:identity:effects:scope");}}`

func privateSymbolDisjointVariant(list, rename, handle bool) (string, string) {
	s := privateSymbolDisjointFixture
	owner := "DisjointOwner"
	if handle {
		s = strings.Replace(s, "return java.util.Collections.unmodifiableMap(input);", "java.util.function.Function<java.util.Map,java.util.Map> transform=java.util.Collections::unmodifiableMap;return transform.apply(input);", 1)
	}
	if list {
		s = strings.NewReplacer("java.util.Map", "java.util.List", "unmodifiableMap", "unmodifiableList", "new java.util.HashMap()", "new java.util.ArrayList()", "input.put(\"key\",value)", "input.add(value)", "view.get(\"key\")", "view.get(0)", "input.put(\"key\",\"changed\")", "input.set(0,\"changed\")", "view.put(\"forbidden\",value)", "view.add(value)", "input.containsKey(\"forbidden\")", "input.size()!=1").Replace(s)
	}
	if rename {
		owner = "RenamedDisjointEnvelope"
		s = strings.ReplaceAll(s, "DisjointOwner", owner)
	}
	return s, owner
}
func TestAdversarialPrivateSymbolDisjointPlatformRoundTrip(t *testing.T) {
	for _, list := range []bool{false, true} {
		for _, rename := range []bool{false, true} {
			for _, handle := range []bool{false, true} {
				t.Run(fmt.Sprintf("list=%t/renamed=%t/handle=%t", list, rename, handle), func(t *testing.T) {
					s, owner := privateSymbolDisjointVariant(list, rename, handle)
					testNativePrivateSetterFixture(t, s, owner, "DisjointDriver", "private-symbol:disjoint:identity:effects:scope\n")
				})
			}
		}
	}
}
func TestAdversarialPrivateSymbolDisjointPlatformNativeCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("JAVA8_JAVAC required")
	}
	for _, list := range []bool{false, true} {
		for _, handle := range []bool{false, true} {
			t.Run(fmt.Sprintf("list=%t/handle=%t", list, handle), func(t *testing.T) {
				s, owner := privateSymbolDisjointVariant(list, false, handle)
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, s, owner, debug) }, NativeJavac8, javac, []string{owner}, "DisjointDriver", "private-symbol:disjoint:identity:effects:scope\n", nil, nativeLexicalExactSignatures)
			})
		}
	}
}
