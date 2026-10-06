package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialCrossFamilyGenericEnclosingSuperclassRoundTrip(t *testing.T) {
	testCrossFamilyGenericEnclosing(t, false, "ordinary")
}
func TestAdversarialCrossFamilyGenericMemberEnclosingSuperclassRoundTrip(t *testing.T) {
	testCrossFamilyGenericEnclosing(t, true, "ordinary")
}
func TestAdversarialCrossFamilyGenericBoundedMemberEnclosingSuperclassRoundTrip(t *testing.T) {
	testCrossFamilyGenericEnclosing(t, true, "bounded")
}
func TestAdversarialCrossFamilyGenericArrayMemberEnclosingSuperclassRoundTrip(t *testing.T) {
	testCrossFamilyGenericEnclosing(t, true, "array")
}
func TestAdversarialCrossFamilyGenericOverloadMemberEnclosingSuperclassRoundTrip(t *testing.T) {
	testCrossFamilyGenericEnclosing(t, true, "overload")
}
func TestAdversarialCrossFamilyGenericMarkerOverloadMemberEnclosingSuperclassRoundTrip(t *testing.T) {
	testCrossFamilyGenericEnclosing(t, true, "marker overload")
}
func TestAdversarialCrossFamilyGenericThisOverloadMemberEnclosingSuperclassRoundTrip(t *testing.T) {
	testCrossFamilyGenericEnclosing(t, true, "this overload")
}
func TestAdversarialCrossFamilyGenericThisFalseOverloadMemberEnclosingSuperclassRoundTrip(t *testing.T) {
	testCrossFamilyGenericEnclosing(t, true, "this overload false")
}
func TestAdversarialCrossFamilyGenericThisRenamedOverloadMemberEnclosingSuperclassRoundTrip(t *testing.T) {
	f := crossFamilyGenericEnclosingFixture(true, "this overload")
	f = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(f, "Binding", "Separate"), "Member", "EnclosingPart"), "Leaf", "CurrentPart")
	testNativeIndependentFamilyFixture(t, f, []string{"SeparateBase", "SeparateCurrent"}, "SeparateDriver", "6:cross-family:outer:identity:callback\n", nativeLexicalExactSignatures)
}
func TestAdversarialCrossFamilyGenericIntWordOverloadMemberEnclosingSuperclassRoundTrip(t *testing.T) {
	testCrossFamilyGenericEnclosing(t, true, "int word overload")
}
func testCrossFamilyGenericEnclosing(t *testing.T, genericMember bool, variant string) {
	f := crossFamilyGenericEnclosingFixture(genericMember, variant)
	want := "6:cross-family:outer:identity:callback\n"
	if variant == "int word overload" {
		want = "24:cross-family:outer:identity:callback\n"
	}
	testNativeIndependentFamilyFixture(t, f, []string{"BindingBase", "BindingCurrent"}, "BindingDriver", want, nativeLexicalExactSignatures)
}

func crossFamilyGenericEnclosingFixture(genericMember bool, variant string) string {
	f := strings.Replace(crossFamilyEnclosingFixture, "class BindingBase{", "class BindingBase<B>{", 1)
	f = strings.Replace(f, "class BindingCurrent extends BindingBase{", "class BindingCurrent<C> extends BindingBase<C>{", 1)
	f = strings.Replace(f, "final Object observed;final int n;Member(int n)", "final B kept;final Object observed;final int n;Member(B seed,int n)", 1)
	f = strings.Replace(f, "observed=current();", "observed=current();kept=seed;", 1)
	f = strings.Replace(f, "Leaf(int n)throws java.io.IOException{super(n);}", "Leaf(C seed,int n)throws java.io.IOException{super(seed,n);}", 1)
	f = strings.Replace(f, "Leaf build(int n)throws java.io.IOException{return new Leaf(n);}", "Leaf build(C seed,int n)throws java.io.IOException{return new Leaf(seed,n);}", 1)
	f = strings.Replace(f, "int rows=0;", "int rows=0;Object seed=new Object();", 1)
	f = strings.ReplaceAll(f, "o.build(n)", "o.build(seed,n)")
	f = strings.ReplaceAll(f, "x.base()!=o", "x.kept!=seed||x.base()!=o")

	if genericMember {
		f = strings.Replace(f, "class Member{", "class Member<M>{", 1)
		f = strings.Replace(f, "final B kept;", "final B kept;final M marker;", 1)
		f = strings.Replace(f, "Member(B seed,int n)", "Member(B seed,M marker,int n)", 1)
		f = strings.Replace(f, "kept=seed;", "kept=seed;this.marker=marker;", 1)
		f = strings.Replace(f, "class Leaf extends Member{", "class Leaf extends Member<String>{", 1)
		f = strings.Replace(f, "super(seed,n);", "super(seed,\"marker\",n);", 1)
		f = strings.ReplaceAll(f, "x.kept!=seed", "!x.marker.equals(\"marker\")||x.kept!=seed")
	}

	switch variant {
	case "int word overload":
		f = strings.Replace(f, "Member(B seed,M marker,int n)", "Member(B seed,M marker,byte n)throws java.io.IOException{throw new AssertionError(\"wrong byte overload\");}Member(B seed,M marker,int n)", 1)
		f = strings.Replace(f, "super(seed,\"marker\",n);", "super(seed,\"marker\",(int)(byte)n);", 1)
		f = strings.Replace(f, "for(int n:new int[]{-1,0,17}){", "for(int originalWord:new int[]{Integer.MIN_VALUE,-65537,-129,-128,-1,0,1,127,128,65535,65536,Integer.MAX_VALUE}){int n=(originalWord<<24)>>24;", 1)
		f = strings.ReplaceAll(f, "o.build(seed,n)", "o.build(seed,originalWord)")
	case "this overload", "this overload false":
		f = strings.Replace(f, "class Leaf extends Member<String>", "class Leaf extends Member<Object>", 1)
		f = strings.Replace(f, "super(seed,\"marker\",n);", "super(seed,(Object)\"marker\",n);", 1)
		f = strings.Replace(f, "Member(B seed,M marker,int n)", "Member(B seed,M marker,int n)throws java.io.IOException{this(seed,marker,n,true);}Member(B seed,String marker,int n,boolean flag)throws java.io.IOException{throw new AssertionError(\"wrong this overload\");}Member(B seed,M marker,int n,boolean flag)", 1)
		f = strings.Replace(f, `BindingEffects.trace+="P";`, `BindingEffects.trace+=(flag?"T":"F");BindingEffects.trace+="P";`, 1)
		want := "TP"
		if variant == "this overload false" {
			f = strings.Replace(f, "this(seed,marker,n,true);", "this(seed,marker,n,false);", 1)
			want = "FP"
		}
		f = strings.ReplaceAll(f, `.trace.equals("P")`, `.trace.equals("`+want+`")`)
	case "bounded":
		f = strings.Replace(f, "class BindingBase<B>", "class BindingBase<B extends CharSequence>", 1)
		f = strings.Replace(f, "class BindingCurrent<C>", "class BindingCurrent<C extends CharSequence>", 1)
		f = strings.Replace(f, "Object seed=new Object();", "String seed=new String(\"seed\");", 1)
	case "array":
		f = strings.Replace(f, "class Leaf extends Member<String>", "class Leaf extends Member<Object[]>", 1)
		f = strings.Replace(f, "super(seed,\"marker\",n);", "super(seed,new Object[]{\"marker\"},n);", 1)
		f = strings.ReplaceAll(f, "!x.marker.equals(\"marker\")", "((Object[])x.marker).length!=1||!((Object[])x.marker)[0].equals(\"marker\")")

	case "marker overload":
		f = strings.Replace(f, "class Leaf extends Member<String>", "class Leaf extends Member<Object>", 1)
		f = strings.Replace(f, "super(seed,\"marker\",n);", "super(seed,(Object)\"marker\",n);", 1)
		f = strings.Replace(f, "Member(B seed,M marker,int n)", "Member(B seed,String marker,int n)throws java.io.IOException{throw new AssertionError(\"wrong marker overload\");}Member(B seed,M marker,int n)", 1)
	case "overload":
		f = strings.Replace(f, "Member(B seed,M marker,int n)", "Member(String seed,Object marker,int n)throws java.io.IOException{throw new AssertionError(\"wrong overload\");}Member(B seed,M marker,int n)", 1)
	}
	return f
}
