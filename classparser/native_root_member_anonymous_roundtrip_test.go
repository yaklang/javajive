package javaclassparser

import (
	"strings"
	"testing"
)

const nativeRootMemberAnonymousFixture = `class RootAnonMemberEffects{static String trace="";static final java.io.IOException error=new java.io.IOException("original");}
class RootAnonMemberOwner {final Object token=new Object();class Parent {final Object observed;final long number;Parent(long n,Object seed)throws java.io.IOException{RootAnonMemberEffects.trace+="P";observed=owner();if(observed!=RootAnonMemberOwner.this)throw new AssertionError("captured enclosing receiver before SUPER callback");if(seed==null)throw RootAnonMemberEffects.error;number=n;}Object owner(){return RootAnonMemberOwner.this;}Object capture(){return null;}}
Parent make(Object seed,Object kept,long number)throws java.io.IOException{return new Parent(number,seed){Object owner(){return RootAnonMemberOwner.this;}Object capture(){return kept;}};}}
class RootAnonMemberDriver {public static void main(String[]args)throws Exception{RootAnonMemberOwner a=new RootAnonMemberOwner();RootAnonMemberOwner b=new RootAnonMemberOwner();Object token=new Object();int rows=0;for(RootAnonMemberOwner o:new RootAnonMemberOwner[]{a,b})for(Object value:new Object[]{null,token})for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){RootAnonMemberEffects.trace="";RootAnonMemberOwner.Parent p=o.make(token,value,n);if(p.owner()!=o||p.observed!=o||p.capture()!=value||p.number!=n||!RootAnonMemberEffects.trace.equals("P")||p.getClass().getEnclosingClass()!=RootAnonMemberOwner.class||!p.getClass().getName().equals("RootAnonMemberOwner$1"))throw new AssertionError("enclosing identity/capture/value/order/binary owner");rows++;}try{a.make(null,token,0);throw new AssertionError("missing checked failure");}catch(java.io.IOException e){if(e!=RootAnonMemberEffects.error)throw new AssertionError("checked failure identity");}System.out.println(rows+":root:member:anonymous:identity:callback:checked");}}
`

func TestNativeRootMemberAnonymousSuperclassRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeRootMemberAnonymousFixture, "RootAnonMemberOwner", "RootAnonMemberDriver", "12:root:member:anonymous:identity:callback:checked\n")
}

func TestNativeRootMemberAnonymousSourceSpellingRoundTrip(t *testing.T) {
	fixture := strings.ReplaceAll(nativeRootMemberAnonymousFixture, "RootAnonMemberOwner", "IndependentParentScope")
	fixture = strings.ReplaceAll(fixture, "kept", "retainedReference")
	testNativePrivateSetterFixture(t, fixture, "IndependentParentScope", "RootAnonMemberDriver", "12:root:member:anonymous:identity:callback:checked\n")
}

const nativeRootMemberGenericAnonymousFixture = `class RootMemberGenericEffects{static String trace="";static final java.io.IOException error=new java.io.IOException("original");}
class RootMemberGenericOwner<T>{class Parent{final Object observed;Parent(Object seed)throws java.io.IOException{RootMemberGenericEffects.trace+="P";observed=owner();if(seed==null)throw RootMemberGenericEffects.error;}Object owner(){return RootMemberGenericOwner.this;}T[] values(){return null;}}
Parent make(Object seed,T[] kept)throws java.io.IOException{return new Parent(seed){Object owner(){return RootMemberGenericOwner.this;}T[] values(){return kept;}};}}
class RootMemberGenericDriver{public static void main(String[]args)throws Exception{RootMemberGenericOwner<Object> a=new RootMemberGenericOwner<>();RootMemberGenericOwner<Object>b=new RootMemberGenericOwner<>();Object seed=new Object();int rows=0;for(RootMemberGenericOwner<Object>o:java.util.Arrays.asList(a,b))for(Object[]v:new Object[][]{null,new Object[]{seed},new String[]{"original"}}){RootMemberGenericEffects.trace="";RootMemberGenericOwner<Object>.Parent p=o.make(seed,v);if(p.owner()!=o||p.observed!=o||p.values()!=v||!RootMemberGenericEffects.trace.equals("P")||!p.getClass().getName().equals("RootMemberGenericOwner$1")||p.getClass().getEnclosingClass()!=RootMemberGenericOwner.class)throw new AssertionError("generic owner/array/callback/order");rows++;}try{a.make(null,new Object[]{seed});throw new AssertionError("missing checked failure");}catch(java.io.IOException e){if(e!=RootMemberGenericEffects.error)throw new AssertionError("checked identity");}System.out.println(rows+":generic:root:member:anonymous:identity");}}
`

func TestNativeRootMemberGenericAnonymousSuperclassRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeRootMemberGenericAnonymousFixture, "RootMemberGenericOwner", "RootMemberGenericDriver", "6:generic:root:member:anonymous:identity\n")
}
