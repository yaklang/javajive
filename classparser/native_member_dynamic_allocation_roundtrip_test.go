package javaclassparser

import (
	"strings"
	"testing"
)

const nativeDynamicAllocationFixture = `class DynamicAllocationTrace{static String events="";}class DynamicAllocationOwner{static class Token{final int value;Token(int value){DynamicAllocationTrace.events+="token;";this.value=value;}}class Node{final Token token;final java.util.function.IntUnaryOperator step;Node(Token token,java.util.function.IntUnaryOperator step){DynamicAllocationTrace.events+="node;";this.token=token;this.step=step;}int run(int x){return step.applyAsInt(x)+token.value;}}Node make(int value){return new Node(new Token(value),x->{DynamicAllocationTrace.events+="apply;";return x*3;});}}class DynamicAllocationDriver{public static void main(String[]args){for(int value:new int[]{0,1,-1,Integer.MAX_VALUE}){DynamicAllocationTrace.events="";DynamicAllocationOwner.Node node=new DynamicAllocationOwner().make(value);if(node.getClass().getDeclaringClass()!=DynamicAllocationOwner.class||node.getClass().getEnclosingClass()!=DynamicAllocationOwner.class||node.token.getClass().getDeclaringClass()!=DynamicAllocationOwner.class||DynamicAllocationOwner.class.getDeclaredClasses().length!=2)throw new AssertionError("original declaration owners");if(node.run(7)!=21+value||!DynamicAllocationTrace.events.equals("token;node;apply;"))throw new AssertionError("nested allocation/dynamic producer order:"+DynamicAllocationTrace.events);}System.out.println("4:dynamic:nested:member:effects");}}`

// A nested allocation and opaque indy argument share one typed constructor
// snapshot. Original declaration identity/effects are measured independently;
// falling back to flat types does not meet this contract.
func TestNativeMemberNestedAllocationWithDynamicArgumentRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeDynamicAllocationFixture, []string{"DynamicAllocationOwner"}, "DynamicAllocationDriver", "4:dynamic:nested:member:effects\n", nativeLexicalExactSignatures)
}
func TestNativeMemberNestedAllocationWithDynamicArgumentRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeDynamicAllocationFixture, "DynamicAllocationOwner", "EvaluationScope")
	f = strings.ReplaceAll(f, "Token", "Operand")
	f = strings.ReplaceAll(f, "Node", "Result")
	testNativeIndependentFamilyFixture(t, f, []string{"EvaluationScope"}, "DynamicAllocationDriver", "4:dynamic:nested:member:effects\n", nativeLexicalExactSignatures)
}

func TestNativeMemberNestedAllocationWithCapturedDynamicArgumentRoundTrip(t *testing.T) {
	f := strings.Replace(nativeDynamicAllocationFixture, "return x*3;", "return x*3+value;", 1)
	f = strings.Replace(f, "node.run(7)!=21+value", "node.run(7)!=21+value+value", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"DynamicAllocationOwner"}, "DynamicAllocationDriver", "4:dynamic:nested:member:effects\n", nativeLexicalExactSignatures)
}
func TestNativeMemberNestedAllocationWithWideAndDynamicArgumentsRoundTrip(t *testing.T) {
	f := strings.Replace(nativeDynamicAllocationFixture, "final Token token;", "final Token token;final long wide;final double fraction;", 1)
	f = strings.Replace(f, "Node(Token token,java.util.function.IntUnaryOperator step)", "Node(Token token,long wide,double fraction,java.util.function.IntUnaryOperator step)", 1)
	f = strings.Replace(f, "this.token=token;this.step=step;", "this.token=token;this.wide=wide;this.fraction=fraction;this.step=step;", 1)
	f = strings.Replace(f, "step.applyAsInt(x)+token.value", "step.applyAsInt(x)+token.value+(int)wide+(int)fraction", 1)
	f = strings.Replace(f, "new Node(new Token(value),x->", "new Node(new Token(value),4L,3.0D,x->", 1)
	f = strings.Replace(f, "node.run(7)!=21+value", "node.run(7)!=28+value", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"DynamicAllocationOwner"}, "DynamicAllocationDriver", "4:dynamic:nested:member:effects\n", nativeLexicalExactSignatures)
}
func TestNativeMemberBranchNestedAllocationWithDynamicArgumentRoundTrip(t *testing.T) {
	f := strings.Replace(nativeDynamicAllocationFixture, "Node make(int value)", "Node make(int value,boolean flip)", 1)
	f = strings.Replace(f, "new Node(new Token(value),x->", "new Node(flip?new Token(-value):new Token(value),x->", 1)
	f = strings.Replace(f, "{DynamicAllocationTrace.events=\"\";", "for(boolean flip:new boolean[]{false,true}){DynamicAllocationTrace.events=\"\";", 1)
	f = strings.Replace(f, ".make(value)", ".make(value,flip)", 1)
	f = strings.Replace(f, "node.run(7)!=21+value", "node.run(7)!=21+(flip?-value:value)", 1)
	f = strings.Replace(f, "4:dynamic:nested:member:effects", "8:dynamic:nested:member:effects", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"DynamicAllocationOwner"}, "DynamicAllocationDriver", "8:dynamic:nested:member:effects\n", nativeLexicalExactSignatures)
}
