package javaclassparser

import "testing"

// The original callee observes the actual JVM component and writes through the
// same array. Recompiling a generic varargs invocation from elements can silently
// synthesize String[] instead of the original Object[], adding ArrayStoreException.
func TestNativeGenericVarargsRetainsOriginalArrayComponentRoundTrip(t *testing.T) {
	const f = `class GenericArrayObserver{static <T> String inspect(T... items){Object[] a=items;try{a[0]=Integer.valueOf(3);return a.getClass().getName()+":"+a[0]+":"+a[1];}catch(ArrayStoreException e){return "wrong-array-component:"+a.getClass().getName();}}}class GenericArrayCaller{static String run(boolean first){return first?GenericArrayObserver.inspect(new Object[]{"a","b"}):GenericArrayObserver.inspect(new Object[]{"c","d"});}}class GenericArrayDriver{public static void main(String[]args){for(boolean b:new boolean[]{true,false}){String s=GenericArrayCaller.run(b);String expected="[Ljava.lang.Object;:3:"+(b?"b":"d");if(!s.equals(expected))throw new AssertionError("original allocation component:"+s);System.out.println(s);}}}`
	testNativeIndependentFamilyFixture(t, f, []string{"GenericArrayCaller"}, "GenericArrayDriver", "[Ljava.lang.Object;:3:b\n[Ljava.lang.Object;:3:d\n", nativeLexicalExactSignatures)
}
