package javaclassparser

import (
	"strings"
	"testing"
)

func nativeProtectedGenericSources(bound, root, formal string) map[string]string {
	parent := strings.ReplaceAll(nativeProtectedParentFixture, "class DispatchParent", "class DispatchParent<T extends "+bound+">")
	parent = strings.Replace(parent, "protected Object prepare(Object token,long n)", "protected T prepare(T token,long n)", 1)
	source := strings.ReplaceAll(nativeProtectedCallFixture, "ProtectedCallOwner", root)
	source = strings.Replace(source, "public class "+root+" extends probe.base.DispatchParent", "public class "+root+"<"+formal+" extends "+bound+"> extends probe.base.DispatchParent<"+formal+">", 1)
	// Calls deliberately use raw receivers. The original JVM descriptor carries
	// the declaring class formal's bound, not a caller formal with the same name.
	if bound != "Object" {
		source = strings.ReplaceAll(source, "Object token", bound+" token")
		source = strings.ReplaceAll(source, "Object result", bound+" result")
		source = strings.ReplaceAll(source, "Object argument", bound+" argument")
		source = strings.ReplaceAll(source, "Object first", bound+" first")
		source = strings.ReplaceAll(source, "Object second", bound+" second")
		source = strings.ReplaceAll(source, "public Object prepare", "public "+bound+" prepare")
		source = strings.ReplaceAll(source, "new Object[]{null,new Object()}", "new "+bound+"[]{null,Long.valueOf(Long.MIN_VALUE),Double.valueOf(Double.longBitsToDouble(0x7ff8000000000001L))}")
	}
	return map[string]string{"probe/base/DispatchParent.java": parent, "probe/access/" + root + ".java": source}
}

func TestNativeProtectedGenericRawCallRoundTrip(t *testing.T) {
	testNativePrivateSetterSourceFixture(t, nativeProtectedGenericSources("Object", "GenericProtectedOwner", "T"), "probe/access/GenericProtectedOwner", "probe.access.ProtectedCallDriver", "4:protected:inherited:virtual:arguments:bridges\n")
}
func TestNativeProtectedGenericBoundedRawCallRoundTrip(t *testing.T) {
	testNativePrivateSetterSourceFixture(t, nativeProtectedGenericSources("Number", "BoundedProtectedOwner", "E"), "probe/access/BoundedProtectedOwner", "probe.access.ProtectedCallDriver", "6:protected:inherited:virtual:arguments:bridges\n")
}
func TestNativeProtectedGenericRenamedRawCallRoundTrip(t *testing.T) {
	testNativePrivateSetterSourceFixture(t, nativeProtectedGenericSources("Number", "DifferentGenericDispatchScope", "IndependentBinding"), "probe/access/DifferentGenericDispatchScope", "probe.access.ProtectedCallDriver", "6:protected:inherited:virtual:arguments:bridges\n")
}

func TestNativeProtectedGenericFixedAncestorRawCallRoundTrip(t *testing.T) {
	sources := nativeProtectedGenericSources("Object", "FixedAncestorRawOwner", "Unused")
	key := "probe/access/FixedAncestorRawOwner.java"
	sources[key] = strings.Replace(sources[key], "probe.base.DispatchParent<Unused>", "probe.base.DispatchParent<Integer>", 1)
	testNativePrivateSetterSourceFixture(t, sources, "probe/access/FixedAncestorRawOwner", "probe.access.ProtectedCallDriver", "4:protected:inherited:virtual:arguments:bridges\n")
}

func TestNativeProtectedGenericArrayRawCallRoundTrip(t *testing.T) {
	parent := `package probe.base;public class ArrayParent<T extends Number>{public static final java.io.IOException failure=new java.io.IOException("original");protected T[][] prepare(T[][] token,long n)throws java.io.IOException{if(n==Long.MIN_VALUE)throw failure;return token;}protected String[] prepare(String[] token,long n){throw new AssertionError("wrong overload");}}`
	source := `package probe.access;class ArrayEffects{static String trace="";static int fail;static ArrayProtectedOwner receiver(ArrayProtectedOwner owner)throws java.io.IOException{trace+="R";if(fail==1)throw probe.base.ArrayParent.failure;return owner;}static Number[][] argument(Number[][] token)throws java.io.IOException{trace+="A";if(fail==2)throw probe.base.ArrayParent.failure;return token;}static long wide(long n)throws java.io.IOException{trace+="W";if(fail==3)throw probe.base.ArrayParent.failure;return n;}}
public class ArrayProtectedOwner<E extends Number> extends probe.base.ArrayParent<E>{static class Reader{Number[][] first(ArrayProtectedOwner owner,Number[][] token,long n)throws java.io.IOException{return ArrayEffects.receiver(owner).prepare(ArrayEffects.argument(token),ArrayEffects.wide(n));}Number[][] second(ArrayProtectedOwner owner,Number[][] token,long n)throws java.io.IOException{return ArrayEffects.receiver(owner).prepare(ArrayEffects.argument(token),ArrayEffects.wide(n));}}}
class ArrayDerived extends ArrayProtectedOwner{public Number[][] prepare(Number[][] token,long n)throws java.io.IOException{ArrayEffects.trace+="D";return super.prepare(token,n);}}
class ArrayDriver{public static void main(String[]args)throws Exception{ArrayProtectedOwner.Reader reader=new ArrayProtectedOwner.Reader();ArrayProtectedOwner owner=new ArrayDerived();int rows=0;Number payload=Double.valueOf(Double.longBitsToDouble(0x7ff8000000000001L));for(int method:new int[]{0,1})for(Number[][] token:new Number[][][]{null,new Number[0][],new Number[][]{null,{payload}}}){ArrayEffects.fail=0;ArrayEffects.trace="";Number[][] result=method==0?reader.first(owner,token,7):reader.second(owner,token,7);if(result!=token||!ArrayEffects.trace.equals("RAWD"))throw new AssertionError("rank/identity/binding/once");if(token!=null&&token.length==2){if(result[1][0]!=payload)throw new AssertionError("nested identity");result[1][0]=Long.valueOf(Long.MIN_VALUE);if(token[1][0].longValue()!=Long.MIN_VALUE)throw new AssertionError("original alias mutation");}ArrayEffects.trace="";try{reader.first(owner,token,Long.MIN_VALUE);throw new AssertionError("missing target failure");}catch(java.io.IOException e){if(e!=probe.base.ArrayParent.failure||!ArrayEffects.trace.equals("RAWD"))throw new AssertionError("checked identity");}rows++;}for(int fail:new int[]{0,1,2,3}){ArrayEffects.fail=fail;ArrayEffects.trace="";try{reader.first(null,null,1);throw new AssertionError("missing failure");}catch(Exception e){if(fail==0?!(e instanceof NullPointerException):e!=probe.base.ArrayParent.failure)throw new AssertionError("failure identity");}if(!ArrayEffects.trace.equals(fail==1?"R":fail==2?"RA":"RAW"))throw new AssertionError("receiver/RHS order");}System.out.println(rows+":protected:generic:array:rank:identity");}}`
	testNativePrivateSetterSourceFixture(t, map[string]string{"probe/base/ArrayParent.java": parent, "probe/access/ArrayProtectedOwner.java": source}, "probe/access/ArrayProtectedOwner", "probe.access.ArrayDriver", "6:protected:generic:array:rank:identity\n")
}

func TestNativeProtectedGenericThrowsRawCallRoundTrip(t *testing.T) {
	parent := strings.Replace(nativeProtectedParentFixture, "class DispatchParent", "class DispatchParent<T extends Exception>", 1)
	parent = strings.Replace(parent, "throws java.io.IOException{if(n==Long.MIN_VALUE)throw failure;", "throws T{if(n==Long.MIN_VALUE)throw (T)failure;", 1)
	sources := nativeProtectedGenericSources("Object", "CheckedGenericOwner", "E")
	sources["probe/base/DispatchParent.java"] = parent
	key := "probe/access/CheckedGenericOwner.java"
	source := strings.Replace(sources[key], "<E extends Object>", "<E extends Exception>", 1)
	sources[key] = strings.ReplaceAll(source, "java.io.IOException", "Exception")
	testNativePrivateSetterSourceFixture(t, sources, "probe/access/CheckedGenericOwner", "probe.access.ProtectedCallDriver", "4:protected:inherited:virtual:arguments:bridges\n")
}
