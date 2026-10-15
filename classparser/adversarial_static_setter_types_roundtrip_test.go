package javaclassparser

import (
	"strings"
	"testing"
)

func staticSetterTypesFixture() string {
	s := strings.ReplaceAll(nativePrivateSetterTypesFixture, "TypedSetterOwner<T>", "TypedSetterOwner")
	s = strings.ReplaceAll(s, "TypedSetterOwner<Object>", "TypedSetterOwner")
	s = strings.ReplaceAll(s, "class Layer extends TypedSetterParent", "static class Layer extends TypedSetterParent")
	s = strings.ReplaceAll(s, "private volatile T token", "private static volatile Object token")
	s = strings.ReplaceAll(s, "T token(", "Object token(")
	s = strings.ReplaceAll(s, "T v)", "Object v)")
	for _, typ := range []string{"boolean", "byte", "char", "short", "int", "long", "float", "double", "Object[]"} {
		s = strings.ReplaceAll(s, "private "+typ+" ", "private static "+typ+" ")
	}
	s = strings.ReplaceAll(s, "return TypedSetterOwner.this.seed;", "return null;")
	s = strings.ReplaceAll(s, "owner.new Layer()", "new TypedSetterOwner.Layer()")
	s = strings.ReplaceAll(s, "if(layer.observed!=token)", "if(layer.observed!=null)")
	s = strings.Replace(s, "for(Object v:new Object[]", "for(TypedSetterOwner.Layer receiver:new TypedSetterOwner.Layer[]{layer,null}){for(Object v:new Object[]", 1)
	s = strings.ReplaceAll(s, "(layer,v)", "(receiver,v)")
	s = strings.ReplaceAll(s, "(layer)", "(receiver)")
	s = strings.Replace(s, "System.out.println(rows+", "}System.out.println(rows+", 1)
	return strings.ReplaceAll(s, "TypedSetter", "StaticWords")
}
func TestAdversarialStaticSetterWidthsPreserveOriginalValues(t *testing.T) {
	testNativePrivateSetterFixtureDefaultPolicy(t, staticSetterTypesFixture(), "StaticWordsOwner", "StaticWordsDriver", "76:typed-bits:identity\n", nil)
}

// Static qualifier expressions are evaluated even when they produce null;
// neither that null nor a noncanonical JVM boolean word may be reinterpreted
// as an instance dereference or a nonzero truth test.
func TestAdversarialStaticBooleanSetterPreservesLowBitAndNullQualifier(t *testing.T) {
	f := strings.Replace(nativeBooleanAccessorFixture, "private boolean value;", "private static boolean value;", 1)
	old := `try{writer.put(null,next);throw new AssertionError("missing failure");}catch(RuntimeException e){if(fail==0?!(e instanceof NullPointerException):e!=BoolAccessEffects.failure)throw new AssertionError("failure identity");}`
	replacement := `if(fail==0){if(writer.put(null,next)!=next||writer.get()!=next)throw new AssertionError("static null qualifier/value");}else try{writer.put(null,next);throw new AssertionError("missing operand failure");}catch(RuntimeException e){if(e!=BoolAccessEffects.failure)throw new AssertionError("operand failure identity",e);}`
	if strings.Count(f, old) != 1 {
		t.Fatal("original static driver failure contract missing")
	}
	f = strings.Replace(f, old, replacement, 1)
	for _, owner := range []string{"BoolAccessOwner", "BooleanStaticScope"} {
		t.Run(owner, func(t *testing.T) {
			fixture := strings.ReplaceAll(f, "BoolAccessOwner", owner)
			testNativePrivateSetterFixtureDefaultPolicy(t, fixture, owner, "BoolAccessDriver", "2:boolean:accessor:store:result:order\n", func(t *testing.T, files map[string][]byte) {
				mutateNativeBooleanAccessorNoncanonicalWordsForOwner(t, files, owner)
			})
		})
	}
}
