package javaclassparser

import (
	"strings"
	"testing"
)

func nativeProtectedStaticFieldSources(root string) map[string]string {
	effects := `package probe.base;public class FieldEffects{public static String trace="";}`
	parent := `package probe.base;public class FieldParent{protected static Object value=initialize();protected static volatile long number;protected static double bits;private static Object initialize(){FieldEffects.trace+="P";return new Object();}public static void set(Object token,long n,double d){value=token;number=n;bits=d;}}`
	middle := `package probe.base;public class FieldMiddle extends FieldParent{static{FieldEffects.trace+="M";}}`
	source := `package probe.access;public class ProtectedFieldOwner extends probe.base.FieldMiddle{static{probe.base.FieldEffects.trace+="S";}static class Reader{Object first(){return ProtectedFieldOwner.value;}Object second(){return ProtectedFieldOwner.value;}long number(){return ProtectedFieldOwner.number;}double bits(){return ProtectedFieldOwner.bits;}}}
class ProtectedFieldDriver{public static void main(String[]args){probe.base.FieldEffects.trace="";ProtectedFieldOwner.Reader reader=new ProtectedFieldOwner.Reader();Object first=reader.first();if(first==null||!probe.base.FieldEffects.trace.equals("PMS")||reader.second()!=first)throw new AssertionError("declaring and accessor class initialization/order/shared identity");int rows=0;for(Object token:new Object[]{null,new Object()})for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){double d=Double.longBitsToDouble(n);probe.base.FieldParent.set(token,n,d);if(reader.first()!=token||reader.second()!=token||reader.number()!=n||Double.doubleToRawLongBits(reader.bits())!=n)throw new AssertionError("mutable read/volatile width/FP payload");rows++;}System.out.println(rows+":protected:static:field:binding:initialization:identity");}}`
	source = strings.ReplaceAll(source, "ProtectedFieldOwner", root)
	return map[string]string{"probe/base/FieldEffects.java": effects, "probe/base/FieldParent.java": parent, "probe/base/FieldMiddle.java": middle, "probe/access/" + root + ".java": source}
}
func TestNativeProtectedStaticFieldRoundTrip(t *testing.T) {
	testNativePrivateSetterSourceFixture(t, nativeProtectedStaticFieldSources("ProtectedFieldOwner"), "probe/access/ProtectedFieldOwner", "probe.access.ProtectedFieldDriver", "10:protected:static:field:binding:initialization:identity\n")
}
func TestNativeProtectedStaticFieldRenamedRoundTrip(t *testing.T) {
	testNativePrivateSetterSourceFixture(t, nativeProtectedStaticFieldSources("OtherProtectedFieldScope"), "probe/access/OtherProtectedFieldScope", "probe.access.ProtectedFieldDriver", "10:protected:static:field:binding:initialization:identity\n")
}
func TestNativeProtectedStaticFieldInitializationFailureRoundTrip(t *testing.T) {
	sources := nativeProtectedStaticFieldSources("ProtectedFieldOwner")
	sources["probe/base/FieldEffects.java"] = `package probe.base;public class FieldEffects{public static String trace="";public static final RuntimeException failure=new RuntimeException("original");}`
	sources["probe/base/FieldParent.java"] = strings.Replace(sources["probe/base/FieldParent.java"], "return new Object();", "throw FieldEffects.failure;", 1)
	source := sources["probe/access/ProtectedFieldOwner.java"]
	source = source[:strings.Index(source, "class ProtectedFieldDriver")] + `class ProtectedFieldDriver{public static void main(String[]args){probe.base.FieldEffects.trace="";ProtectedFieldOwner.Reader reader=new ProtectedFieldOwner.Reader();try{reader.first();throw new AssertionError("missing init failure");}catch(ExceptionInInitializerError e){if(e.getCause()!=probe.base.FieldEffects.failure||!probe.base.FieldEffects.trace.equals("P"))throw new AssertionError("original init cause/order");}probe.base.FieldEffects.trace="";try{reader.second();throw new AssertionError("missing subsequent failure");}catch(NoClassDefFoundError e){if(!probe.base.FieldEffects.trace.equals(""))throw new AssertionError("initialization retry");}System.out.println("protected:static:field:init:failure:identity");}}`
	sources["probe/access/ProtectedFieldOwner.java"] = source
	testNativePrivateSetterSourceFixture(t, sources, "probe/access/ProtectedFieldOwner", "probe.access.ProtectedFieldDriver", "protected:static:field:init:failure:identity\n")
}

func TestNativeProtectedStaticFieldEmptyInterfaceRoundTrip(t *testing.T) {
	sources := nativeProtectedStaticFieldSources("ProtectedFieldOwner")
	sources["probe/base/FieldMarkerBase.java"] = `package probe.base;public interface FieldMarkerBase{}`
	sources["probe/base/FieldMarker.java"] = `package probe.base;public interface FieldMarker extends FieldMarkerBase{}`
	sources["probe/base/FieldMiddle.java"] = strings.Replace(sources["probe/base/FieldMiddle.java"], "extends FieldParent", "extends FieldParent implements FieldMarker", 1)
	testNativePrivateSetterSourceFixture(t, sources, "probe/access/ProtectedFieldOwner", "probe.access.ProtectedFieldDriver", "10:protected:static:field:binding:initialization:identity\n")
}
