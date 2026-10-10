package javaclassparser

import "testing"

// Enum constructor descriptors include name and ordinal; their original
// Signature describes only source arguments. These declarations affect generic
// overload/functional binding even though JVM descriptors retain their erasure.
func TestNativeEnumConstructorGenericOverloadsRoundTrip(t *testing.T) {
	const f = `enum SignatureChoice{LEFT(java.util.Arrays.asList("a","bb")),RIGHT((java.util.Collection<String>)java.util.Arrays.asList("ccc"));final String result;SignatureChoice(java.util.List<String> values){result="L"+values.get(1);}SignatureChoice(java.util.Collection<String> values){result="C"+values.iterator().next();}}class SignatureDriver{public static void main(String[]args)throws Exception{if(!SignatureChoice.LEFT.result.equals("Lbb")||!SignatureChoice.RIGHT.result.equals("Cccc"))throw new AssertionError("original constructor overloads");System.out.println("signature:generic:overload");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"SignatureChoice"}, "SignatureDriver", "signature:generic:overload\n", nativeLexicalExactSignatures)
}
func TestNativeEnumConstructorWideNestedGenericArrayRoundTrip(t *testing.T) {
	const f = `class WideSignatureEffects{static final java.util.List<java.util.Map<String,? super Integer>> ROWS=java.util.Arrays.<java.util.Map<String,? super Integer>>asList(new java.util.HashMap<String,Object>());static final String[] TAGS={"a","bbb"};}enum WideSignatureChoice{LEFT(0x1020304050607080L,0.25,WideSignatureEffects.ROWS,WideSignatureEffects.TAGS);final long result;WideSignatureChoice(long value,double ratio,java.util.List<java.util.Map<String,? super Integer>> rows,String... tags){rows.get(0).put("count",tags.length);result=value+(long)(ratio*4)+tags[1].length()+((Number)rows.get(0).get("count")).intValue();}}class WideSignatureDriver{public static void main(String[]args){if(WideSignatureChoice.LEFT.result!=0x1020304050607086L)throw new AssertionError("wide/generic/varargs binding");System.out.println("signature:wide:nested:varargs");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"WideSignatureChoice"}, "WideSignatureDriver", "signature:wide:nested:varargs\n", nativeLexicalExactSignatures)
}
func TestNativeEnumConstructorOwnFormalFunctionalBindingRoundTrip(t *testing.T) {
	const f = `class FormalSignatureEffects{static final java.util.function.Function<Integer,String> RENDER=(Integer value)->"v"+value;}enum FormalSignatureChoice{LEFT(7,FormalSignatureEffects.RENDER);final String result;<T>FormalSignatureChoice(T value,java.util.function.Function<T,String> render){result=render.apply(value);}}class FormalSignatureDriver{public static void main(String[]args){if(!FormalSignatureChoice.LEFT.result.equals("v7"))throw new AssertionError("constructor-owned formal and SAM");System.out.println("signature:own-formal:functional");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"FormalSignatureChoice"}, "FormalSignatureDriver", "signature:own-formal:functional\n", nativeLexicalExactSignatures)
}
