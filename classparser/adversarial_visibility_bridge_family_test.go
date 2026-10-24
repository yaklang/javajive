package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// javac supplies an access bridge for a public class whose implementation is
// inherited from a package-private parent. Writing an explicit override here
// can introduce an additional generic interface bridge into the child's ABI.
const inheritedVisibilityBridgeFixture = `
interface BridgeContract<T> {T read(T value);}
class BridgeHidden implements BridgeContract<String> {
 static int calls; static boolean fail; static final IllegalStateException error=new IllegalStateException("identity");
 public String read(String value){calls++;if(fail)throw error;return value;}
}
public class BridgeOuter {
 public static class Leaf extends BridgeHidden {public Leaf(){}}
 static Object leaf(){return new Leaf();}
}
class BridgeDriver {public static void main(String[]args)throws Exception {
 Object leaf=BridgeOuter.leaf();java.lang.reflect.Method[] methods=leaf.getClass().getDeclaredMethods();
 if(methods.length!=1||!methods[0].isBridge()||!methods[0].isSynthetic()||methods[0].getReturnType()!=String.class||methods[0].getParameterTypes()[0]!=String.class)throw new AssertionError("access bridge method universe");
 methods[0].setAccessible(true);int rows=0;String shared=new String("shared");
 for(String value:new String[]{null,shared})for(boolean fail:new boolean[]{false,true}){
  BridgeHidden.fail=fail;BridgeHidden.calls=0;boolean caught=false;
  try{if(methods[0].invoke(leaf,value)!=value)throw new AssertionError("reference identity");}catch(java.lang.reflect.InvocationTargetException e){if(e.getCause()!=BridgeHidden.error)throw e;caught=true;}
  if(caught!=fail||BridgeHidden.calls!=1)throw new AssertionError("reflection call and exception identity");
  BridgeHidden.calls=0;caught=false;
  try{if(((BridgeContract)leaf).read(value)!=value)throw new AssertionError("erased interface result");}catch(IllegalStateException e){if(e!=BridgeHidden.error)throw e;caught=true;}
  if(caught!=fail||BridgeHidden.calls!=1)throw new AssertionError("inherited erased dispatch");rows++;
 }
 System.out.println(rows+":access:bridge:inherited:generic:identity");
}}
`

func TestAdversarialVisibilityBridgeDoesNotInventCovariantDeclarations(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	// Parameter erasure and result covariance are independent reasons for a
	// compiler bridge; cover either axis alone as well as their composition.
	for _, declaration := range []struct{ name, method string }{
		{"argument-and-result", "T read(T value);"},
		{"argument-only", "String read(T value);"},
		{"result-only", "T read(String value);"},
	} {
		for _, root := range []string{"BridgeOuter", "RenamedVisibilityScope"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				for _, parameters := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/%s/parameters%v", declaration.name, root, debug, parameters), func(t *testing.T) {
						fixture := strings.Replace(inheritedVisibilityBridgeFixture, "T read(T value);", declaration.method, 1)
						testIndependentInheritedBridge(t, modern, java, native, strings.ReplaceAll(fixture, "BridgeOuter", root), root, debug, parameters, "4:access:bridge:inherited:generic:identity\n")
					})
				}
			}
		}
	}
}
