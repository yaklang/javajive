package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

const finalInheritedBridgeFixture = `
interface FinalRoot {Object value();}
interface FinalTyped<T> extends FinalRoot {T value();}
class FinalEffects {static int calls;static boolean fail;static final RuntimeException error=new IllegalStateException("identity");}
class FinalBase<T extends FinalBase<T>> implements FinalRoot {
 public final T value(){FinalEffects.calls++;if(FinalEffects.fail)throw FinalEffects.error;return (T)this;}
}
public abstract class FinalBridgeOwner<T extends FinalBridgeOwner<T>> extends FinalBase<T> implements FinalTyped<T> {
 public Object forwardForOriginal(){return super.value();}
 static class Leaf extends FinalBridgeOwner<Leaf>{}
}
class BridgeDriver {public static void main(String[]args)throws Throwable{
 java.lang.reflect.Method bridge=FinalBridgeOwner.class.getDeclaredMethod("value");
 if(!bridge.isBridge()||!bridge.isSynthetic()||bridge.getReturnType()!=Object.class||FinalBridgeOwner.class.getDeclaredMethods().length!=1)throw new AssertionError("bridge declaration universe");
 Object leaf=new FinalBridgeOwner.Leaf();int rows=0;for(boolean fail:new boolean[]{false,true})for(int i=0;i<3;i++){
  FinalEffects.fail=fail;FinalEffects.calls=0;Object got;
  try{got=i==0?bridge.invoke(leaf):i==1?((FinalRoot)leaf).value():((FinalTyped)leaf).value();}catch(java.lang.reflect.InvocationTargetException e){got=e.getCause();}catch(Throwable e){got=e;}
  if(got!=(fail?FinalEffects.error:leaf)||FinalEffects.calls!=1)throw new AssertionError("final target/effect/identity");rows++;
 }System.out.println(rows+":final:inherited:bridge:identity:effects");}}
`

func TestAdversarialFinalInheritedBridgeKeepsOriginalDeclaration(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original native compiler required")
	}
	for _, owner := range []string{"FinalBridgeOwner", "DifferentFinalOwner"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(fmt.Sprintf("%s/%s", owner, debug), func(t *testing.T) {
				fixture := strings.ReplaceAll(finalInheritedBridgeFixture, "FinalBridgeOwner", owner)
				// javac8 omits this redundant bridge in some source universes. The JVM
				// protocol remains legal: retitle an independently compiled super-forwarder,
				// preserving its Code and setting its original bridge/synthetic flags. The
				// unchanged original JVM observer runs before any reconstruction.
				testIndependentInheritedBridgeOwner(t, modern, java, native, fixture, owner, owner, debug, false, "6:final:inherited:bridge:identity:effects\n", func(files map[string][]byte) {
					obj := mustParseLowering(t, files[owner+".class"])
					found := 0
					for _, m := range obj.Methods {
						name, _ := sourceBridgeUTF8(obj, m.NameIndex)
						if name == "forwardForOriginal" {
							m.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("value"))
							m.AccessFlags = 0x1041
							found++
						}
					}
					if found != 1 {
						t.Fatal("authored forwarding declaration")
					}
					files[owner+".class"] = obj.Bytes()
				})
			})
		}
	}
}
