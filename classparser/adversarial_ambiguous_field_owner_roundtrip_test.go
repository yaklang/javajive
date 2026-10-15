package javaclassparser

import "testing"

func TestAdversarialAmbiguousInheritedFieldSelectionKeepsExactBytecodeOwner(t *testing.T) {
	const fixture = `interface BindingTag{int word=43;}interface BindingOtherTag{int word=71;}
class BindingBase{public int word;public int baseWord(){return word;}}
class BindingChild extends BindingBase implements BindingTag,BindingOtherTag{}
class BindingAccess{static int read(BindingChild c){return ((BindingBase)c).word;}static void write(BindingChild c,int n){((BindingBase)c).word=n;}static int assigned(BindingChild c,int n){return ((BindingBase)c).word=n;}}
class BindingDriver{public static void main(String[]args){int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){BindingChild c=new BindingChild();((BindingBase)c).word=17;if(BindingAccess.read(c)!=17)throw new AssertionError("ambiguous source lookup read");BindingAccess.write(c,n);if(c.baseWord()!=n||BindingTag.word!=43||BindingOtherTag.word!=71)throw new AssertionError("ambiguous source lookup store");if(BindingAccess.assigned(c,n+1)!=n+1||c.baseWord()!=n+1)throw new AssertionError("ambiguous source assignment result");rows++;}System.out.println(rows+":ambiguous:field-owner:result");}}
`
	fieldOwnerOriginalDriverRoundTrip(t, fixture, "5:ambiguous:field-owner:result\n")
}
