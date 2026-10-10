package javaclassparser

import (
	"strings"
	"testing"
)

const nativeAnonymousArrayInitializerFixture = `class ArrayInitTrace {static String trace="";static final RuntimeException failure=new RuntimeException("producer");static ArrayInitParent published;static int dimension(int n){trace+="D";if(n==9)throw failure;return n;}}
abstract class ArrayInitParent {ArrayInitParent(int n){ArrayInitTrace.trace+="P";ArrayInitTrace.published=this;if(size()!=0)throw new AssertionError("parent sees defaults");}abstract int size();abstract Object view();}
class ArrayInitOwner {ArrayInitParent make(int input){return new ArrayInitParent(input){int first=17;byte[] bytes=new byte[ArrayInitTrace.dimension(input)];int measured=bytes.length;int size(){return measured;}Object view(){return bytes;}};}}
class ArrayInitDriver {public static void main(String[]args)throws Exception{for(int n:new int[]{0,1,7}){ArrayInitTrace.trace="";ArrayInitParent p=new ArrayInitOwner().make(n);if(p.size()!=n||java.lang.reflect.Array.getLength(p.view())!=n||!p.view().getClass().equals(byte[].class)||!ArrayInitTrace.trace.equals("PD")||!p.getClass().getName().equals("ArrayInitOwner$1"))throw new AssertionError("array type/length/effect/owner");for(int i=0;i<n;i++)if(java.lang.reflect.Array.getByte(p.view(),i)!=0)throw new AssertionError("zero array");}for(int n:new int[]{-1,9}){ArrayInitTrace.trace="";ArrayInitTrace.published=null;try{new ArrayInitOwner().make(n);throw new AssertionError("missing failure");}catch(RuntimeException failure){if(n==-1?!(failure instanceof NegativeArraySizeException):failure!=ArrayInitTrace.failure)throw new AssertionError("failure kind/identity");ArrayInitParent partial=ArrayInitTrace.published;java.lang.reflect.Field first=partial.getClass().getDeclaredField("first");first.setAccessible(true);if(first.getInt(partial)!=17||partial.view()!=null||partial.size()!=0||!ArrayInitTrace.trace.equals("PD"))throw new AssertionError("ordered partial/default fields");}}System.out.println("5:array:dimension:identity:default:partial:order");}}
`

func TestNativeAnonymousInitializerPrimitiveArrayRoundTrip(t *testing.T) {
	for _, kind := range []string{"boolean", "byte", "char", "short", "int", "long", "float", "double"} {
		t.Run(kind, func(t *testing.T) {
			f := strings.ReplaceAll(nativeAnonymousArrayInitializerFixture, "byte[]", kind+"[]")
			f = strings.Replace(f, "new byte[", "new "+kind+"[", 1)
			f = strings.Replace(f, "java.lang.reflect.Array.getByte(p.view(),i)!=0", "!java.util.Objects.equals(java.lang.reflect.Array.get(p.view(),i),java.lang.reflect.Array.get(java.lang.reflect.Array.newInstance("+kind+".class,1),0))", 1)
			testNativePrivateSetterFixture(t, f, "ArrayInitOwner", "ArrayInitDriver", "5:array:dimension:identity:default:partial:order\n")
		})
	}
}
func TestNativeAnonymousInitializerReferenceArrayRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeAnonymousArrayInitializerFixture, "byte[]", "Object[]")
	f = strings.Replace(f, "new byte[", "new Object[", 1)
	f = strings.Replace(f, `java.lang.reflect.Array.getByte(p.view(),i)!=0`, `java.lang.reflect.Array.get(p.view(),i)!=null`, 1)
	testNativePrivateSetterFixture(t, f, "ArrayInitOwner", "ArrayInitDriver", "5:array:dimension:identity:default:partial:order\n")
}
func TestNativeAnonymousInitializerArrayComponentRankRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeAnonymousArrayInitializerFixture, "byte[]", "int[][]")
	f = strings.Replace(f, `new byte[ArrayInitTrace.dimension(input)]`, `new int[ArrayInitTrace.dimension(input)][]`, 1)
	f = strings.Replace(f, `java.lang.reflect.Array.getByte(p.view(),i)!=0`, `java.lang.reflect.Array.get(p.view(),i)!=null`, 1)
	testNativePrivateSetterFixture(t, f, "ArrayInitOwner", "ArrayInitDriver", "5:array:dimension:identity:default:partial:order\n")
}
func TestNativeAnonymousInitializerPrimitiveArrayRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(strings.ReplaceAll(nativeAnonymousArrayInitializerFixture, "ArrayInitOwner", "DifferentArrayScope"), "input", "dimensionInput")
	testNativePrivateSetterFixture(t, f, "DifferentArrayScope", "ArrayInitDriver", "5:array:dimension:identity:default:partial:order\n")
}
