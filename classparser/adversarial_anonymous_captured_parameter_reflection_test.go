package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// The original parent observes the synthetic capture before its constructor
// completes. Keep both that parent and the driver as unchanged binary callers.
func TestAdversarialAnonymousCapturedParameterReflectionRoundTrip(t *testing.T) {
	const fixture = `
class ForeignMergeRoot {
 static class MergeBase {
  final java.util.concurrent.CompletableFuture<Boolean> future=new java.util.concurrent.CompletableFuture<Boolean>();final java.util.List<Integer> values;final int total;Object observed;
  MergeBase(java.util.List<Integer> input){if(input.size()==0)throw new IllegalArgumentException("empty");values=new java.util.ArrayList<Integer>(input);total=input.stream().mapToInt(Integer::intValue).sum();try{java.lang.reflect.Field f=getClass().getDeclaredField("val$supplier");f.setAccessible(true);observed=f.get(this);}catch(ReflectiveOperationException e){throw new AssertionError(e);}}
  Object retained(){return null;}
 }
}
class MergeLambdaOwner {
 static java.util.function.Function<java.util.List<Integer>,ForeignMergeRoot.MergeBase> bind(java.util.function.Supplier<Object> supplier){return input->new ForeignMergeRoot.MergeBase(input){Object retained(){return supplier.get();}};}
 static ForeignMergeRoot.MergeBase direct(java.util.List<Integer> input,java.util.function.Supplier<Object> supplier){return new ForeignMergeRoot.MergeBase(input){Object retained(){return supplier.get();}};}
}
class MergeLambdaDriver {public static void main(String[]args){Object token=new Object();java.util.function.Supplier<Object>supplier=()->token;java.util.function.Function<java.util.List<Integer>,ForeignMergeRoot.MergeBase>factory=MergeLambdaOwner.bind(supplier);int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int size=1;size<=8;size++){java.util.List<Integer>input=new java.util.ArrayList<Integer>();int total=0;for(int i=0;i<size;i++){input.add(n+i);total+=n+i;}for(ForeignMergeRoot.MergeBase m:new ForeignMergeRoot.MergeBase[]{factory.apply(input),MergeLambdaOwner.direct(input,supplier)}){if(m.total!=total||m.retained()!=token||m.observed!=supplier||m.values==input||!m.values.equals(input)||m.future.isDone())throw new AssertionError("merge binding");rows++;}}if(rows!=80)throw new AssertionError("coverage");System.out.println("80:merge:lambda:storage:identity:overflow");}}
`
	for _, capture := range []string{"supplier", "gate"} {
		t.Run(capture, func(t *testing.T) {
			source := strings.ReplaceAll(fixture, "supplier", capture)
			source = strings.ReplaceAll(source, "MergeLambdaOwner", "Factory"+capture)
			owner := "Factory" + capture
			t.Run("modern", func(t *testing.T) {
				testNativeIndependentFamilyFixture(t, source, []string{owner}, "MergeLambdaDriver", "80:merge:lambda:storage:identity:overflow\n", nativeLexicalExactSignatures)
			})
			t.Run("original8", func(t *testing.T) {
				compiler := os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required for actual original compiler")
				}
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, source, owner, debug) }, NativeJavac8, compiler, []string{owner}, "MergeLambdaDriver", "80:merge:lambda:storage:identity:overflow\n", nil, nativeLexicalExactSignatures)
			})
		})
	}
}
