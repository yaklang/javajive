package javaclassparser

import (
	"strings"
	"testing"
)

// The independently compiled driver keeps its original REF_invokeVirtual
// bootstrap. Restoring the lexical capture must not change the physical target,
// receiver identity, null handling, or constructor callback timing.
const nativeOrdinaryHandleFixture = `abstract class HandleParent{final Object observed;HandleParent(){observed=owner();}abstract Object owner();}
class HandleOwner{final Object token;HandleOwner(Object token){this.token=token;}class Child extends HandleParent{Object owner(){return HandleOwner.this;}public Object echo(Object value){return value;}}Child make(){return new Child();}}
class HandleOracle{public static void main(String[]args){Object token=new Object();HandleOwner owner=new HandleOwner(token);HandleOwner.Child child=owner.make();if(child.observed!=owner)throw new AssertionError("capture before callback");java.util.function.Function<Object,Object> echo=child::echo;for(Object value:new Object[]{null,token,owner,child})if(echo.apply(value)!=value)throw new AssertionError("handle identity");try{HandleOwner.Child absent=null;java.util.function.Function<Object,Object> bad=absent::echo;throw new AssertionError("missing receiver failure");}catch(NullPointerException expected){}System.out.println("ordinary:handle:capture:identity:null");}}`

func TestNativeMemberOrdinaryHandleKeepsOriginalTargetRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeOrdinaryHandleFixture, []string{"HandleOwner"}, "HandleOracle", "ordinary:handle:capture:identity:null\n")
}
func TestNativeMemberOrdinaryHandleRenamedKeepsOriginalTargetRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeOrdinaryHandleFixture, "HandleOwner", "SeparateLexicalScope")
	f = strings.ReplaceAll(f, "echo", "unchanged")
	testNativeIndependentFamilyFixture(t, f, []string{"SeparateLexicalScope"}, "HandleOracle", "ordinary:handle:capture:identity:null\n")
}

func TestNativeMemberOrdinaryStaticHandleKeepsOriginalTargetRoundTrip(t *testing.T) {
	f := strings.Replace(nativeOrdinaryHandleFixture, "class Child extends HandleParent", "static class PublicValue{public static Object echo(Object x){return x;}}class Child extends HandleParent", 1)
	f = strings.Replace(f, "child::echo", "HandleOwner.PublicValue::echo", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"HandleOwner"}, "HandleOracle", "ordinary:handle:capture:identity:null\n")
}
