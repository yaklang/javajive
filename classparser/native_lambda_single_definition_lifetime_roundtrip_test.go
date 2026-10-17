package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// A slot belongs to verifier lifetimes, not to one source variable for the
// whole method. Both branch-local arrays die before the final lambda capture.
// Keep their observable effects and the factory-time snapshot independently.
func nativeLambdaSingleLifetimeFixture() string {
	source := nativeRootBridgeHandleSource("lambda")
	source = strings.Replace(source, "static java.util.function.IntUnaryOperator lambda(){return value->value+3;}", `static int trace;
 static void observe(Object[] values){trace+=values.length;}
 static java.util.function.IntUnaryOperator lambda(int branch){
  if(branch==0){Object[] scratch=new Object[2];observe(scratch);}
  else{Object[] scratch=new Object[3];observe(scratch);}
  final int[] captured=new int[]{3};
  return value->value+captured[0];
 }`, 1)
	source = strings.Replace(source, "BridgeHandleRoot.lambda()", "BridgeHandleRoot.lambda((int)(value&1))", 1)
	source = strings.Replace(source, `System.out.println("root-bridge:unrelated-handles:verified");`, `if(BridgeHandleRoot.trace!=13)throw new AssertionError("factory effects exactly once");System.out.println("root-bridge:unrelated-handles:verified");`, 1)
	return source
}

func TestNativeLambdaSingleDefinitionDisjointLifetimesRoundTrip(t *testing.T) {
	for _, variant := range []string{"different types", "same array type", "old counter loop", "caught exception lifetime", "renamed", "reference cell", "shared factories"} {
		t.Run(variant, func(t *testing.T) {
			source := nativeLambdaSingleLifetimeFixture()
			owner, driver := "BridgeHandleRoot", "BridgeHandleDriver"
			switch variant {
			case "same array type":
				source = strings.ReplaceAll(source, "Object[] scratch=new Object[", "int[] scratch=new int[")
				source = strings.ReplaceAll(source, "observe(scratch)", "observeInts(scratch)")
				source = strings.Replace(source, "static int trace;", "static int trace;static void observeInts(int[] values){trace+=values.length;}", 1)
			case "old counter loop":
				source = strings.Replace(source, "if(branch==0){Object[] scratch=new Object[2];observe(scratch);}\n  else{Object[] scratch=new Object[3];observe(scratch);}", "for(int counter=0;counter<(branch==0?2:3);counter++){trace++;}", 1)
			case "caught exception lifetime":
				source = strings.Replace(source, "if(branch==0){Object[] scratch=new Object[2];observe(scratch);}\n  else{Object[] scratch=new Object[3];observe(scratch);}", `try{Object[] scratch=new Object[branch==0?2:3];observe(scratch);if(branch!=0)throw new IllegalArgumentException("caught");}catch(IllegalArgumentException error){if(!error.getMessage().equals("caught"))throw new AssertionError("wrong caught value");}`, 1)
			case "renamed":
				owner, driver = "ReusedCaptureScope", "ReusedCaptureDriver"
				source = strings.NewReplacer("BridgeHandleRoot", owner, "BridgeHandleDriver", driver, "captured", "packet", "scratch", "temporary").Replace(source)
			case "reference cell":
				source = strings.Replace(source, "final int[] captured=new int[]{3};", "final LifetimeValueCell captured=new LifetimeValueCell(3);", 1)
				source = strings.Replace(source, "captured[0]", "captured.value", 1)
				source += "class LifetimeValueCell{final int value;LifetimeValueCell(int value){this.value=value;}}"
			case "shared factories":
				source = strings.Replace(source, "return value->value+captured[0];", `final int[] other=new int[]{5};java.util.function.IntUnaryOperator first=value->value+captured[0];java.util.function.IntUnaryOperator second=value->value+other[0];return branch==0?first:second;`, 1)
				source = strings.Replace(source, "BridgeHandleRoot.lambda((int)(value&1)).applyAsInt(Integer.MAX_VALUE)!=Integer.MIN_VALUE+2", "BridgeHandleRoot.lambda((int)(value&1)).applyAsInt(Integer.MAX_VALUE)!=((value&1)==0?Integer.MIN_VALUE+2:Integer.MIN_VALUE+4)", 1)
			}
			t.Run("modern", func(t *testing.T) {
				testNativeIndependentFamilyFixture(t, source, []string{owner}, driver, "root-bridge:unrelated-handles:verified\n", nativeLexicalExactSignatures)
			})
			t.Run("native8", func(t *testing.T) {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return localGetterCompileJavac8(t, map[string]string{"BridgeHandleRoot.java": source}, debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{owner}, driver, "root-bridge:unrelated-handles:verified\n", nil, nativeLexicalExactSignatures)
			})
		})
	}
}
