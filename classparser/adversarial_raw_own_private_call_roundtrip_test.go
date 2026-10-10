package javaclassparser

import (
	"strings"
	"testing"
)

const rawOwnPrivateCallFixture = `class RawOwnEffects{static String trace="";static final java.io.IOException error=new java.io.IOException("same");}
 class RawOwnCallOwner<T extends Number>{private final T token;RawOwnCallOwner(T token){this.token=token;}private T choose(Object value,long n)throws java.io.IOException{RawOwnEffects.trace+="P";if(n==Long.MAX_VALUE)throw RawOwnEffects.error;return token;}private String choose(String value,long n){throw new AssertionError("wrong String overload");}class Reader<T extends CharSequence>{Object get(Object value,long n)throws java.io.IOException{return choose(value,n);}}Reader<String>reader(){return new Reader<String>();}}
 class RawOwnDerived extends RawOwnCallOwner<Integer>{RawOwnDerived(Integer value){super(value);}public Number choose(Object value,long n){throw new AssertionError("private dispatch became virtual");}}
 class RawOwnDriver{public static void main(String[]args)throws Exception{Integer token=Integer.valueOf(7);RawOwnCallOwner<Integer>o=new RawOwnDerived(token);RawOwnCallOwner<Integer>.Reader<String>r=o.reader();int rows=0;for(Object v:new Object[]{null,"text",new Object()})for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){RawOwnEffects.trace="";try{if(r.get(v,n)!=token||n==Long.MAX_VALUE)throw new AssertionError("class-bound result identity");}catch(java.io.IOException e){if(n!=Long.MAX_VALUE||e!=RawOwnEffects.error)throw new AssertionError("checked failure identity");}if(!RawOwnEffects.trace.equals("P"))throw new AssertionError("effects");rows++;}if(r.getClass().getDeclaringClass()!=RawOwnCallOwner.class||RawOwnCallOwner.class.getDeclaredClasses().length!=1)throw new AssertionError("owners");System.out.println(rows+":raw:own:shadow:private:overload:identity:failure");}}`

func TestAdversarialRawOwnPrivateClassBoundCallRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, rawOwnPrivateCallFixture, []string{"RawOwnCallOwner"}, "RawOwnDriver", "9:raw:own:shadow:private:overload:identity:failure\n", nativeLexicalExactSignatures)
}
func TestAdversarialRawOwnPrivateClassBoundCallRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(rawOwnPrivateCallFixture, "RawOwnCallOwner", "RenamedBoundCallOwner")
	f = strings.ReplaceAll(f, "choose", "selectOriginal")
	testNativeIndependentFamilyFixture(t, f, []string{"RenamedBoundCallOwner"}, "RawOwnDriver", "9:raw:own:shadow:private:overload:identity:failure\n", nativeLexicalExactSignatures)
}

// The caller's source result needs the declaring class variable, even though
// the physical bridge returns its Number erasure. This guards both lookup and
// the source re-adaptation after the explicit raw receiver call.
func TestAdversarialRawOwnPrivateClassBoundResultRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(rawOwnPrivateCallFixture, "class Reader<T extends CharSequence>{Object get", "class Reader<U extends CharSequence>{T get")
	testNativeIndependentFamilyFixture(t, f, []string{"RawOwnCallOwner"}, "RawOwnDriver", "9:raw:own:shadow:private:overload:identity:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialRawOwnPrivateRecursiveArrayBoundRoundTrip(t *testing.T) {
	const f = `class RecursiveArrayEffects{static String trace="";}
 class RecursiveArrayOwner<T extends Comparable<T>>{private final T[] token;RecursiveArrayOwner(T[]x){token=x;}private T[] choose(T[]x,long n){RecursiveArrayEffects.trace+="P";if(x!=token)throw new AssertionError("argument identity");return x;}private Object[] choose(Object x,long n){throw new AssertionError("wrong Object overload");}class Reader<T extends Number>{Object[]get(Comparable[]x,long n){return choose((RecursiveArrayOwner.this.token),n);}}Reader<Integer>reader(){return new Reader<Integer>();}}
 class RecursiveArrayDriver{public static void main(String[]a){String[]token={"same"};RecursiveArrayOwner<String>o=new RecursiveArrayOwner<String>(token);RecursiveArrayOwner<String>.Reader<Integer>r=o.reader();for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){RecursiveArrayEffects.trace="";if(r.get(token,n)!=token||!RecursiveArrayEffects.trace.equals("P"))throw new AssertionError("array erasure or effects");}if(r.getClass().getDeclaringClass()!=RecursiveArrayOwner.class)throw new AssertionError("owner");System.out.println("3:recursive:array:bound:private:identity");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"RecursiveArrayOwner"}, "RecursiveArrayDriver", "3:recursive:array:bound:private:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialRawOwnPrivateMethodBoundRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(rawOwnPrivateCallFixture, "private T choose(Object", "private <U extends Comparable<U>> T choose(Object")
	testNativeIndependentFamilyFixture(t, f, []string{"RawOwnCallOwner"}, "RawOwnDriver", "9:raw:own:shadow:private:overload:identity:failure\n", nativeLexicalExactSignatures)
}
func TestAdversarialRawOwnPrivateMethodShadowsClassBoundRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(rawOwnPrivateCallFixture, "RawOwnCallOwner<T extends Number>", "RawOwnCallOwner<T extends CharSequence>")
	f = strings.ReplaceAll(f, "private final T token;RawOwnCallOwner(T token)", "private final Number token;RawOwnCallOwner(Number token)")
	f = strings.ReplaceAll(f, "private T choose(Object", "private <T extends Number> T choose(Object")
	f = strings.ReplaceAll(f, "return token;", "return (T)token;")
	f = strings.ReplaceAll(f, "RawOwnCallOwner<Integer>", "RawOwnCallOwner<String>")
	testNativeIndependentFamilyFixture(t, f, []string{"RawOwnCallOwner"}, "RawOwnDriver", "9:raw:own:shadow:private:overload:identity:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialRawOwnPrivateMethodCheckedIntersectionRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(rawOwnPrivateCallFixture, "private T choose(Object value,long n)throws java.io.IOException", "private <T extends Number & java.io.Serializable,E extends java.io.IOException> T choose(Object value,long n)throws E")
	f = strings.ReplaceAll(f, "throw RawOwnEffects.error;return token;", "throw (E)RawOwnEffects.error;return (T)token;")
	testNativeIndependentFamilyFixture(t, f, []string{"RawOwnCallOwner"}, "RawOwnDriver", "9:raw:own:shadow:private:overload:identity:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialRawOwnPrivateMethodKeepsUncheckedThrowRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(rawOwnPrivateCallFixture, "private T choose(Object value,long n)throws java.io.IOException", "private <T extends Number,E extends java.io.IOException> T choose(Object value,long n)throws E")
	f = strings.ReplaceAll(f, "throw RawOwnEffects.error;return token;", "throw (E)RawOwnEffects.error;return (T)token;")
	f = strings.ReplaceAll(f, "static String trace=\"\";", "static String trace=\"\";static final RuntimeException unchecked=new RuntimeException(\"same unchecked\");")
	f = strings.ReplaceAll(f, "if(n==Long.MAX_VALUE)throw", "if(n==Long.MIN_VALUE)throw RawOwnEffects.unchecked;if(n==Long.MAX_VALUE)throw")
	f = strings.ReplaceAll(f, "}catch(java.io.IOException e)", "}catch(RuntimeException e){if(n!=Long.MIN_VALUE||e!=RawOwnEffects.unchecked)throw new AssertionError(\"unchecked failure changed\");}catch(java.io.IOException e)")
	testNativeIndependentFamilyFixture(t, f, []string{"RawOwnCallOwner"}, "RawOwnDriver", "9:raw:own:shadow:private:overload:identity:failure\n", nativeLexicalExactSignatures)
}
