package javaclassparser

import "testing"

// This is an ordinary javac-generated modern nest with no protocol changes.
// Its private implementation is used only by its original factory. The retained
// Driver checks the complete owner method set, including its synthetic helper.
func TestAdversarialMethodSourceOriginalModernNestLambdaRoundTrip(t *testing.T) {
	fixture := `class NestLambdaOwner{static class Anchor{}static java.util.function.IntUnaryOperator first(int captured){return value->captured+value;}}class NestLambdaDriver{public static void main(String[]args)throws Exception{if(NestLambdaOwner.first(7).applyAsInt(2)!=9)throw new AssertionError("modern nest lambda");java.lang.reflect.Method m=NestLambdaOwner.class.getDeclaredMethod("lambda$first$0",int.class,int.class);if(NestLambdaOwner.class.getDeclaredMethods().length!=2||!m.isSynthetic()||m.getModifiers()!=4106)throw new AssertionError("complete modern owner method set");System.out.println("nest-lambda:original");}}`
	testSourceTargetReleaseFamilyFixture(t, fixture, "NestLambdaOwner", "NestLambdaDriver", "nest-lambda:original\n", "11", []int{11})
}
