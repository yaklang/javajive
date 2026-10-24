package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

// A bridge can preserve its direct parent's exact descriptor: it need not
// adapt an argument or covariant result. Mark an independently compiled super
// forwarding declaration as a bridge in the authored original, leaving all
// executable bytes untouched. The original JVM oracle then checks reflection,
// typed results and invokespecial on a further overriding subclass before the
// candidate is rendered. Only the leaf is owned by the source artifact.
func TestAdversarialExactSuperBridgePreservesDeclarationAndDispatch(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, shape := range []string{"reference", "long", "void"} {
		for _, root := range []string{"BridgeOuter", "RenamedExactScope"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				for _, major := range []uint16{49, 52} {
					t.Run(fmt.Sprintf("%s/%s/%s/major%d", shape, root, debug, major), func(t *testing.T) {
						fixture := exactSuperBridgeFixture(shape)
						fixture = strings.ReplaceAll(fixture, "BridgeOuter", root)
						testIndependentInheritedBridgeWithOriginal(t, modern, java, native, fixture, root, debug, false, "36:exact:super:bridge:result:identity\n", func(files map[string][]byte) {
							name := root + "$Leaf.class"
							object := mustParseLowering(t, bytes.Clone(files[name]))
							found := 0
							for _, m := range object.Methods {
								if n, _ := sourceBridgeUTF8(object, m.NameIndex); n == "read" {
									if m.AccessFlags != 1 {
										t.Fatal("authored ordinary super forwarding declaration")
									}
									m.AccessFlags = 0x1041
									found++
								}
							}
							if found != 1 {
								t.Fatal("exactly one original bridge declaration")
							}
							object.MajorVersion = major
							files[name] = object.Bytes()
						})
					})
				}
			}
		}
	}
}

func exactSuperBridgeFixture(shape string) string {
	result, body, forwarding, special, check := "Object", "return token;", "return super.read(n,d,token);", "return (Object)h.bindTo(this).invokeExact(n,d,v);", "result!=v"
	if shape == "long" {
		result, body = "long", "return n^Double.doubleToRawLongBits(d);"
		special = "return Long.valueOf((long)h.bindTo(this).invokeExact(n,d,v));"
		check = "!(result instanceof Long)||((Long)result).longValue()!=(n^Double.doubleToRawLongBits(d))"
	} else if shape == "void" {
		result, body, forwarding = "void", "", "super.read(n,d,token);"
		special, check = "h.bindTo(this).invokeExact(n,d,v);return null;", "result!=null"
	}
	return fmt.Sprintf(`class BridgeEffects {static int calls;static boolean fail;static Object observed;static final java.io.IOException error=new java.io.IOException("identity");}
class BridgeBase {public %s read(long n,double d,Object token)throws java.io.IOException {throw new AssertionError("wrong parent");}}
public class BridgeOuter extends BridgeBase {
 public static class Leaf extends BridgeOuter {public Leaf(){}public %s read(long n,double d,Object token)throws java.io.IOException {%s}}
 public %s read(long n,double d,Object token)throws java.io.IOException {BridgeEffects.calls++;BridgeEffects.observed=token;if(BridgeEffects.fail)throw BridgeEffects.error;%s}
 static Object leaf(){return new Leaf();}
}
class BridgeDerived extends BridgeOuter.Leaf {
 public %s read(long n,double d,Object v){throw new AssertionError("super bridge became virtual");}
 public Object special(long n,double d,Object v)throws Throwable {
  java.lang.invoke.MethodHandle h=java.lang.invoke.MethodHandles.lookup().findSpecial(BridgeOuter.Leaf.class,"read",java.lang.invoke.MethodType.methodType(%s.class,long.class,double.class,Object.class),BridgeDerived.class);
  %s
 }
}
class BridgeDriver {public static void main(String[]args)throws Exception {
 Object leaf=BridgeOuter.leaf();java.lang.reflect.Method bridge=null;
 for(java.lang.reflect.Method m:Class.forName("BridgeOuter$Leaf").getDeclaredMethods()){if(bridge!=null||!m.isSynthetic()||!m.isBridge()||m.getReturnType()!=%s.class)throw new AssertionError("exact original bridge universe");bridge=m;}
 if(bridge==null)throw new AssertionError("missing original bridge");
 if(args.length>0&&args[0].equals("special")){leaf=new BridgeDerived();bridge=BridgeDerived.class.getDeclaredMethod("special",long.class,double.class,Object.class);}
 bridge.setAccessible(true);int rows=0;Object shared=new Object();
 for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(double d:new double[]{-0.0,Double.NaN,Double.POSITIVE_INFINITY})for(Object v:new Object[]{null,shared})for(boolean fail:new boolean[]{false,true}) {
  BridgeEffects.fail=fail;BridgeEffects.calls=0;BridgeEffects.observed=new Object();boolean caught=false;
  try{Object result=bridge.invoke(leaf,n,d,v);if(%s)throw new AssertionError("typed result");}catch(java.lang.reflect.InvocationTargetException e){if(e.getCause()!=BridgeEffects.error)throw e;caught=true;}
  if(caught!=fail||BridgeEffects.calls!=1||BridgeEffects.observed!=v)throw new AssertionError("invocation target/effects/identity");rows++;
 }
 System.out.println(rows+":exact:super:bridge:result:identity");
}}
`, result, result, forwarding, result, body, result, result, special, result, check)
}
