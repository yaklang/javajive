package javaclassparser

import (
	"strings"
	"testing"
)

const nativeAnonymousArrayReadFixture = `class ArrayReadTrace {static String trace="";static ArrayReadParent published;static final RuntimeException error=new RuntimeException("index");static int index(int n){trace+="D";if(n==9)throw error;return n;}}
abstract class ArrayReadParent {ArrayReadParent(int n){ArrayReadTrace.trace+="P";ArrayReadTrace.published=this;if(first()!=0)throw new AssertionError("parent sees default field");}abstract int first();abstract Object view();}
class ArrayReadOwner {ArrayReadParent make(byte[] elements,int position){return new ArrayReadParent(position){int before=17;byte item=elements[ArrayReadTrace.index(position)];int first(){return before;}Object view(){return item;}};}}
class ArrayReadDriver {static boolean same(Object a,Object b){if(a instanceof Float&&b instanceof Float)return Float.floatToRawIntBits(((Float)a).floatValue())==Float.floatToRawIntBits(((Float)b).floatValue());if(a instanceof Double&&b instanceof Double)return Double.doubleToRawLongBits(((Double)a).doubleValue())==Double.doubleToRawLongBits(((Double)b).doubleValue());return java.util.Objects.equals(a,b);}public static void main(String[]args){byte[] input=new byte[]{-128,0,127};for(int i=0;i<3;i++){ArrayReadTrace.trace="";ArrayReadParent p=new ArrayReadOwner().make(input,i);if(p.first()!=17||!same(p.view(),java.lang.reflect.Array.get(input,i))||!ArrayReadTrace.trace.equals("PD")||!p.getClass().getName().equals("ArrayReadOwner$1"))throw new AssertionError("original array value/type/effects/owner");}for(int kind=0;kind<5;kind++){byte[] inputOrNull=kind<2?null:input;int n=kind==0?0:kind==1||kind==4?9:kind==2?-1:3;ArrayReadTrace.trace="";ArrayReadTrace.published=null;try{new ArrayReadOwner().make(inputOrNull,n);throw new AssertionError("missing array/index failure");}catch(RuntimeException e){if(n==9?e!=ArrayReadTrace.error:inputOrNull==null?!(e instanceof NullPointerException):!(e instanceof ArrayIndexOutOfBoundsException))throw new AssertionError("exception kind/identity");ArrayReadParent partial=ArrayReadTrace.published;Object defaultValue=java.lang.reflect.Array.get(java.lang.reflect.Array.newInstance(input.getClass().getComponentType(),1),0);if(partial==null||partial.first()!=17||!same(partial.view(),defaultValue)||!ArrayReadTrace.trace.equals("PD"))throw new AssertionError("original partial fields and index before array null/bounds check");}}System.out.println("8:arrayread:rawvalue:index:exception:partial:order");}}
`

func TestNativeAnonymousInitializerPrimitiveArrayReadRoundTrip(t *testing.T) {
	literals := map[string]string{"boolean": "false,true,false", "byte": "-128,0,127", "char": "0,32768,65535", "short": "-32768,0,32767", "int": "Integer.MIN_VALUE,0,Integer.MAX_VALUE", "long": "Long.MIN_VALUE,0,Long.MAX_VALUE", "float": "Float.intBitsToFloat(0x7fc01234),-0.0f,Float.intBitsToFloat(1)", "double": "Double.longBitsToDouble(0x7ff8000000001234L),-0.0,Double.longBitsToDouble(1)"}
	for _, kind := range []string{"boolean", "byte", "char", "short", "int", "long", "float", "double"} {
		t.Run(kind, func(t *testing.T) {
			f := strings.ReplaceAll(nativeAnonymousArrayReadFixture, "byte[]", kind+"[]")
			f = strings.Replace(f, "byte item=", kind+" item=", 1)
			f = strings.Replace(f, "new "+kind+"[]{-128,0,127}", "new "+kind+"[]{"+literals[kind]+"}", 1)
			testNativePrivateSetterFixture(t, f, "ArrayReadOwner", "ArrayReadDriver", "8:arrayread:rawvalue:index:exception:partial:order\n")
		})
	}
}
func TestNativeAnonymousInitializerReferenceArrayReadRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeAnonymousArrayReadFixture, "byte[]", "Object[]")
	f = strings.Replace(f, "byte item=", "Object item=", 1)
	f = strings.Replace(f, "new Object[]{-128,0,127}", "new Object[]{null,new Object(),new Object()}", 1)
	f = strings.Replace(f, "!same(p.view(),java.lang.reflect.Array.get(input,i))", "p.view()!=java.lang.reflect.Array.get(input,i)", 1)
	testNativePrivateSetterFixture(t, f, "ArrayReadOwner", "ArrayReadDriver", "8:arrayread:rawvalue:index:exception:partial:order\n")
}
func TestNativeAnonymousInitializerNestedArrayReadRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeAnonymousArrayReadFixture, "byte[]", "int[][]")
	f = strings.Replace(f, "byte item=", "int[] item=", 1)
	f = strings.Replace(f, "new int[][]{-128,0,127}", "new int[][]{null,new int[]{17},new int[0]}", 1)
	f = strings.Replace(f, "!same(p.view(),java.lang.reflect.Array.get(input,i))", "p.view()!=java.lang.reflect.Array.get(input,i)", 1)
	testNativePrivateSetterFixture(t, f, "ArrayReadOwner", "ArrayReadDriver", "8:arrayread:rawvalue:index:exception:partial:order\n")
}

func TestNativeAnonymousInitializerGenericArrayReadPreservesLexicalBindingRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousArrayReadFixture, "class ArrayReadOwner {", "class ArrayReadOwner<Q extends Number> {", 1)
	f = strings.Replace(f, "make(byte[] elements", "make(Q[] elements", 1)
	f = strings.Replace(f, "byte item=", "Q item=", 1)
	f = strings.ReplaceAll(f, "byte[] input", "Number[] input")
	f = strings.Replace(f, "new byte[]{-128,0,127}", "new Number[]{Integer.valueOf(Integer.MIN_VALUE),Float.valueOf(-0.0f),Double.valueOf(Double.longBitsToDouble(0x7ff8000000001234L))}", 1)
	f = strings.ReplaceAll(f, "new ArrayReadOwner()", "new ArrayReadOwner<Number>()")
	f = strings.Replace(f, "!same(p.view(),java.lang.reflect.Array.get(input,i))", "p.view()!=java.lang.reflect.Array.get(input,i)", 1)
	testNativePrivateSetterFixture(t, f, "ArrayReadOwner", "ArrayReadDriver", "8:arrayread:rawvalue:index:exception:partial:order\n")
}
