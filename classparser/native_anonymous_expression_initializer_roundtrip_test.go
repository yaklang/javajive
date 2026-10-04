package javaclassparser

import (
	"strings"
	"testing"
)

func TestNativeAnonymousPostSuperEffectfulCaptureInitializerRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousInitializerFixture, `static String trace="";`, `static String trace="";static Object observe(Object value){trace+="I";return value;}`, 1)
	f = strings.Replace(f, `Object ref=captured;`, `Object ref=AnonymousInitEffects.observe(captured);`, 1)
	f = strings.Replace(f, `!AnonymousInitEffects.trace.equals("P0")||`, `!AnonymousInitEffects.trace.equals("P0I")||`, 1)
	testNativePrivateSetterFixture(t, f, "AnonymousInitOwner", "AnonymousInitDriver", "6:anonymous:initializer:callback:identity:owner\n")
}
func TestNativeAnonymousPostSuperEffectfulCaptureMutationRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousInitializerFixture, `static String trace="";`, `static String trace="";static final Object replacement=new Object();static Object observe(Object value){trace+="I";return value;}`, 1)
	f = strings.Replace(f, `Object ref=captured;`, `Object ref=AnonymousInitEffects.observe(captured);`, 1)
	f = strings.Replace(f, `AnonymousInitEffects.trace+="P"+first();`, `AnonymousInitEffects.trace+="P"+first();try{java.lang.reflect.Field capture=getClass().getDeclaredField("val$captured");capture.setAccessible(true);capture.set(this,AnonymousInitEffects.replacement);}catch(ReflectiveOperationException e){throw new AssertionError(e);}`, 1)
	f = strings.Replace(f, `p.alias()!=v`, `p.alias()!=AnonymousInitEffects.replacement`, 1)
	f = strings.Replace(f, `!AnonymousInitEffects.trace.equals("P0")||`, `!AnonymousInitEffects.trace.equals("P0I")||`, 1)
	testNativePrivateSetterFixture(t, f, "AnonymousInitOwner", "AnonymousInitDriver", "6:anonymous:initializer:callback:identity:owner\n")
}

func TestNativeAnonymousPostSuperEffectfulInitializerRenamedRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousInitializerFixture, `static String trace="";`, `static String trace="";static Object observe(Object value){trace+="I";return value;}`, 1)
	f = strings.Replace(f, `Object ref=captured;`, `Object ref=AnonymousInitEffects.observe(captured);`, 1)
	f = strings.Replace(f, `!AnonymousInitEffects.trace.equals("P0")||`, `!AnonymousInitEffects.trace.equals("P0I")||`, 1)
	f = strings.ReplaceAll(strings.ReplaceAll(f, "AnonymousInitOwner", "DifferentInitializationScope"), "captured", "differentCapture")
	testNativePrivateSetterFixture(t, f, "DifferentInitializationScope", "AnonymousInitDriver", "6:anonymous:initializer:callback:identity:owner\n")
}
func TestNativeAnonymousPostSuperNestedCallInitializerRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousInitializerFixture, `static String trace="";`, `static String trace="";static Object observe(Object value){trace+="I";return value;}`, 1)
	f = strings.Replace(f, `Object ref=captured;`, `Object ref=AnonymousInitEffects.observe(AnonymousInitEffects.observe(captured));`, 1)
	f = strings.Replace(f, `!AnonymousInitEffects.trace.equals("P0")||`, `!AnonymousInitEffects.trace.equals("P0II")||`, 1)
	testNativePrivateSetterFixture(t, f, "AnonymousInitOwner", "AnonymousInitDriver", "6:anonymous:initializer:callback:identity:owner\n")
}
func TestNativeAnonymousPostSuperFreshAllocationInitializerRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousInitializerFixture, `static String trace="";`, `static String trace="";static Object observe(Object value){trace+="I";return value;}`, 1)
	f = strings.Replace(f, `Object ref=captured;`, `Object ref=AnonymousInitEffects.observe(new java.util.ArrayList<Object>(java.util.Collections.singletonList(captured)));`, 1)
	f = strings.Replace(f, `p.alias()!=v`, `!(p.alias() instanceof java.util.ArrayList)||((java.util.List<?>)p.alias()).size()!=1||((java.util.List<?>)p.alias()).get(0)!=v`, 1)
	f = strings.Replace(f, `!AnonymousInitEffects.trace.equals("P0")||`, `!AnonymousInitEffects.trace.equals("P0I")||`, 1)
	testNativePrivateSetterFixture(t, f, "AnonymousInitOwner", "AnonymousInitDriver", "6:anonymous:initializer:callback:identity:owner\n")
}

func TestNativeAnonymousPostSuperGenericCheckedCallInitializerRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousGenericInitializerFixture, `static String trace="";`, `static String trace="";static <Q> Q[] observe(Q[] value)throws java.io.IOException{trace+="I";return value;}`, 1)
	f = strings.Replace(f, `final Q[] copy=values;`, `final Q[] copy=GenericAnonInitEffects.observe(values);`, 1)
	f = strings.Replace(f, `!GenericAnonInitEffects.trace.equals("Ptrue")||`, `!GenericAnonInitEffects.trace.equals("PtrueI")||`, 1)
	testNativePrivateSetterFixture(t, f, "GenericAnonInitOwner", "GenericAnonInitDriver", "3:generic:array:initializer:checked:owner\n")
}
func TestNativeAnonymousPostSuperInitializerOwnVirtualCallRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousInitializerFixture, `Object ref=captured;`, `Object ref=observe();Object observe(){AnonymousInitEffects.trace+="I";if(first()!=-1||Double.doubleToRawLongBits(saved)!=Double.doubleToRawLongBits(wide))throw new AssertionError("earlier initializer order");return captured;}`, 1)
	f = strings.Replace(f, `!AnonymousInitEffects.trace.equals("P0")||`, `!AnonymousInitEffects.trace.equals("P0I")||`, 1)
	testNativePrivateSetterFixture(t, f, "AnonymousInitOwner", "AnonymousInitDriver", "6:anonymous:initializer:callback:identity:owner\n")
}

func TestNativeAnonymousPostSuperInitializerFailureKeepsOrderedPartialStoresRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousInitializerFixture, `static String trace="";`, `static String trace="";static AnonymousInitParent published;static Object observe(Object value){trace+="I";if(value==null)throw error;return value;}`, 1)
	f = strings.Replace(f, `this.seed=seed;`, `this.seed=seed;AnonymousInitEffects.published=this;`, 1)
	f = strings.Replace(f, `Object ref=captured;`, `Object ref=AnonymousInitEffects.observe(captured);`, 1)
	f = strings.Replace(f, `new Object[]{null,token}`, `new Object[]{token}`, 1)
	f = strings.Replace(f, `!AnonymousInitEffects.trace.equals("P0")||`, `!AnonymousInitEffects.trace.equals("P0I")||`, 1)
	f = strings.Replace(f, `System.out.println(rows+`, `AnonymousInitEffects.trace="";AnonymousInitEffects.published=null;try{new AnonymousInitOwner().make(seed,null,42.0);throw new AssertionError("missing initializer failure");}catch(RuntimeException e){AnonymousInitParent partial=AnonymousInitEffects.published;if(e!=AnonymousInitEffects.error||partial==null||partial.seed!=seed||partial.first()!=-1||partial.wide()!=42.0||partial.alias()!=null||partial.character()!=0||partial.enabled()||!AnonymousInitEffects.trace.equals("P0I"))throw new AssertionError("original partial store/exception order");}System.out.println(rows+`, 1)
	testNativePrivateSetterFixture(t, f, "AnonymousInitOwner", "AnonymousInitDriver", "3:anonymous:initializer:callback:identity:owner\n")
}
