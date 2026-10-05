package javaclassparser

import (
	"strings"
	"testing"
)

const nativeAnonymousGenericSuperFixture = `class WideningParent<T>{final T value;final Object observed;WideningParent(T value){this.value=value;observed=owner();}Object owner(){return null;}}
class WideningOwner{Object make(String value){return new WideningParent<String>(value){Object owner(){return WideningOwner.this;}};}}
class WideningOracle{public static void main(String[]args){WideningOwner owner=new WideningOwner();for(String v:new String[]{null,"",new String("token")}){WideningParent<?> p=(WideningParent<?>)owner.make(v);if(p.value!=v||p.observed!=owner)throw new AssertionError("generic super identity/capture");}System.out.println("super:generic:identity:capture");}}`

func TestNativeAnonymousGenericSuperParameterWideningRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeAnonymousGenericSuperFixture, []string{"WideningOwner"}, "WideningOracle", "super:generic:identity:capture\n")
}

func TestNativeAnonymousGenericSuperParameterWideningRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeAnonymousGenericSuperFixture, "WideningOwner", "OtherForwardingScope")
	f = strings.ReplaceAll(f, "WideningParent", "IndependentGenericParent")
	testNativeIndependentFamilyFixture(t, f, []string{"OtherForwardingScope"}, "WideningOracle", "super:generic:identity:capture\n")
}

func TestNativeAnonymousGenericBoundSuperParameterWideningRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousGenericSuperFixture, "class WideningParent<T>", "class WideningToken{} class NarrowToken extends WideningToken{} class WideningParent<T extends WideningToken>", 1)
	f = strings.ReplaceAll(f, "String value", "NarrowToken value")
	f = strings.ReplaceAll(f, "WideningParent<String>", "WideningParent<NarrowToken>")
	f = strings.Replace(f, `String v:new String[]{null,"",new String("token")}`, `NarrowToken v:new NarrowToken[]{null,new NarrowToken(),new NarrowToken()}`, 1)
	testNativeIndependentFamilyFixture(t, f, []string{"WideningOwner"}, "WideningOracle", "super:generic:identity:capture\n")
}

func TestNativeAnonymousGenericArraySuperParameterWideningRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeAnonymousGenericSuperFixture, "String value", "String[] value")
	f = strings.ReplaceAll(f, "WideningParent<String>", "WideningParent<String[]>")
	f = strings.Replace(f, `String v:new String[]{null,"",new String("token")}`, `String[] v:new String[][]{null,new String[0],new String[]{"token",null}}`, 1)
	testNativeIndependentFamilyFixture(t, f, []string{"WideningOwner"}, "WideningOracle", "super:generic:identity:capture\n")
}

func TestNativeAnonymousGenericCovariantArraySuperParameterWideningRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousGenericSuperFixture, "final T value;", "final T[] value;", 1)
	f = strings.Replace(f, "WideningParent(T value)", "WideningParent(T[] value)", 1)
	f = strings.ReplaceAll(f, "String value", "String[][] value")
	f = strings.ReplaceAll(f, "WideningParent<String>", "WideningParent<String[]>")
	f = strings.Replace(f, `String v:new String[]{null,"",new String("token")}`, `String[][] v:new String[][][]{null,new String[0][],new String[][]{new String[]{"token",null},null}}`, 1)
	testNativeIndependentFamilyFixture(t, f, []string{"WideningOwner"}, "WideningOracle", "super:generic:identity:capture\n")
}

func TestNativeAnonymousGenericSuperWideningKeepsInstantiatedOverloadRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousGenericSuperFixture, "Object owner(){return null;}", `WideningParent(CharSequence other){throw new AssertionError("wrong overload");}Object owner(){return null;}`, 1)
	testNativeIndependentFamilyFixture(t, f, []string{"WideningOwner"}, "WideningOracle", "super:generic:identity:capture\n")
}

func TestNativeAnonymousGenericSuperWideningKeepsParentFailureRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousGenericSuperFixture, "class WideningParent<T>", `class WideningFailure{static final java.io.IOException failure=new java.io.IOException("original");static WideningParent<?> published;}class WideningParent<T>`, 1)
	f = strings.Replace(f, "WideningParent(T value){this.value=value;observed=owner();}", `WideningParent(T value)throws java.io.IOException{this.value=value;observed=owner();WideningFailure.published=this;if(value==null)throw WideningFailure.failure;}`, 1)
	f = strings.Replace(f, "Object make(String value){", "Object make(String value)throws java.io.IOException{", 1)
	f = strings.Replace(f, "main(String[]args){", "main(String[]args)throws Exception{", 1)
	f = strings.Replace(f, `for(String v:new String[]{null,"",new String("token")})`, `for(String v:new String[]{"",new String("token")})`, 1)
	f = strings.Replace(f, `System.out.println("super:generic:identity:capture");`, `try{owner.make(null);throw new AssertionError("missing parent failure");}catch(java.io.IOException e){if(e!=WideningFailure.failure||WideningFailure.published.value!=null||WideningFailure.published.observed!=owner)throw new AssertionError("failure/capture priority");}System.out.println("super:generic:identity:capture");`, 1)
	testNativeIndependentFamilyFixture(t, f, []string{"WideningOwner"}, "WideningOracle", "super:generic:identity:capture\n")
}
