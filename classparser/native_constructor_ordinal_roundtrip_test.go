package javaclassparser

import (
	"strings"
	"testing"
)

const nativeConstructorOrdinalFixture = `class CtorOrdinalOwner{private final int value;CtorOrdinalOwner(){value=17;}static Reader make(){return new Reader();}static class Reader{private Reader(){}int read(CtorOrdinalOwner owner){return owner.value;}}}
class CtorOrdinalDriver{public static void main(String[]args){CtorOrdinalOwner owner=new CtorOrdinalOwner();CtorOrdinalOwner.Reader first=CtorOrdinalOwner.make(),second=CtorOrdinalOwner.make();if(first==second||first.read(owner)!=17||second.read(owner)!=17)throw new AssertionError("constructor/field binding");try{first.read(null);throw new AssertionError("missing read failure");}catch(NullPointerException expected){}System.out.println("original:private:constructor:accessor:ordinal");}}`

func TestNativeConstructorOrdinalRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeConstructorOrdinalFixture, "CtorOrdinalOwner", "CtorOrdinalDriver", "original:private:constructor:accessor:ordinal\n")
}
func TestNativeConstructorOrdinalRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeConstructorOrdinalFixture, "CtorOrdinalOwner", "OtherConstructorAccessScope")
	testNativePrivateSetterFixture(t, f, "OtherConstructorAccessScope", "CtorOrdinalDriver", "original:private:constructor:accessor:ordinal\n")
}

const nativeConstructorOrdinalArgumentsFixture = `class CtorOrdinalEffects{static String trace="";static boolean fail;static final java.io.IOException failure=new java.io.IOException("original");static long argument(long n)throws java.io.IOException{trace+="A";if(fail)throw failure;return n;}}
class CtorOrdinalArgsOwner{private final int value;CtorOrdinalArgsOwner(){value=17;}static Reader make(long n)throws java.io.IOException{return new Reader(CtorOrdinalEffects.argument(n));}static class Reader{private final long recorded;private Reader(long n)throws java.io.IOException{CtorOrdinalEffects.trace+="C";recorded=n;}int read(CtorOrdinalArgsOwner owner){return owner.value;}long number(){return recorded;}}}
class CtorOrdinalArgsDriver{public static void main(String[]args)throws Exception{CtorOrdinalArgsOwner owner=new CtorOrdinalArgsOwner();int rows=0;for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){CtorOrdinalEffects.trace="";CtorOrdinalArgsOwner.Reader reader=CtorOrdinalArgsOwner.make(n);if(reader.read(owner)!=17||reader.number()!=n||!CtorOrdinalEffects.trace.equals("AC"))throw new AssertionError("private constructor binding/order/once");rows++;}CtorOrdinalEffects.fail=true;CtorOrdinalEffects.trace="";try{CtorOrdinalArgsOwner.make(1);throw new AssertionError("missing checked failure");}catch(java.io.IOException e){if(e!=CtorOrdinalEffects.failure||!CtorOrdinalEffects.trace.equals("A"))throw new AssertionError("producer failure before constructor");}System.out.println(rows+":original:private:constructor:arguments:ordinal");}}`

func TestNativeConstructorOrdinalArgumentsRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeConstructorOrdinalArgumentsFixture, "CtorOrdinalArgsOwner", "CtorOrdinalArgsDriver", "5:original:private:constructor:arguments:ordinal\n")
}
