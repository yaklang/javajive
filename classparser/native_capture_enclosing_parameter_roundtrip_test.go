package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const nativeEnclosingParameterCaptureFixture = `abstract class ReceiverBox<E>{ReceiverBox(int word){ReceiverEffects.trace+="B";ReceiverEffects.published=this;ReceiverEffects.word=word;if(ReceiverEffects.fail)throw ReceiverEffects.error;}abstract int compare(E a,E b);}
class ReceiverHolder {final ReceiverBox<Number> box;ReceiverHolder(ReceiverBox<Number> b){box=b;}}
class ReceiverEffects {static String trace="";static Object published;static int word;static boolean fail;static final RuntimeException error=new IllegalStateException("identity");}
class ReceiverScope {final int size;ReceiverScope(int n){size=n;}int capacity(){ReceiverEffects.trace+="A";return size;}
 final class Cell {final ReceiverBox<Number> box;Cell(){box=new ReceiverBox<Number>(capacity()){int compare(Number a,Number b){int x=a.intValue(),y=b.intValue();return x<y?-1:x==y?0:1;}};}}
}
class ReceiverDriver {public static void main(String[]args)throws Exception{int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(boolean fail:new boolean[]{false,true}){ReceiverScope owner=new ReceiverScope(n);ReceiverEffects.fail=fail;ReceiverEffects.trace="";ReceiverEffects.published=null;ReceiverScope.Cell cell=null;try{cell=owner.new Cell();if(fail)throw new AssertionError("missing failure");}catch(RuntimeException e){if(!fail||e!=ReceiverEffects.error)throw new AssertionError("failure identity",e);}Object box=ReceiverEffects.published;if(box==null||ReceiverEffects.word!=n||!ReceiverEffects.trace.equals("AB"))throw new AssertionError("evaluation/init order");java.lang.reflect.Field f=box.getClass().getDeclaredField("val$this$0");f.setAccessible(true);if(!f.isSynthetic()||f.getModifiers()!=0x1010||f.get(box)!=owner)throw new AssertionError("captured original enclosing identity");if(!fail){if(cell.box!=box)throw new AssertionError("allocation identity");for(int x:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int y:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){int expected=x<y?-1:x==y?0:1;if(cell.box.compare(Integer.valueOf(x),Long.valueOf(y))!=expected)throw new AssertionError("comparison");rows++;}}}System.out.println(rows+":receiver:capture:metadata:effects");}}
`

func testNativeEnclosingParameterCapture(t *testing.T, native bool) {
	shapes := []string{"body", "delegating", "renamed"}
	if !native {
		shapes = append(shapes, "super-argument")
	}
	for _, shape := range shapes {
		t.Run(shape, func(t *testing.T) {
			source := nativeEnclosingParameterCaptureFixture
			owner := "ReceiverScope"
			if shape == "super-argument" {
				source = strings.Replace(source, "final class Cell {final ReceiverBox<Number> box;Cell(){box=new ReceiverBox<Number>(capacity())", "final class Cell extends ReceiverHolder {Cell(){super(new ReceiverBox<Number>(capacity())", 1)
				source = strings.Replace(source, "return x<y?-1:x==y?0:1;}};}}", "return x<y?-1:x==y?0:1;}});}}", 1)
			}
			if shape == "delegating" {
				source = strings.Replace(source, "Cell(){box=", "Cell(){this(0);}Cell(int unused){box=", 1)
			}
			if shape == "renamed" {
				source = strings.ReplaceAll(source, "ReceiverScope", "RenamedEnclosingScope")
				owner = "RenamedEnclosingScope"
			}
			if native {
				javac := os.Getenv("JAVA8_JAVAC")
				if javac == "" {
					t.Skip("JAVA8_JAVAC required")
				}
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, source, owner, debug) }, NativeJavac8, javac, []string{owner}, "ReceiverDriver", "125:receiver:capture:metadata:effects\n", nil, nativeLexicalExactSignatures, nativeCovariantBridgeABI)
			} else {
				testNativePrivateSetterCompiledFixture(t, owner, "ReceiverDriver", "125:receiver:capture:metadata:effects\n", func(t *testing.T, debug string) map[string][]byte {
					return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": source}, debug, "8")
				}, nativeLexicalExactSignatures, nativeCovariantBridgeABI)
			}
		})
	}
}
func TestAdversarialEnclosingParameterCapture(t *testing.T) {
	testNativeEnclosingParameterCapture(t, false)
}
func TestAdversarialEnclosingParameterCaptureNativeCompiler(t *testing.T) {
	testNativeEnclosingParameterCapture(t, true)
}
