package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const forwardAllocationWorld = `
class ForwardEffects{static int calls,mode;static Object published;static String trace="";static RuntimeException failure=new RuntimeException("identity");static int step(int x){int n=++calls;trace+="a";if(mode==n)throw failure;return x*31+n;}}
class ForwardValue{final int value;ForwardValue(int n){ForwardEffects.trace+="N";ForwardEffects.published=this;value=n;if(ForwardEffects.mode==25)throw ForwardEffects.failure;}}
class ForwardOwner{static int evaluate(int seed){try{ForwardValue v=new ForwardValue(CHAIN);return v.value;}finally{ForwardEffects.trace+="F";}}}
class ForwardDriver{public static void main(String[]args){int rows=0;for(int seed:new int[]{Integer.MIN_VALUE,-99,-1,0,1,99,Integer.MAX_VALUE})for(int mode:new int[]{0,1,12,24,25}){ForwardEffects.calls=0;ForwardEffects.mode=mode;ForwardEffects.trace="";ForwardEffects.published=null;java.math.BigInteger exact=java.math.BigInteger.valueOf(seed);for(int i=1;i<=24;i++)exact=java.math.BigInteger.valueOf(exact.multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(i)).intValue());int expected=exact.intValue();boolean failed=false;try{if(ForwardOwner.evaluate(seed)!=expected)throw new AssertionError("arithmetic");}catch(RuntimeException e){if(e!=ForwardEffects.failure)throw e;failed=true;}int calls=mode==0||mode==25?24:mode;boolean constructed=mode==0||mode==25;StringBuilder trace=new StringBuilder();for(int i=0;i<calls;i++)trace.append('a');if(constructed)trace.append('N');trace.append('F');if(failed!=(mode!=0)||ForwardEffects.calls!=calls||!ForwardEffects.trace.equals(trace.toString()))throw new AssertionError("effect/failure order");if(constructed){if(!(ForwardEffects.published instanceof ForwardValue)||((ForwardValue)ForwardEffects.published).value!=expected)throw new AssertionError("partial object identity/value");}else if(ForwardEffects.published!=null)throw new AssertionError("premature constructor");rows++;}System.out.println(rows+":allocation:overflow:abrupt:partial:finally");}}
`

func TestNativeForwardedAllocationControlRoundTrip(t *testing.T) {
	testNativeForwardedAllocationControlRoundTrip(t, false)
}
func TestNativeForwardedAllocationControlNativeRoundTrip(t *testing.T) {
	testNativeForwardedAllocationControlRoundTrip(t, true)
}
func testNativeForwardedAllocationControlRoundTrip(t *testing.T, native bool) {
	chain := "seed"
	for i := 0; i < 24; i++ {
		chain = "ForwardEffects.step(" + chain + ")"
	}
	fixture := strings.Replace(forwardAllocationWorld, "CHAIN", chain, 1)
	javac, _ := t04Tools(t)
	profile := ModernJavac
	if native {
		javac = os.Getenv("JAVA8_JAVAC")
		if javac == "" {
			t.Fatal("JAVA8_JAVAC required")
		}
		profile = NativeJavac8
	}
	compile := func(debug string) map[string][]byte {
		if native {
			return nativePrivateEnumCompile(t, fixture, "ForwardOwner", debug)
		}
		return nativeCompileDebugClasses(t, fixture, debug)
	}
	testNativeIndependentCompilerFamilyFixture(t, compile, profile, javac, []string{"ForwardOwner"}, "ForwardDriver", "35:allocation:overflow:abrupt:partial:finally\n", nil, nativeLexicalExactSignatures)
}
