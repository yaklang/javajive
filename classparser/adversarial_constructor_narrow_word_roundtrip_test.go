package javaclassparser

import (
	"strings"
	"testing"
)

const narrowDelegationFixture = `class NarrowTarget{final int value;NarrowTarget(TYPE value){this.value=value;}NarrowTarget(int word){throw new AssertionError("wrong int overload");}}
class NarrowOwner{class Part extends NarrowTarget{Part(int word){super((TYPE)word);}Object owner(){return NarrowOwner.this;}}Part build(int word){return new Part(word);}}
class NarrowDriver{public static void main(String[]args){NarrowOwner owner=new NarrowOwner();int rows=0;for(int word:new int[]{Integer.MIN_VALUE,-65537,-129,-128,-1,0,1,127,128,65535,65536,Integer.MAX_VALUE}){NarrowOwner.Part part=owner.build(word);int expected=EXPR;if(part.value!=expected||part.owner()!=owner)throw new AssertionError("narrow word/descriptor/outer identity");rows++;}System.out.println(rows+":narrow:constructor:identity");}}`

func TestAdversarialConstructorNarrowByteDelegationRoundTrip(t *testing.T) {
	testNarrowDelegation(t, "byte", "(word<<24)>>24", false)
}
func TestAdversarialConstructorNarrowShortDelegationRoundTrip(t *testing.T) {
	testNarrowDelegation(t, "short", "(word<<16)>>16", false)
}
func TestAdversarialConstructorNarrowCharDelegationRoundTrip(t *testing.T) {
	testNarrowDelegation(t, "char", "word&65535", false)
}
func TestAdversarialConstructorNarrowByteRenamedDelegationRoundTrip(t *testing.T) {
	testNarrowDelegation(t, "byte", "(word<<24)>>24", true)
}
func TestAdversarialConstructorNarrowByteIntTargetRoundTrip(t *testing.T) {
	testNarrowIntTarget(t, "byte", "(word<<24)>>24")
}
func TestAdversarialConstructorNarrowShortIntTargetRoundTrip(t *testing.T) {
	testNarrowIntTarget(t, "short", "(word<<16)>>16")
}
func TestAdversarialConstructorNarrowCharIntTargetRoundTrip(t *testing.T) {
	testNarrowIntTarget(t, "char", "word&65535")
}
func testNarrowIntTarget(t *testing.T, typ, oracle string) {
	f := strings.Replace(narrowDelegationFixture, `NarrowTarget(TYPE value){this.value=value;}NarrowTarget(int word){throw new AssertionError("wrong int overload");}`, `NarrowTarget(TYPE value){throw new AssertionError("wrong narrow overload");}NarrowTarget(int value){this.value=value;}`, 1)
	f = strings.Replace(f, "super((TYPE)word)", "super((int)(TYPE)word)", 1)
	f = strings.NewReplacer("TYPE", typ, "EXPR", oracle).Replace(f)
	testNativeIndependentFamilyFixture(t, f, []string{"NarrowOwner"}, "NarrowDriver", "12:narrow:constructor:identity\n", nativeLexicalExactSignatures)
}
func testNarrowDelegation(t *testing.T, typ, oracle string, rename bool) {
	f := strings.NewReplacer("TYPE", typ, "EXPR", oracle).Replace(narrowDelegationFixture)
	owner, driver := "NarrowOwner", "NarrowDriver"
	if rename {
		f = strings.ReplaceAll(f, "Narrow", "SeparateBits")
		owner, driver = "SeparateBitsOwner", "SeparateBitsDriver"
	}
	testNativeIndependentFamilyFixture(t, f, []string{owner}, driver, "12:narrow:constructor:identity\n", nativeLexicalExactSignatures)
}
