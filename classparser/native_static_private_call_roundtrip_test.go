package javaclassparser

import (
	"strings"
	"testing"
)

const nativeStaticPrivateCallFixture = `class StaticCallEffects{static String trace="";static int fail;static final java.io.IOException failure=new java.io.IOException("original");static Object argument(Object token)throws java.io.IOException{trace+="A";if(fail==1)throw failure;return token;}static long wide(long n)throws java.io.IOException{trace+="W";if(fail==2)throw failure;return n;}}
class StaticCallOwner{private static Object prepare(Object token,long n)throws java.io.IOException{StaticCallEffects.trace+="P";if(StaticCallEffects.fail==3)throw StaticCallEffects.failure;if(n!=Long.MIN_VALUE)throw new AssertionError("wide");return token;}private static Object prepare(String token,long n){throw new AssertionError("overload");}static class Reader{Object get(Object token,long n)throws java.io.IOException{return prepare(StaticCallEffects.argument(token),StaticCallEffects.wide(n));}}}
class StaticCallDriver{public static void main(String[]args)throws Exception{StaticCallOwner.Reader reader=new StaticCallOwner.Reader();int rows=0;for(Object token:new Object[]{null,new Object()}){StaticCallEffects.fail=0;StaticCallEffects.trace="";if(reader.get(token,Long.MIN_VALUE)!=token||!StaticCallEffects.trace.equals("AWP"))throw new AssertionError("binding/result/once");for(int fail:new int[]{1,2,3}){StaticCallEffects.fail=fail;StaticCallEffects.trace="";try{reader.get(token,Long.MIN_VALUE);throw new AssertionError("missing checked failure");}catch(java.io.IOException e){if(e!=StaticCallEffects.failure)throw new AssertionError("checked identity");}if(!StaticCallEffects.trace.equals(fail==1?"A":fail==2?"AW":"AWP"))throw new AssertionError("argument order");}rows++;}System.out.println(rows+":static:private:call:binding:order");}}`

func TestNativeStaticPrivateCallRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeStaticPrivateCallFixture, "StaticCallOwner", "StaticCallDriver", "2:static:private:call:binding:order\n")
}
func TestNativeStaticPrivateCallRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeStaticPrivateCallFixture, "StaticCallOwner", "OtherStaticFactoryScope")
	testNativePrivateSetterFixture(t, f, "OtherStaticFactoryScope", "StaticCallDriver", "2:static:private:call:binding:order\n")
}

const nativeStaticPrivateFactoryFixture = `abstract class StaticFactoryParent{StaticFactoryParent(){}}
class StaticFactoryEffects{static String trace="";static final UnsupportedOperationException failure=new UnsupportedOperationException("original");}
class StaticFactoryOwner{private static UnsupportedOperationException failure(){StaticFactoryEffects.trace+="F";return StaticFactoryEffects.failure;}static class Child extends StaticFactoryParent{Child(){super();}void fail(){throw failure();}}}
class StaticFactoryDriver{public static void main(String[]args){StaticFactoryOwner.Child child=new StaticFactoryOwner.Child();StaticFactoryEffects.trace="";try{child.fail();throw new AssertionError("missing failure");}catch(UnsupportedOperationException e){if(e!=StaticFactoryEffects.failure||!StaticFactoryEffects.trace.equals("F"))throw new AssertionError("factory identity/once");}System.out.println("static:private:factory:zero:args");}}`

func TestNativeStaticPrivateFactoryRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeStaticPrivateFactoryFixture, "StaticFactoryOwner", "StaticFactoryDriver", "static:private:factory:zero:args\n")
}

const nativeStaticPrivateInitializationFixture = `class StaticInitEffects{static String trace="";static final RuntimeException failure=new RuntimeException("original");static int initialize(){trace+="I";throw failure;}static Object argument(Object token){trace+="A";return token;}}
class StaticInitOwner{private static int state=StaticInitEffects.initialize();private static Object prepare(Object token){StaticInitEffects.trace+="P";return token;}static class Reader{Object get(Object token){return prepare(StaticInitEffects.argument(token));}}}
class StaticInitDriver{public static void main(String[]args){StaticInitOwner.Reader reader=new StaticInitOwner.Reader();Object token=new Object();StaticInitEffects.trace="";try{reader.get(token);throw new AssertionError("missing initial failure");}catch(ExceptionInInitializerError e){if(e.getCause()!=StaticInitEffects.failure||!StaticInitEffects.trace.equals("AI"))throw new AssertionError("original class init cause/order");}StaticInitEffects.trace="";try{reader.get(token);throw new AssertionError("missing subsequent failure");}catch(NoClassDefFoundError e){if(!StaticInitEffects.trace.equals("A"))throw new AssertionError("no initialization retry/argument once");}System.out.println("static:private:call:initialization:order:identity");}}`

func TestNativeStaticPrivateInitializationRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeStaticPrivateInitializationFixture, "StaticInitOwner", "StaticInitDriver", "static:private:call:initialization:order:identity\n")
}
