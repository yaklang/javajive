package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// Nested lambda methods may be renamed by a different javac, while an ordinary
// access declaration loses the unspellable synthetic flag. Neither independent
// obligation may hide a method-universe or visibility change in the other.
const lambdaSyntheticCompositionFixture = `
class SyntheticLambdaOwner {
 private static int salt=0x5a;
 static class Reader {static int read(){return salt;}}
 static java.util.function.IntUnaryOperator choose(final int delta){
  return n->{java.util.function.IntUnaryOperator step=x->x+delta;return step.applyAsInt(n)^Reader.read();};
 }
}
class SuffixDriver {public static void main(String[]args)throws Exception{
 int accessors=0;for(java.lang.reflect.Method m:SyntheticLambdaOwner.class.getDeclaredMethods()){
  if(m.getParameterCount()==0&&m.getReturnType()==int.class){
   if(!m.isSynthetic()||!java.lang.reflect.Modifier.isStatic(m.getModifiers())||java.lang.reflect.Modifier.isPublic(m.getModifiers()))throw new AssertionError("access declaration flags");accessors++;
  }
 }
 if(accessors!=1)throw new AssertionError("declaration universe");
 int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,0x12345678,Integer.MAX_VALUE})for(int delta:new int[]{Integer.MIN_VALUE,-7,0,Integer.MAX_VALUE}){
  int got=SyntheticLambdaOwner.choose(delta).applyAsInt(n);
  int expected=java.math.BigInteger.valueOf(n).add(java.math.BigInteger.valueOf(delta)).intValue()^0x5a;
  if(got!=expected)throw new AssertionError("nested closure result");rows++;
 }System.out.println(rows+":lambda:ordinary-synthetic:binding:overflow");}}
`

func TestAdversarialLambdaAndOrdinarySyntheticMetadataCompose(t *testing.T) {
	for _, owner := range []string{"SyntheticLambdaOwner", "DifferentClosureOwner"} {
		for _, profile := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("%s/%s/%s", owner, profile, debug), func(t *testing.T) {
					t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
					fixture := strings.ReplaceAll(lambdaSyntheticCompositionFixture, "SyntheticLambdaOwner", owner)
					constructorSuffixFamilyWithOriginalCompilerRoundTrip(t, fixture, debug, []string{owner, owner + "$Reader"}, "24:lambda:ordinary-synthetic:binding:overflow\n", 8, 8, ModernJavac, profile)
				})
			}
		}
	}
}
