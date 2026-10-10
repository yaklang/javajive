package javaclassparser

import (
	"strings"
	"testing"
)

// A local capture is a snapshot of the local's reference, while mutations of
// the referenced array remain observable. It cannot be replaced by an entry
// parameter, a fresh array, or a producer re-executed when the SAM is called.
const memberLambdaLocalCaptureFixture = `
class LocalCaptureEffects {static String trace="";static final RuntimeException failure=new RuntimeException("original");
 static int[] create(int seed){trace+="A";if(seed==17)throw failure;return new int[]{seed,Integer.MIN_VALUE,Integer.MAX_VALUE};}}
class LocalCaptureOwner {
 static class View {
  java.util.function.IntBinaryOperator comparator(int seed){final int[] words=LocalCaptureEffects.create(seed);java.util.function.IntBinaryOperator result=(left,right)->{LocalCaptureEffects.trace+="B";return Integer.compare(words[left],words[right]);};words[0]=seed^0x12345678;return result;}
 }
 static View view(){return new View();}
}
class LocalCaptureDriver {
 public static void main(String[]args){int rows=0;LocalCaptureOwner.View owner=LocalCaptureOwner.view();
  if(!owner.getClass().isMemberClass()||owner.getClass().getDeclaringClass()!=LocalCaptureOwner.class)throw new AssertionError("original member declaration identity");
  for(int seed:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
   LocalCaptureEffects.trace="";java.util.function.IntBinaryOperator compare=owner.comparator(seed);
   if(!LocalCaptureEffects.trace.equals("A"))throw new AssertionError("capture construction must be eager, body lazy");
   int[] expected={seed^0x12345678,Integer.MIN_VALUE,Integer.MAX_VALUE};
   for(int left=0;left<3;left++)for(int right=0;right<3;right++){
    LocalCaptureEffects.trace="";
    if(compare.applyAsInt(left,right)!=Integer.compare(expected[left],expected[right])||!LocalCaptureEffects.trace.equals("B"))throw new AssertionError("local reference snapshot/content mutation/body order");rows++;
   }
   LocalCaptureEffects.trace="";try{compare.applyAsInt(-1,0);throw new AssertionError("missing bounds failure");}catch(ArrayIndexOutOfBoundsException e){if(!LocalCaptureEffects.trace.equals("B"))throw new AssertionError("lazy failure order");rows++;}
  }
  LocalCaptureEffects.trace="";try{owner.comparator(17);throw new AssertionError("missing producer failure");}catch(RuntimeException e){if(e!=LocalCaptureEffects.failure||!LocalCaptureEffects.trace.equals("A"))throw new AssertionError("capture producer exception identity/order");rows++;}
  System.out.println(rows+":local:lambda:capture:snapshot:mutation:lazy:failure");
 }
}`

func TestAdversarialMemberLambdaLocalCapturePreservesOriginalSnapshot(t *testing.T) {
	testNativeIndependentFamilyFixture(t, memberLambdaLocalCaptureFixture, []string{"LocalCaptureOwner"}, "LocalCaptureDriver", "51:local:lambda:capture:snapshot:mutation:lazy:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberLambdaLocalCaptureKeepsEqualTypedOperandsDistinct(t *testing.T) {
	fixture := strings.NewReplacer(
		"final int[] words=LocalCaptureEffects.create(seed);", "final int[] words=LocalCaptureEffects.create(seed);final int[] other=LocalCaptureEffects.create(seed^0x13579bdf);",
		"Integer.compare(words[left],words[right])", "Integer.compare(words[left],other[right])",
		"words[0]=seed^0x12345678;", "words[0]=seed^0x12345678;other[1]=seed+7;",
		"trace.equals(\"A\"))throw new AssertionError(\"capture construction", "trace.equals(\"AA\"))throw new AssertionError(\"capture construction",
		"int[] expected={seed^0x12345678,Integer.MIN_VALUE,Integer.MAX_VALUE};", "int[] expected={seed^0x12345678,Integer.MIN_VALUE,Integer.MAX_VALUE};int[] rightExpected={seed^0x13579bdf,seed+7,Integer.MAX_VALUE};",
		"Integer.compare(expected[left],expected[right])", "Integer.compare(expected[left],rightExpected[right])",
	).Replace(memberLambdaLocalCaptureFixture)
	testNativeIndependentFamilyFixture(t, fixture, []string{"LocalCaptureOwner"}, "LocalCaptureDriver", "51:local:lambda:capture:snapshot:mutation:lazy:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberLambdaLocalCaptureRetainsReceiverAndWideParameter(t *testing.T) {
	fixture := strings.NewReplacer(
		"static class View {", "static class View {int direction=-1;",
		"comparator(int seed)", "comparator(long padding,int seed)",
		"Integer.compare(words[left],words[right])", "Integer.compare(words[left],words[right])*(padding==Long.MIN_VALUE?this.direction:-this.direction)",
		"owner.comparator(seed)", "owner.comparator((seed&1)!=0?Long.MIN_VALUE:0x123456789abcdefL,seed)",
		"Integer.compare(expected[left],expected[right])", "Integer.compare(expected[left],expected[right])*((seed&1)!=0?-1:1)",
		"owner.comparator(17)", "owner.comparator(Long.MIN_VALUE,17)",
	).Replace(memberLambdaLocalCaptureFixture)
	testNativeIndependentFamilyFixture(t, fixture, []string{"LocalCaptureOwner"}, "LocalCaptureDriver", "51:local:lambda:capture:snapshot:mutation:lazy:failure\n", nativeLexicalExactSignatures)
}
