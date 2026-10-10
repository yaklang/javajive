package javaclassparser

import (
	"strings"
	"testing"
)

const nativeRootPrivateConstructorFixture = `class RootBridgeEffects{static String trace="";static final RuntimeException error=new RuntimeException("original");static boolean fail;}
class RootBridgePacket {
 final Object value;private RootBridgePacket(Object value){RootBridgeEffects.trace+="P";if(RootBridgeEffects.fail)throw RootBridgeEffects.error;this.value=value;}
 static class Member extends RootBridgePacket {Member(Object value){super(value);RootBridgeEffects.trace+="M";}}
 static RootBridgePacket make(Object value){return new Member(value);}
}
class RootBridgeDriver {public static void main(String[]args){Object token=new Object();int rows=0;for(Object v:new Object[]{null,token}){RootBridgeEffects.trace="";RootBridgePacket p=RootBridgePacket.make(v);if(p.value!=v||!RootBridgeEffects.trace.equals("PM")||p.getClass().getDeclaringClass()!=RootBridgePacket.class)throw new AssertionError("value/order/owner");rows++;}RootBridgeEffects.fail=true;RootBridgeEffects.trace="";try{RootBridgePacket.make(token);throw new AssertionError("failure");}catch(RuntimeException e){if(e!=RootBridgeEffects.error||!RootBridgeEffects.trace.equals("P"))throw new AssertionError("failure identity/order");}System.out.println(rows+":private-root:identity:order:owner");}}`

func TestNativeRootPrivateConstructorBridgeRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeRootPrivateConstructorFixture, "RootBridgePacket", "RootBridgeDriver", "2:private-root:identity:order:owner\n")
}

const nativeRootWideConstructorFixture = `class WideBridgeEffects {static String trace="";static int fail;static final RuntimeException error=new RuntimeException("original");static long left(long v){trace+="L";if(fail==1)throw error;return v;}static Object value(Object v){trace+="V";if(fail==2)throw error;return v;}static double right(double v){trace+="R";if(fail==3)throw error;return v;}}
class WideBridgePacket {
 final long left;final Object value;final double right;final String selected;
 private WideBridgePacket(long l,Object v,double r){WideBridgeEffects.trace+="P";left=l;value=v;right=r;selected="primitive";}
 private WideBridgePacket(Long l,Object v,Double r){throw new AssertionError("wrong overload");}
 static class Member extends WideBridgePacket {Member(long l,Object v,double r){super(WideBridgeEffects.left(l),WideBridgeEffects.value(v),WideBridgeEffects.right(r));WideBridgeEffects.trace+="M";}}
 static WideBridgePacket make(long l,Object v,double r){return new Member(l,v,r);}
}
class WideBridgeDriver {public static void main(String[]args){Object token=new Object();int rows=0;for(long l:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(double r:new double[]{-0.0,Double.NaN,Double.POSITIVE_INFINITY}){WideBridgeEffects.trace="";WideBridgePacket p=WideBridgePacket.make(l,token,r);if(p.left!=l||p.value!=token||Double.doubleToRawLongBits(p.right)!=Double.doubleToRawLongBits(r)||!p.selected.equals("primitive")||!WideBridgeEffects.trace.equals("LVRPM")||p.getClass().getDeclaringClass()!=WideBridgePacket.class)throw new AssertionError("wide/overload/order/owner");rows++;}for(int f=1;f<=3;f++){WideBridgeEffects.trace="";WideBridgeEffects.fail=f;try{WideBridgePacket.make(1,token,2);throw new AssertionError("missing failure");}catch(RuntimeException e){if(e!=WideBridgeEffects.error||!WideBridgeEffects.trace.equals(f==1?"L":f==2?"LV":"LVR"))throw new AssertionError("failure identity/order");}}System.out.println(rows+":wide:overload:order:owner");}}
`

func TestNativeRootWidePrivateConstructorBridgeRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeRootWideConstructorFixture, "WideBridgePacket", "WideBridgeDriver", "9:wide:overload:order:owner\n")
}

const nativeRootGenericConstructorFixture = `class GenericBridgeEffects {static String trace="";static boolean fail;static final java.io.IOException error=new java.io.IOException("original");}
class GenericBridgePacket<T> {
 final T[] values;
 private GenericBridgePacket(T... values)throws java.io.IOException{GenericBridgeEffects.trace+="P";if(GenericBridgeEffects.fail)throw GenericBridgeEffects.error;this.values=values;}
 static class Member<U> extends GenericBridgePacket<U>{Member(U[] values)throws java.io.IOException{super(values);GenericBridgeEffects.trace+="M";}}
 static <V> GenericBridgePacket<V> make(V[] values)throws java.io.IOException{return new Member<V>(values);}
}
class GenericBridgeDriver{public static void main(String[]args)throws Exception{Object token=new Object();Object[] objects=new Object[]{token,null};String[] strings=new String[]{"original"};int rows=0;for(Object[] a:new Object[][]{null,objects,strings}){GenericBridgeEffects.trace="";GenericBridgePacket<?> p=GenericBridgePacket.make(a);if(p.values!=a||!GenericBridgeEffects.trace.equals("PM")||p.getClass().getDeclaringClass()!=GenericBridgePacket.class)throw new AssertionError("generic array identity/order/owner");rows++;}boolean varargs=false;int bridges=0;for(java.lang.reflect.Constructor<?> c:GenericBridgePacket.class.getDeclaredConstructors()){if(c.isSynthetic()){bridges++;if(c.getParameterTypes().length!=2||c.getExceptionTypes().length!=1||c.getExceptionTypes()[0]!=java.io.IOException.class)throw new AssertionError("bridge ABI");}else varargs=c.isVarArgs();}if(!varargs||bridges!=1)throw new AssertionError("varargs/bridge declaration");GenericBridgeEffects.fail=true;GenericBridgeEffects.trace="";try{GenericBridgePacket.make(objects);throw new AssertionError("missing failure");}catch(java.io.IOException e){if(e!=GenericBridgeEffects.error||!GenericBridgeEffects.trace.equals("P"))throw new AssertionError("checked failure identity/order");}System.out.println(rows+":generic:varargs:checked:owner");}}
`

func TestNativeRootGenericPrivateConstructorBridgeRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeRootGenericConstructorFixture, "GenericBridgePacket", "GenericBridgeDriver", "3:generic:varargs:checked:owner\n")
}

func TestNativeRootAndMemberPrivateConstructorsShareOriginalMarker(t *testing.T) {
	fixture := strings.ReplaceAll(nativeRootPrivateConstructorFixture, "Member(Object value)", "private Member(Object value)")
	testNativePrivateSetterFixture(t, fixture, "RootBridgePacket", "RootBridgeDriver", "2:private-root:identity:order:owner\n")
}

func TestNativeRootPrivateConstructorBridgeIgnoresSourceSpelling(t *testing.T) {
	fixture := strings.ReplaceAll(nativeRootPrivateConstructorFixture, "RootBridgePacket", "IndependentConstructorSignal")
	fixture = strings.ReplaceAll(fixture, "value", "payload")
	testNativePrivateSetterFixture(t, fixture, "IndependentConstructorSignal", "RootBridgeDriver", "2:private-root:identity:order:owner\n")
}
