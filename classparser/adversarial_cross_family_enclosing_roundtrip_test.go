package javaclassparser

import (
	"strings"
	"testing"
)

// Both enclosing fields are distinct original storage. A valid source family
// must bind the parent to the exact outer argument, not merely its declared type.
const crossFamilyEnclosingFixture = `class BindingEffects{static Object published;static String trace="";static final java.io.IOException error=new java.io.IOException("original");}
class BindingBase{final int id;BindingBase(int id){this.id=id;}class Member{final Object observed;final int n;Member(int n)throws java.io.IOException{BindingEffects.trace+="P";observed=current();BindingEffects.published=this;if(n<0)throw BindingEffects.error;this.n=n;}Object base(){return BindingBase.this;}Object current(){return BindingBase.this;}int id(){return BindingBase.this.id;}}}
class BindingCurrent extends BindingBase{BindingCurrent(int id){super(id);}class Leaf extends Member{Leaf(int n)throws java.io.IOException{super(n);}Object current(){return BindingCurrent.this;}}Leaf build(int n)throws java.io.IOException{return new Leaf(n);}}
class BindingDriver{public static void main(String[]args)throws Exception{int rows=0;for(BindingCurrent o:new BindingCurrent[]{new BindingCurrent(7),new BindingCurrent(31)})for(int n:new int[]{-1,0,17}){BindingEffects.trace="";BindingEffects.published=null;try{BindingCurrent.Leaf x=o.build(n);if(n<0||x.base()!=o||x.current()!=o||x.observed!=o||x.id()!=o.id||x.n!=n||BindingEffects.published!=x||!BindingEffects.trace.equals("P"))throw new AssertionError("outer identity/callback/order");}catch(java.io.IOException e){BindingCurrent.Leaf x=(BindingCurrent.Leaf)BindingEffects.published;if(n>=0||e!=BindingEffects.error||x==null||x.base()!=o||x.current()!=o||x.observed!=o||x.n!=0||!BindingEffects.trace.equals("P"))throw new AssertionError("checked identity/publication");}rows++;}System.out.println(rows+":cross-family:outer:identity:callback");}}`

func TestAdversarialCrossFamilyEnclosingSuperclassRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, crossFamilyEnclosingFixture, []string{"BindingBase", "BindingCurrent"}, "BindingDriver", "6:cross-family:outer:identity:callback\n", nativeLexicalExactSignatures)
}

func TestAdversarialCrossFamilyEnclosingSuperclassRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(crossFamilyEnclosingFixture, "Binding", "Independent")
	f = strings.ReplaceAll(f, "Member", "AncestorPart")
	f = strings.ReplaceAll(f, "Leaf", "CurrentPart")
	testNativeIndependentFamilyFixture(t, f, []string{"IndependentBase", "IndependentCurrent"}, "IndependentDriver", "6:cross-family:outer:identity:callback\n", nativeLexicalExactSignatures)
}
func TestAdversarialCrossFamilyEnclosingSuperclassPackagedRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, "package binding.separate;\n"+crossFamilyEnclosingFixture, []string{"binding/separate/BindingBase", "binding/separate/BindingCurrent"}, "binding.separate.BindingDriver", "6:cross-family:outer:identity:callback\n", nativeLexicalExactSignatures)
}
func TestAdversarialCrossFamilyEnclosingSuperclassTwoAncestorsRoundTrip(t *testing.T) {
	f := strings.Replace(crossFamilyEnclosingFixture, "class BindingCurrent extends BindingBase", "class BindingMiddle extends BindingBase{BindingMiddle(int id){super(id);}}class BindingCurrent extends BindingMiddle", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"BindingBase", "BindingMiddle", "BindingCurrent"}, "BindingDriver", "6:cross-family:outer:identity:callback\n", nativeLexicalExactSignatures)
}
func TestAdversarialCrossFamilyEnclosingSuperclassThisDelegationRoundTrip(t *testing.T) {
	f := strings.Replace(crossFamilyEnclosingFixture, "Leaf(int n)throws java.io.IOException{super(n);}", "Leaf(int n)throws java.io.IOException{this(n,0L);}Leaf(int n,long ignored)throws java.io.IOException{super(n);}", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"BindingBase", "BindingCurrent"}, "BindingDriver", "6:cross-family:outer:identity:callback\n", nativeLexicalExactSignatures)
}

func TestAdversarialCrossFamilyEnclosingSuperclassOverloadBindingRoundTrip(t *testing.T) {
	f := strings.Replace(crossFamilyEnclosingFixture, "Member(int n)throws java.io.IOException{", "Member(String value,int n)throws java.io.IOException{throw new AssertionError(\"wrong narrowed overload\");}Member(Object value,int n)throws java.io.IOException{", 1)
	f = strings.Replace(f, "Leaf(int n)throws java.io.IOException{super(n);}", "Leaf(int n)throws java.io.IOException{super((Object)\"original\",n);}", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"BindingBase", "BindingCurrent"}, "BindingDriver", "6:cross-family:outer:identity:callback\n", nativeLexicalExactSignatures)
}
