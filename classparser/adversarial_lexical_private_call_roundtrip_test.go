package javaclassparser

import (
	"strings"
	"testing"
)

// The method's U is declared by the outer class, whose first bound names T.
// The reader shadows T. Its synthetic bridge must use the declaration's bound
// environment, preserve private dispatch and retain the exact exception/token.
const lexicalPrivateCallFixture = `class LexicalCallEffects{static String trace="";static final java.io.IOException error=new java.io.IOException("same checked");}
class LexicalCallOwner<T extends Number,U extends T>{final U token;LexicalCallOwner(U token){this.token=token;}class Value{private U choose(java.util.List<? super U> values,long n)throws java.io.IOException{LexicalCallEffects.trace+="P";if(values!=null)values.add(token);if(n==Long.MAX_VALUE)throw LexicalCallEffects.error;return token;}private String choose(java.util.ArrayList<String>values,long n){throw new AssertionError("wrong narrower overload");}class Reader<T extends CharSequence>{Object get(java.util.List<Object>values,long n)throws java.io.IOException{return choose((java.util.List)values,n);}}Reader<String> reader(){return new Reader<String>();}}Value value(){return new Value();}}
class LexicalCallDriver{public static void main(String[]args)throws Exception{Integer token=Integer.valueOf(17);LexicalCallOwner<Integer,Integer>o=new LexicalCallOwner<Integer,Integer>(token);LexicalCallOwner<Integer,Integer>.Value v=o.value();LexicalCallOwner<Integer,Integer>.Value.Reader<String>r=v.reader();int rows=0;for(boolean present:new boolean[]{false,true})for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){java.util.List<Object>list=present?new java.util.ArrayList<Object>():null;LexicalCallEffects.trace="";try{if(r.get(list,n)!=token||n==Long.MAX_VALUE)throw new AssertionError("result identity");}catch(java.io.IOException e){if(n!=Long.MAX_VALUE||e!=LexicalCallEffects.error)throw new AssertionError("checked identity");}if(!LexicalCallEffects.trace.equals("P")||present&&(list.size()!=1||list.get(0)!=token))throw new AssertionError("ordered partial effects");rows++;}if(r.getClass().getDeclaringClass()!=v.getClass()||v.getClass().getDeclaringClass()!=LexicalCallOwner.class)throw new AssertionError("lexical declarations");System.out.println(rows+":lexical:private:bound:shadow:identity:effects:failure");}}`

func TestAdversarialLexicalPrivateBoundCallRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, lexicalPrivateCallFixture, []string{"LexicalCallOwner"}, "LexicalCallDriver", "6:lexical:private:bound:shadow:identity:effects:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialLexicalPrivateBoundCallRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(lexicalPrivateCallFixture, "LexicalCallOwner", "RenamedLexicalMethodScope")
	f = strings.ReplaceAll(f, "choose", "originalTarget")
	testNativeIndependentFamilyFixture(t, f, []string{"RenamedLexicalMethodScope"}, "LexicalCallDriver", "6:lexical:private:bound:shadow:identity:effects:failure\n", nativeLexicalExactSignatures)
}

func lexicalPrivateDependentShadowFixture() string {
	f := strings.Replace(lexicalPrivateCallFixture, "class Value{", "class Mid<T extends CharSequence>{class Value{", 1)
	f = strings.Replace(f, "Value value(){return new Value();}}", "}Mid<String>.Value value(){return new Mid<String>().new Value();}}", 1)
	f = strings.ReplaceAll(f, "LexicalCallOwner<Integer,Integer>.Value", "LexicalCallOwner<Integer,Integer>.Mid<String>.Value")
	f = strings.Replace(f, "v.getClass().getDeclaringClass()!=LexicalCallOwner.class", "v.getClass().getDeclaringClass().getDeclaringClass()!=LexicalCallOwner.class", 1)
	return f
}

func TestAdversarialLexicalPrivateDependentBoundShadowRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, lexicalPrivateDependentShadowFixture(), []string{"LexicalCallOwner"}, "LexicalCallDriver", "6:lexical:private:bound:shadow:identity:effects:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialLexicalPrivateArrayBoundCallRoundTrip(t *testing.T) {
	f := strings.Replace(lexicalPrivateCallFixture, "final U token;LexicalCallOwner(U token)", "final U[] token;LexicalCallOwner(U[] token)", 1)
	f = strings.Replace(f, "private U choose", "private U[] choose", 1)
	f = strings.ReplaceAll(f, "List<? super U>", "List<? super U[]>")
	f = strings.Replace(f, "Integer token=Integer.valueOf(17)", "Integer[] token={Integer.valueOf(17),null}", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"LexicalCallOwner"}, "LexicalCallDriver", "6:lexical:private:bound:shadow:identity:effects:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialLexicalPrivateInterfaceFirstBoundCallRoundTrip(t *testing.T) {
	f := strings.Replace(lexicalPrivateCallFixture, "T extends Number,U extends T", "T extends CharSequence & java.io.Serializable,U extends T", 1)
	f = strings.Replace(f, "Integer token=Integer.valueOf(17)", "String token=new String(\"same token\")", 1)
	f = strings.ReplaceAll(f, "LexicalCallOwner<Integer,Integer>", "LexicalCallOwner<String,String>")
	testNativeIndependentFamilyFixture(t, f, []string{"LexicalCallOwner"}, "LexicalCallDriver", "6:lexical:private:bound:shadow:identity:effects:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialLexicalPrivateCallKeepsNonvirtualTargetRoundTrip(t *testing.T) {
	f := strings.Replace(lexicalPrivateCallFixture, "Value value(){return new Value();}", "class Derived extends Value{public Number choose(java.util.List values,long n){throw new AssertionError(\"private call became virtual\");}}Value value(){return new Derived();}", 1)
	f = strings.Replace(f, "r.getClass().getDeclaringClass()!=v.getClass()", "r.getClass().getDeclaringClass()!=LexicalCallOwner.Value.class", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"LexicalCallOwner"}, "LexicalCallDriver", "6:lexical:private:bound:shadow:identity:effects:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialLexicalPrivateStaticBoundaryCallRoundTrip(t *testing.T) {
	f := strings.Replace(lexicalPrivateCallFixture, "class LexicalCallOwner<", "class LexicalScopeTop<T extends CharSequence>{static class LexicalCallOwner<", 1)
	f = strings.Replace(f, "class LexicalCallDriver{", "}class LexicalCallDriver{", 1)
	prefix, driver, ok := strings.Cut(f, "class LexicalCallDriver{")
	if !ok {
		t.Fatal("fixture driver")
	}
	f = prefix + "class LexicalCallDriver{" + strings.ReplaceAll(driver, "LexicalCallOwner", "LexicalScopeTop.LexicalCallOwner")
	testNativeIndependentFamilyFixture(t, f, []string{"LexicalScopeTop"}, "LexicalCallDriver", "6:lexical:private:bound:shadow:identity:effects:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialLexicalPrivateMethodShadowsOuterBoundRoundTrip(t *testing.T) {
	f := strings.Replace(lexicalPrivateCallFixture, "private U choose", "private <U extends Number> U choose", 1)
	f = strings.Replace(f, "return token;", "return (U)token;", 1)
	f = strings.Replace(f, "values.add(token)", "values.add((U)token)", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"LexicalCallOwner"}, "LexicalCallDriver", "6:lexical:private:bound:shadow:identity:effects:failure\n", nativeLexicalExactSignatures)
}
