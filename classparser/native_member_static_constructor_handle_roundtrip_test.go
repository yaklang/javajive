package javaclassparser

import (
	"strings"
	"testing"
)

// A static member's public constructor has no implicit enclosing operand.
// Original independently retained bootstrap/driver bytes must still bind the
// same physical descriptor after all declarations move into their lexical owner.
func TestNativeMemberStaticPublicConstructorHandleRoundTrip(t *testing.T) {
	for _, owner := range []string{"HandleOwner", "IndependentConstructorScope"} {
		t.Run(owner, func(t *testing.T) {
			f := strings.Replace(nativeOrdinaryHandleFixture, "class Child extends HandleParent", `static class StaticValue{final Object token;public StaticValue(Object token){this.token=token;}}class Child extends HandleParent`, 1)
			f = strings.Replace(f, `java.util.function.Function<Object,Object> echo=child::echo;`, `java.util.function.Function<Object,HandleOwner.StaticValue> ctor=HandleOwner.StaticValue::new;for(Object v:new Object[]{null,token,owner,child}){HandleOwner.StaticValue a=ctor.apply(v);HandleOwner.StaticValue b=ctor.apply(v);if(a==b||a.token!=v||b.token!=v)throw new AssertionError("constructor handle allocation/identity");}java.util.function.Function<Object,Object> echo=child::echo;`, 1)
			f = strings.ReplaceAll(f, "HandleOwner", owner)
			testNativeIndependentFamilyFixture(t, f, []string{owner}, "HandleOracle", "ordinary:handle:capture:identity:null\n")
		})
	}
}

func TestNativeMemberStaticPublicVarargsConstructorHandleKeepsArrayIdentityRoundTrip(t *testing.T) {
	f := strings.Replace(nativeOrdinaryHandleFixture, "class Child extends HandleParent", `static class StaticValue{final Object[] tokens;public StaticValue(Object... tokens){this.tokens=tokens;}}class Child extends HandleParent`, 1)
	f = strings.Replace(f, `java.util.function.Function<Object,Object> echo=child::echo;`, `java.util.function.Function<Object[],HandleOwner.StaticValue> ctor=HandleOwner.StaticValue::new;for(Object[] v:new Object[][]{null,new Object[0],new Object[]{token,null,owner,child}}){HandleOwner.StaticValue a=ctor.apply(v);HandleOwner.StaticValue b=ctor.apply(v);if(a==b||a.tokens!=v||b.tokens!=v)throw new AssertionError("constructor varargs allocation/array identity");}java.util.function.Function<Object,Object> echo=child::echo;`, 1)
	testNativeIndependentFamilyFixture(t, f, []string{"HandleOwner"}, "HandleOracle", "ordinary:handle:capture:identity:null\n")
}
