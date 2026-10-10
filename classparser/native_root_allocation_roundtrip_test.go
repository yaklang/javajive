package javaclassparser

import (
	"strings"
	"testing"
)

func nativeRootDirectAllocationFixture() string {
	fixture := strings.Replace(nativeRootPrivateConstructorFixture,
		"Member(Object value){super(value);RootBridgeEffects.trace+=\"M\";}",
		"Member(Object value){super(value);RootBridgeEffects.trace+=\"M\";} static RootBridgePacket direct(Object value){return new RootBridgePacket(value);}", 1)
	return strings.Replace(fixture, "Object token=new Object();int rows=0;", `Object token=new Object();for(Object value:new Object[]{null,token}){RootBridgeEffects.trace="";RootBridgePacket direct=RootBridgePacket.Member.direct(value);if(direct.value!=value||direct.getClass()!=RootBridgePacket.class||!RootBridgeEffects.trace.equals("P"))throw new AssertionError("allocation identity/type/order");}int rows=0;`, 1)
}

func TestNativeRootPrivateAllocationBridgeRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeRootDirectAllocationFixture(), "RootBridgePacket", "RootBridgeDriver", "2:private-root:identity:order:owner\n")
}

func TestNativeRootPrivateAllocationBridgeIgnoresSourceSpelling(t *testing.T) {
	fixture := strings.ReplaceAll(nativeRootDirectAllocationFixture(), "RootBridgePacket", "OtherCreationPacket")
	fixture = strings.ReplaceAll(fixture, "value", "payload")
	testNativePrivateSetterFixture(t, fixture, "OtherCreationPacket", "RootBridgeDriver", "2:private-root:identity:order:owner\n")
}

func TestNativeRootWidePrivateAllocationBridgeRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeRootWideConstructorFixture,
		"static class Member extends WideBridgePacket {",
		"static class Member extends WideBridgePacket {static WideBridgePacket direct(long l,Object v,double r){return new WideBridgePacket(WideBridgeEffects.left(l),WideBridgeEffects.value(v),WideBridgeEffects.right(r));}", 1)
	fixture = strings.Replace(fixture, "Object token=new Object();int rows=0;", `Object token=new Object();for(long l:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(double r:new double[]{-0.0,Double.NaN,Double.POSITIVE_INFINITY}){WideBridgeEffects.trace="";WideBridgePacket direct=WideBridgePacket.Member.direct(l,token,r);if(direct.left!=l||direct.value!=token||Double.doubleToRawLongBits(direct.right)!=Double.doubleToRawLongBits(r)||direct.getClass()!=WideBridgePacket.class||!direct.selected.equals("primitive")||!WideBridgeEffects.trace.equals("LVRP"))throw new AssertionError("allocation wide/overload/identity/order");}for(int f=1;f<=3;f++){WideBridgeEffects.fail=f;WideBridgeEffects.trace="";try{WideBridgePacket.Member.direct(1,token,2);throw new AssertionError("missing allocation failure");}catch(RuntimeException e){if(e!=WideBridgeEffects.error||!WideBridgeEffects.trace.equals(f==1?"L":f==2?"LV":"LVR"))throw new AssertionError("allocation failure identity/order");}}WideBridgeEffects.fail=0;int rows=0;`, 1)
	testNativePrivateSetterFixture(t, fixture, "WideBridgePacket", "WideBridgeDriver", "9:wide:overload:order:owner\n")
}

func TestNativeRootGenericPrivateAllocationBridgeRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeRootGenericConstructorFixture,
		"static class Member<U> extends GenericBridgePacket<U>{",
		"static class Member<U> extends GenericBridgePacket<U>{static <V> GenericBridgePacket<V> direct(V[] values)throws java.io.IOException{return new GenericBridgePacket<V>(values);}", 1)
	fixture = strings.Replace(fixture, "int rows=0;", `for(Object[] a:new Object[][]{null,objects,strings}){GenericBridgeEffects.trace="";GenericBridgePacket<?> direct=GenericBridgePacket.Member.direct(a);if(direct.values!=a||direct.getClass()!=GenericBridgePacket.class||!GenericBridgeEffects.trace.equals("P"))throw new AssertionError("allocation generic/array/order");}GenericBridgeEffects.fail=true;GenericBridgeEffects.trace="";try{GenericBridgePacket.Member.direct(objects);throw new AssertionError("allocation missing checked failure");}catch(java.io.IOException e){if(e!=GenericBridgeEffects.error||!GenericBridgeEffects.trace.equals("P"))throw new AssertionError("allocation checked identity/order");}GenericBridgeEffects.fail=false;int rows=0;`, 1)
	testNativePrivateSetterFixture(t, fixture, "GenericBridgePacket", "GenericBridgeDriver", "3:generic:varargs:checked:owner\n")
}

const nativeRootFinalAllocationFixture = `class FinalCreationEffects {static String trace="";static boolean fail;static final RuntimeException error=new RuntimeException("original");}
final class FinalCreationPacket {
 final Object payload;private FinalCreationPacket(Object v){FinalCreationEffects.trace+="P";if(FinalCreationEffects.fail)throw FinalCreationEffects.error;payload=v;}
 static class Factory {static FinalCreationPacket create(Object v){return new FinalCreationPacket(v);}}
 class Inner {FinalCreationPacket create(Object v){return new FinalCreationPacket(v);}}
 static FinalCreationPacket make(Object v){return Factory.create(v);}
 Object nested(Object v){return new Inner().create(v).payload;}
}
class FinalCreationDriver {public static void main(String[]args){Object token=new Object();int rows=0;for(Object v:new Object[]{null,token}){FinalCreationEffects.trace="";FinalCreationPacket p=FinalCreationPacket.make(v);if(p.payload!=v||p.getClass()!=FinalCreationPacket.class||!FinalCreationEffects.trace.equals("P"))throw new AssertionError("static allocation");FinalCreationEffects.trace="";if(p.nested(v)!=v||!FinalCreationEffects.trace.equals("P"))throw new AssertionError("nonstatic allocation");rows++;}FinalCreationEffects.fail=true;FinalCreationEffects.trace="";try{FinalCreationPacket.make(token);throw new AssertionError("missing constructor failure");}catch(RuntimeException e){if(e!=FinalCreationEffects.error||!FinalCreationEffects.trace.equals("P"))throw new AssertionError("constructor failure identity/order");}System.out.println(rows+":final:root:static:nonstatic:allocation");}}
`

func TestNativeRootFinalStaticAndNonstaticAllocationsRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeRootFinalAllocationFixture, "FinalCreationPacket", "FinalCreationDriver", "2:final:root:static:nonstatic:allocation\n")
}
