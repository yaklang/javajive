package javaclassparser

import (
	"strings"
	"testing"
)

const nativeMemberEnclosingWideningFixture = `class WideningEffects{static String trace="";static final java.io.IOException failure=new java.io.IOException("original");}
class WideningOwner{class BaseScope{class Parent{final Object declared,observed;final long number;Parent(Object seed,long n)throws java.io.IOException{WideningEffects.trace+="P";declared=BaseScope.this;observed=scope();if(seed==null)throw WideningEffects.failure;number=n;}Object scope(){return BaseScope.this;}}}
class SubScope extends BaseScope{class Child extends Parent{Child(Object seed,long n)throws java.io.IOException{super(seed,n);}Object scope(){return SubScope.this;}}Child make(Object seed,long n)throws java.io.IOException{return new Child(seed,n);}}SubScope sub(){return new SubScope();}}
class WideningDriver{public static void main(String[]args)throws Exception{Object seed=new Object();int rows=0;for(WideningOwner owner:new WideningOwner[]{new WideningOwner(),new WideningOwner()}){WideningOwner.SubScope scope=owner.sub();for(long n:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE}){WideningEffects.trace="";WideningOwner.SubScope.Child child=scope.make(seed,n);if(child.declared!=scope||child.observed!=scope||child.scope()!=scope||child.number!=n||!WideningEffects.trace.equals("P"))throw new AssertionError("enclosing widening/callback/order/width");if(child.getClass().getDeclaringClass()!=WideningOwner.SubScope.class)throw new AssertionError("binary owner");rows++;}try{scope.make(null,0);throw new AssertionError("missing checked failure");}catch(java.io.IOException failure){if(failure!=WideningEffects.failure)throw new AssertionError("failure identity");}}System.out.println(rows+":enclosing:widening:identity:callback");}}`

func TestNativeMemberEnclosingWideningSuperclassRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeMemberEnclosingWideningFixture, "WideningOwner", "WideningDriver", "8:enclosing:widening:identity:callback\n")
}
func TestNativeMemberEnclosingWideningRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeMemberEnclosingWideningFixture, "WideningOwner", "WiderLexicalScope")
	f = strings.ReplaceAll(f, "BaseScope", "OriginScope")
	f = strings.ReplaceAll(f, "SubScope", "CurrentScope")
	testNativePrivateSetterFixture(t, f, "WiderLexicalScope", "WideningDriver", "8:enclosing:widening:identity:callback\n")
}

func TestNativeMemberEnclosingWideningTwoAncestorRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeMemberEnclosingWideningFixture, "class SubScope extends BaseScope", "class MiddleScope extends BaseScope{}class SubScope extends MiddleScope", 1)
	testNativePrivateSetterFixture(t, fixture, "WideningOwner", "WideningDriver", "8:enclosing:widening:identity:callback\n")
}
func TestNativeMemberEnclosingWideningGenericCaptureRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeMemberEnclosingWideningFixture, "class SubScope extends BaseScope", "class SubScope<Payload> extends BaseScope", 1)
	fixture = strings.Replace(fixture, "SubScope sub(){return new SubScope();}", "<Payload> SubScope<Payload> sub(){return new SubScope<Payload>();}", 1)
	fixture = strings.Replace(fixture, "WideningOwner.SubScope scope=owner.sub();", "WideningOwner.SubScope<Object> scope=owner.<Object>sub();", 1)
	fixture = strings.Replace(fixture, "WideningOwner.SubScope.Child child=", "WideningOwner.SubScope<Object>.Child child=", 1)
	testNativePrivateSetterCompiledFixture(t, "WideningOwner", "WideningDriver", "8:enclosing:widening:identity:callback\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileDebugClasses(t, fixture, debug)
	}, nativeLexicalExactSignatures)
}
