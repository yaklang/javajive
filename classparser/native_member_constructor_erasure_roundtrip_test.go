package javaclassparser

import (
	"strings"
	"testing"
)

const nativeMemberConstructorErasureFixture = `class ErasureEffects{static String trace="";static int mode;static final RuntimeException error=new IllegalArgumentException("original");static Object published;}
class ErasureParent{final Object observed,input;ErasureParent(Object input){ErasureEffects.trace+="P";this.observed=owner();this.input=input;ErasureEffects.published=this;if(ErasureEffects.mode==2)throw ErasureEffects.error;}Object owner(){return null;}}
class ErasureRoot{static class Scope<T,U extends java.util.Collection<? super T>>{final java.util.concurrent.Callable<U> supplier;Scope(java.util.concurrent.Callable<U>supplier){this.supplier=supplier;}class Child extends ErasureParent{final U value;Child(U value){super(value);ErasureEffects.trace+="C";this.value=value;}Object owner(){return Scope.this;}}Object make()throws Exception{java.util.Collection raw=null;raw=supplier.call();return new Child((U)raw);}}}
class ErasureDriver{public static void main(String[]args)throws Exception{java.util.ArrayList<Object> token=new java.util.ArrayList<Object>();token.add("identity");ErasureRoot.Scope<Object,java.util.ArrayList<Object>> root=new ErasureRoot.Scope<Object,java.util.ArrayList<Object>>(new java.util.concurrent.Callable<java.util.ArrayList<Object>>(){public java.util.ArrayList<Object> call(){ErasureEffects.trace+="A";if(ErasureEffects.mode==1)throw ErasureEffects.error;return token;}});for(int mode:new int[]{0,1,2}){ErasureEffects.trace="";ErasureEffects.published=null;ErasureEffects.mode=mode;try{ErasureRoot.Scope<Object,java.util.ArrayList<Object>>.Child child=(ErasureRoot.Scope<Object,java.util.ArrayList<Object>>.Child)root.make();if(mode!=0||child.value!=token||child.input!=token||child.observed!=root||child.owner()!=root||!ErasureEffects.trace.equals("APC"))throw new AssertionError("enclosing generic binding");}catch(RuntimeException e){if(e!=ErasureEffects.error||mode==0||!ErasureEffects.trace.equals(mode==1?"A":"AP"))throw new AssertionError("throwable/order",e);if(mode==2){ErasureRoot.Scope<Object,java.util.ArrayList<Object>>.Child child=(ErasureRoot.Scope<Object,java.util.ArrayList<Object>>.Child)ErasureEffects.published;if(child==null||child.value!=null||child.observed!=root||child.owner()!=root)throw new AssertionError("capture/default");}}}System.out.println("3:enclosing:generic:erasure");}}
`

func TestNativeMemberConstructorEnclosingErasureRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeMemberConstructorErasureFixture, []string{"ErasureRoot"}, "ErasureDriver", "3:enclosing:generic:erasure\n")
}
func TestNativeMemberConstructorEnclosingErasureRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeMemberConstructorErasureFixture, "ErasureRoot", "OtherErasureScope")
	testNativeIndependentFamilyFixture(t, f, []string{"OtherErasureScope"}, "ErasureDriver", "3:enclosing:generic:erasure\n")
}
