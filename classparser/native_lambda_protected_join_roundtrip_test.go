package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Throw before either definition and after the branch join. Original-first
// drivers check exceptional identity, handler completion, wide arithmetic and
// exactly-once effects; catch entry cannot borrow the try's normal exit state.
func nativeLambdaProtectedJoinFixture(variant string) (string, string, string) {
	branch := `if((input&1)==0)captured=input+3;else captured=input-7;`
	producers := `provoke(input,1);` + branch + `provoke(input,2);`
	handler := `if(error!=MARKER)throw new AssertionError("exception identity");throw error;`
	body := `try{final long captured;` + producers + `return value->value+(int)captured;}catch(RuntimeException error){` + handler + `}`
	switch variant {
	case "outside protected join":
		body = `final long captured;try{` + producers + `}catch(RuntimeException error){` + handler + `}return value->value+(int)captured;`
	case "returning handler":
		body = `final long captured;try{` + producers + `}catch(RuntimeException error){if(error!=MARKER)throw new AssertionError("exception identity");return value->value+7;}return value->value+(int)captured;`
	case "nested handler":
		body = `try{try{final long captured;` + producers + `return value->value+(int)captured;}catch(IllegalArgumentException inner){throw inner;}}catch(RuntimeException error){` + handler + `}`
	case "nested protected join":
		body = `try{final long captured;try{` + producers + `}catch(IllegalArgumentException inner){throw inner;}return value->value+(int)captured;}catch(RuntimeException error){` + handler + `}`
	case "narrow arithmetic":
		body = strings.ReplaceAll(body, "long captured", "int captured")
		body = strings.ReplaceAll(body, "captured=input", "captured=(int)input")
	}
	source := strings.Replace(nativeRootBridgeHandleSource("lambda"), `static java.util.function.IntUnaryOperator lambda(){return value->value+3;}`, `static int trace;static final RuntimeException MARKER=new IllegalStateException("sentinel");
 static void provoke(long input,int stage){trace+=stage;if(input==-1&&stage==1||input==1&&stage==2)throw MARKER;}
 static java.util.function.IntUnaryOperator lambda(long input){`+body+`}`, 1)
	start := strings.Index(source, "class BridgeHandleDriver")
	source = source[:start] + `class BridgeHandleDriver{public static void main(String[]args){
 for(long input:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
  BridgeHandleRoot root=BridgeHandleRoot.Factory.make(input);if(root.value!=input)throw new AssertionError("constructor value");
  try{java.util.function.IntUnaryOperator operator=BridgeHandleRoot.lambda(input);
   boolean exceptional=input==-1||input==1;
   if(exceptional&&!RETURNING)throw new AssertionError("missing exception");
   int payload=exceptional?7:(int)(input+((input&1)==0?3:-7));
   for(int argument:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})if(operator.applyAsInt(argument)!=argument+payload)throw new AssertionError("captured value/overflow");
  }catch(RuntimeException error){if(RETURNING||error!=BridgeHandleRoot.MARKER||input!=-1&&input!=1)throw new AssertionError("exception identity",error);}
 }
 if(BridgeHandleRoot.trace!=13)throw new AssertionError("reordered/duplicated effects");
 System.out.println("protected-join:verified");}}
`
	returning := "false"
	if variant == "returning handler" {
		returning = "true"
	}
	source = strings.ReplaceAll(source, "RETURNING", returning)
	owner, driver := "BridgeHandleRoot", "BridgeHandleDriver"
	if variant == "renamed" {
		owner, driver = "ProtectedCaptureScope", "ProtectedCaptureDriver"
		source = strings.NewReplacer("BridgeHandleRoot", owner, "BridgeHandleDriver", driver, "captured", "chosen", "provoke", "observe").Replace(source)
	}
	return source, owner, driver
}

func TestNativeLambdaProtectedBranchJoinRoundTrip(t *testing.T) {
	for _, variant := range []string{"inside protected join", "outside protected join", "returning handler", "nested handler", "nested protected join", "narrow arithmetic", "renamed"} {
		t.Run(variant, func(t *testing.T) {
			source, owner, driver := nativeLambdaProtectedJoinFixture(variant)
			t.Run("modern", func(t *testing.T) {
				testNativeIndependentFamilyFixture(t, source, []string{owner}, driver, "protected-join:verified\n", nativeLexicalExactSignatures)
			})
			t.Run("native8", func(t *testing.T) {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return localGetterCompileJavac8(t, map[string]string{"BridgeHandleRoot.java": source}, debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{owner}, driver, "protected-join:verified\n", nil, nativeLexicalExactSignatures)
			})
		})
	}
}
