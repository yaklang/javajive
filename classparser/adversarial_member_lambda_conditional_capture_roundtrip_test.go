package javaclassparser

import (
	"strings"
	"testing"
)

const memberLambdaConditionalCaptureFixture = `
class JoinedCaptureEffects {static String trace="";static final RuntimeException failure=new RuntimeException("original");
 static int left(int seed){trace+="L";if(seed==17)throw failure;return seed^0x76543210;}
 static int right(int seed){trace+="R";if(seed==17)throw failure;return seed^0x13579bdf;}}
class JoinedCaptureOwner {
 static class View {
  java.util.function.IntUnaryOperator operation(int seed,boolean choose){final int value=choose?JoinedCaptureEffects.left(seed):JoinedCaptureEffects.right(seed);return argument->{JoinedCaptureEffects.trace+="B";return argument^value;};}
 }
 static View view(){return new View();}
}
class JoinedCaptureDriver {
 public static void main(String[]args){int rows=0;JoinedCaptureOwner.View owner=JoinedCaptureOwner.view();
  if(!owner.getClass().isMemberClass()||owner.getClass().getDeclaringClass()!=JoinedCaptureOwner.class)throw new AssertionError("member identity");
  for(int seed:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(boolean choose:new boolean[]{false,true}){
   JoinedCaptureEffects.trace="";java.util.function.IntUnaryOperator operation=owner.operation(seed,choose);
   if(!JoinedCaptureEffects.trace.equals(choose?"L":"R"))throw new AssertionError("selected producer exactly once before factory");
   for(int argument:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){JoinedCaptureEffects.trace="";
    if(operation.applyAsInt(argument)!=(argument^(seed^(choose?0x76543210:0x13579bdf)))||!JoinedCaptureEffects.trace.equals("B"))throw new AssertionError("branch selection or lazy body");rows++;}
  }
  for(boolean choose:new boolean[]{false,true}){JoinedCaptureEffects.trace="";try{owner.operation(17,choose);throw new AssertionError("missing producer failure");}catch(RuntimeException e){if(e!=JoinedCaptureEffects.failure||!JoinedCaptureEffects.trace.equals(choose?"L":"R"))throw new AssertionError("selected producer exception identity/order");rows++;}}
  System.out.println(rows+":conditional:local:lambda:choice:lazy:identity");
 }
}`

func TestAdversarialMemberLambdaConditionalCaptureRetainsSelectedValue(t *testing.T) {
	testNativeIndependentFamilyFixture(t, memberLambdaConditionalCaptureFixture, []string{"JoinedCaptureOwner"}, "JoinedCaptureDriver", "52:conditional:local:lambda:choice:lazy:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberLambdaConditionalCaptureRetainsSelectedWideWord(t *testing.T) {
	fixture := memberLambdaConditionalWideFixture()
	testNativeIndependentFamilyFixture(t, fixture, []string{"JoinedCaptureOwner"}, "JoinedCaptureDriver", "52:conditional:local:lambda:choice:lazy:identity\n", nativeLexicalExactSignatures)
}

func memberLambdaConditionalWideFixture() string {
	fixture := strings.NewReplacer(
		"int ", "long ", "new int[]", "new long[]", "Integer.MIN_VALUE", "Long.MIN_VALUE", "Integer.MAX_VALUE", "Long.MAX_VALUE",
		"IntUnaryOperator", "LongUnaryOperator", "applyAsInt", "applyAsLong", "int[]{", "long[]{",
		"int value", "long value", "0x76543210", "0x123456789abcdefL", "0x13579bdf", "0xfedcba987654321L",
	).Replace(memberLambdaConditionalCaptureFixture)
	// The observation count is independent of the captured numeric width.
	fixture = strings.Replace(fixture, "long rows=0", "int rows=0", 1)
	return fixture
}

func TestAdversarialMemberLambdaConditionalCaptureRetainsDisjointBranchStores(t *testing.T) {
	fixture := strings.Replace(memberLambdaConditionalCaptureFixture,
		"final int value=choose?JoinedCaptureEffects.left(seed):JoinedCaptureEffects.right(seed);",
		"final int value;if(choose){value=JoinedCaptureEffects.left(seed);}else{value=JoinedCaptureEffects.right(seed);}", 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"JoinedCaptureOwner"}, "JoinedCaptureDriver", "52:conditional:local:lambda:choice:lazy:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberLambdaConditionalCaptureRetainsSelectedReferenceMutation(t *testing.T) {
	fixture := memberLambdaConditionalReferenceFixture()
	testNativeIndependentFamilyFixture(t, fixture, []string{"JoinedCaptureOwner"}, "JoinedCaptureDriver", "52:conditional:local:lambda:choice:lazy:identity\n", nativeLexicalExactSignatures)
}

func memberLambdaConditionalReferenceFixture() string {
	return strings.NewReplacer(
		"class JoinedCaptureEffects", "class JoinedCaptureBox {int word;JoinedCaptureBox(int value){word=value;}}class JoinedCaptureEffects",
		"static int left", "static JoinedCaptureBox left", "static int right", "static JoinedCaptureBox right",
		"return seed^0x76543210;", "return new JoinedCaptureBox(seed^0x76543210);",
		"return seed^0x13579bdf;", "return new JoinedCaptureBox(seed^0x13579bdf);",
		"final int value=", "final JoinedCaptureBox value=",
		"return argument^value;", "return argument^value.word;",
		"return argument->{", "value.word^=0x55aa33cc;return argument->{",
		"(argument^(seed^(choose?0x76543210:0x13579bdf)))", "(argument^(seed^(choose?0x76543210:0x13579bdf))^0x55aa33cc)",
	).Replace(memberLambdaConditionalCaptureFixture)
}

func TestAdversarialMemberLambdaConditionalCaptureRetainsDisjointWideStores(t *testing.T) {
	fixture := strings.Replace(memberLambdaConditionalWideFixture(),
		"final long value=choose?JoinedCaptureEffects.left(seed):JoinedCaptureEffects.right(seed);",
		"final long value;if(choose){value=JoinedCaptureEffects.left(seed);}else{value=JoinedCaptureEffects.right(seed);}", 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"JoinedCaptureOwner"}, "JoinedCaptureDriver", "52:conditional:local:lambda:choice:lazy:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberLambdaConditionalCaptureRetainsDisjointReferenceStores(t *testing.T) {
	fixture := strings.Replace(memberLambdaConditionalReferenceFixture(),
		"final JoinedCaptureBox value=choose?JoinedCaptureEffects.left(seed):JoinedCaptureEffects.right(seed);",
		"final JoinedCaptureBox value;if(choose){value=JoinedCaptureEffects.left(seed);}else{value=JoinedCaptureEffects.right(seed);}", 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"JoinedCaptureOwner"}, "JoinedCaptureDriver", "52:conditional:local:lambda:choice:lazy:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberLambdaConditionalCaptureRetainsNestedBranchStores(t *testing.T) {
	fixture := strings.Replace(memberLambdaConditionalCaptureFixture,
		"final int value=choose?JoinedCaptureEffects.left(seed):JoinedCaptureEffects.right(seed);",
		"final int value;if(choose){if((seed&1)!=0){value=JoinedCaptureEffects.left(seed);}else{value=JoinedCaptureEffects.left(seed);}}else{value=JoinedCaptureEffects.right(seed);}", 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"JoinedCaptureOwner"}, "JoinedCaptureDriver", "52:conditional:local:lambda:choice:lazy:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberLambdaConditionalCaptureKeepsDisjointSlotLifetimes(t *testing.T) {
	fixture := strings.NewReplacer(
		"final int value=choose?JoinedCaptureEffects.left(seed):JoinedCaptureEffects.right(seed);",
		"{for(int scratch=0;scratch<3;scratch++){JoinedCaptureEffects.trace+=\"S\";}}final int value;if(choose){value=JoinedCaptureEffects.left(seed);}else{value=JoinedCaptureEffects.right(seed);}",
		"trace.equals(choose?\"L\":\"R\")", "trace.equals(choose?\"SSSL\":\"SSSR\")",
	).Replace(memberLambdaConditionalCaptureFixture)
	testNativeIndependentFamilyFixture(t, fixture, []string{"JoinedCaptureOwner"}, "JoinedCaptureDriver", "52:conditional:local:lambda:choice:lazy:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberLambdaConditionalCaptureSnapshotsEachLoopIteration(t *testing.T) {
	fixture := strings.NewReplacer(
		"java.util.function.IntUnaryOperator operation(int seed", "java.util.function.IntUnaryOperator[] operation(int seed",
		"final int value=choose?JoinedCaptureEffects.left(seed):JoinedCaptureEffects.right(seed);return argument->{JoinedCaptureEffects.trace+=\"B\";return argument^value;};",
		"final int value;if(choose){value=JoinedCaptureEffects.left(seed);}else{value=JoinedCaptureEffects.right(seed);}java.util.function.IntUnaryOperator[] operations=new java.util.function.IntUnaryOperator[3];for(int index=0;index<operations.length;index++){final int offset=index;operations[index]=argument->{JoinedCaptureEffects.trace+=\"B\";return argument^value^offset;};}return operations;",
		"java.util.function.IntUnaryOperator operation=owner.operation", "java.util.function.IntUnaryOperator[] operations=owner.operation",
		"for(int argument:new int[]{", "for(int offset=0;offset<operations.length;offset++)for(int argument:new int[]{",
		"operation.applyAsInt(argument)", "operations[offset].applyAsInt(argument)",
		"(argument^(seed^(choose?0x76543210:0x13579bdf)))", "(argument^(seed^(choose?0x76543210:0x13579bdf))^offset)",
	).Replace(memberLambdaConditionalCaptureFixture)
	fixture = strings.Replace(fixture, "for(int offset=0;offset<operations.length;offset++)", "if(operations.length!=3)throw new AssertionError(\"original loop count\");for(int offset=0;offset<operations.length;offset++)", 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"JoinedCaptureOwner"}, "JoinedCaptureDriver", "152:conditional:local:lambda:choice:lazy:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberLambdaConditionalCaptureInitializesBranchValueInsideEachIteration(t *testing.T) {
	fixture := strings.NewReplacer(
		"java.util.function.IntUnaryOperator operation(int seed", "java.util.function.IntUnaryOperator[] operation(int seed",
		"final int value=choose?JoinedCaptureEffects.left(seed):JoinedCaptureEffects.right(seed);return argument->{JoinedCaptureEffects.trace+=\"B\";return argument^value;};",
		"java.util.function.IntUnaryOperator[] operations=new java.util.function.IntUnaryOperator[3];for(int index=0;index<operations.length;index++){final int value;if(choose){value=JoinedCaptureEffects.left(seed)^index;}else{value=JoinedCaptureEffects.right(seed)^index;}operations[index]=argument->{JoinedCaptureEffects.trace+=\"B\";return argument^value;};}return operations;",
		"java.util.function.IntUnaryOperator operation=owner.operation", "java.util.function.IntUnaryOperator[] operations=owner.operation",
		"for(int argument:new int[]{", "for(int offset=0;offset<operations.length;offset++)for(int argument:new int[]{",
		"operation.applyAsInt(argument)", "operations[offset].applyAsInt(argument)",
		"(argument^(seed^(choose?0x76543210:0x13579bdf)))", "(argument^(seed^(choose?0x76543210:0x13579bdf))^offset)",
	).Replace(memberLambdaConditionalCaptureFixture)
	// Successful creation executes all three selected producers. A failing
	// producer still throws the original object on the first iteration.
	fixture = strings.Replace(fixture, "trace.equals(choose?\"L\":\"R\")", "trace.equals(choose?\"LLL\":\"RRR\")", 1)
	fixture = strings.Replace(fixture, "for(int offset=0;offset<operations.length;offset++)", "if(operations.length!=3)throw new AssertionError(\"original loop count\");for(int offset=0;offset<operations.length;offset++)", 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"JoinedCaptureOwner"}, "JoinedCaptureDriver", "152:conditional:local:lambda:choice:lazy:identity\n", nativeLexicalExactSignatures)
}
