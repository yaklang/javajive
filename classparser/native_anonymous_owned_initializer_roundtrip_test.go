package javaclassparser

import (
	"strings"
	"testing"
)

func TestNativeAnonymousInitializerReadsEarlierOwnFieldsRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousInitializerFixture, `Object ref=captured;`, `Object ref=captured;int mirror=this.initial;double mirroredWide=this.saved;Object mirroredRef=this.ref;`, 1)
	f = strings.Replace(f, `main(String[]args){`, `main(String[]args)throws Exception{`, 1)
	f = strings.Replace(f, `rows++;`, `String[] names={"mirror","mirroredWide","mirroredRef"};Object[] expected={-1,d,v};for(int i=0;i<names.length;i++){java.lang.reflect.Field field=p.getClass().getDeclaredField(names[i]);field.setAccessible(true);if(!java.util.Objects.equals(field.get(p),expected[i]))throw new AssertionError("earlier field identity/bits: "+names[i]);}rows++;`, 1)
	testNativePrivateSetterFixture(t, f, "AnonymousInitOwner", "AnonymousInitDriver", "6:anonymous:initializer:callback:identity:owner\n")
}
func TestNativeAnonymousInitializerReadsOwnDefaultFieldsRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousInitializerFixture, `Object ref=captured;`, `Object ref=captured;int before=this.later;volatile int later=42;Object defaultRef=this.after;Object after=captured;`, 1)
	f = strings.Replace(f, `main(String[]args){`, `main(String[]args)throws Exception{`, 1)
	f = strings.Replace(f, `rows++;`, `String[] names={"before","later","defaultRef","after"};Object[] expected={0,42,null,v};for(int i=0;i<names.length;i++){java.lang.reflect.Field field=p.getClass().getDeclaredField(names[i]);field.setAccessible(true);if(!java.util.Objects.equals(field.get(p),expected[i]))throw new AssertionError("default read/order: "+names[i]);}rows++;`, 1)
	testNativePrivateSetterFixture(t, f, "AnonymousInitOwner", "AnonymousInitDriver", "6:anonymous:initializer:callback:identity:owner\n")
}
func TestNativeAnonymousInitializerReadsParentMutatedOwnFieldRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousInitializerFixture, `Object ref=captured;`, `Object ref=captured;int before=this.later;volatile int later=42;`, 1)
	f = strings.Replace(f, `AnonymousInitEffects.trace+="P"+first();`, `AnonymousInitEffects.trace+="P"+first();try{java.lang.reflect.Field field=getClass().getDeclaredField("later");field.setAccessible(true);field.setInt(this,23);}catch(ReflectiveOperationException e){throw new AssertionError(e);}`, 1)
	f = strings.Replace(f, `main(String[]args){`, `main(String[]args)throws Exception{`, 1)
	f = strings.Replace(f, `rows++;`, `java.lang.reflect.Field before=p.getClass().getDeclaredField("before"),later=p.getClass().getDeclaredField("later");before.setAccessible(true);later.setAccessible(true);if(before.getInt(p)!=23||later.getInt(p)!=42)throw new AssertionError("parent-mutated actual field/order");rows++;`, 1)
	testNativePrivateSetterFixture(t, f, "AnonymousInitOwner", "AnonymousInitDriver", "6:anonymous:initializer:callback:identity:owner\n")
}
func TestNativeAnonymousInitializerReadsEarlierOwnFieldsRenamedRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousInitializerFixture, `Object ref=captured;`, `Object ref=captured;int mirror=this.initial;Object mirroredRef=this.ref;Object alias(){if(mirror!=-1||mirroredRef!=ref)throw new AssertionError("earlier read identity");return ref;}`, 1)
	f = strings.Replace(f, `Object alias(){return ref;}`, ``, 1)
	f = strings.ReplaceAll(strings.ReplaceAll(f, "AnonymousInitOwner", "IndependentOwnFieldScope"), "captured", "differentInput")
	testNativePrivateSetterFixture(t, f, "IndependentOwnFieldScope", "AnonymousInitDriver", "6:anonymous:initializer:callback:identity:owner\n")
}
