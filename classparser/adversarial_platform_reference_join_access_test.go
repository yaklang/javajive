package javaclassparser

import "testing"

func TestAdversarialPlatformReferenceJoinPreservesAccessibleDeclaration(t *testing.T) {
	const f = `class AccessibleOwner{static CharSequence cached;static CharSequence select(boolean branch){CharSequence value;if(branch){value=new StringBuilder("first");}else{value=new StringBuffer("second");}cached=value;return value;}}class AccessibleDriver{public static void main(String[]args){for(boolean branch:new boolean[]{false,true}){CharSequence value=AccessibleOwner.select(branch);if(value!=AccessibleOwner.cached||!value.toString().equals(branch?"first":"second")||value.getClass()!=(branch?StringBuilder.class:StringBuffer.class)||value.length()!=(branch?5:6))throw new AssertionError("accessible join identity/value/runtime class");}System.out.println("2:platform-join:accessible:identity:runtime-class");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"AccessibleOwner"}, "AccessibleDriver", "2:platform-join:accessible:identity:runtime-class\n", nativeLexicalExactSignatures)
}
