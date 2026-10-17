package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestAdversarialBooleanCaptureComputationalWordRoundTrip(t *testing.T) {
	const fixture = `
class BooleanFactory {
 boolean value; boolean hasFinished(){return false;} void observe(boolean b){value=b;}
 static int numeric(boolean keep){int word=keep?0:1;return word;}
 synchronized void run(boolean keep){if(!hasFinished()){boolean discard=!keep;java.util.function.Consumer<Object> action=item->observe(discard);action.accept(null);}}
 synchronized void staged(boolean keep){if(!hasFinished()){boolean discard=!keep;BooleanFactory owner=this;boolean retained=discard;java.util.function.Consumer<Object> action=item->owner.observe(retained);action.accept(null);}}
}
class BooleanFactoryDriver{public static void main(String[]args){for(boolean b:new boolean[]{false,true}){BooleanFactory f=new BooleanFactory();if(BooleanFactory.numeric(b)!=(b?0:1))throw new AssertionError("numeric producer");f.run(b);if(f.value==b)throw new AssertionError("capture");f.staged(b);if(f.value==b)throw new AssertionError("staged");}System.out.println("4:boolean:capture");}}
`
	for _, owner := range []string{"BooleanFactory", "DecisionCapture"} {
		t.Run(owner, func(t *testing.T) {
			source := strings.ReplaceAll(fixture, "BooleanFactory", owner)
			t.Run("modern", func(t *testing.T) {
				testNativeIndependentFamilyFixture(t, source, []string{owner}, owner+"Driver", "4:boolean:capture\n", nativeLexicalExactSignatures)
			})
			t.Run("original8", func(t *testing.T) {
				compiler := os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required for actual original compiler")
				}
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, source, owner, debug) }, NativeJavac8, compiler, []string{owner}, owner+"Driver", "4:boolean:capture\n", nil, nativeLexicalExactSignatures)
			})
		})
	}
}
