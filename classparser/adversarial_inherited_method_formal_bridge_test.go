package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The method's own variable is below Class/BridgeNarrow, so instantiating it
// cannot alter any physical invocation port. The old caller is retained: a
// fresh source-only caller could silently inherit the parent's bridge instead
// of detecting the lost declaration and its special-dispatch behavior.
const inheritedMethodFormalBridgeFixture = `
class BridgeResult<T> {final Object token;BridgeResult(Object token){this.token=token;}}
class BridgeNarrow<T> extends BridgeResult<T>{BridgeNarrow(Object token){super(token);}}
class BridgeEffects {static int calls;static boolean fail;static Object actual;static final java.io.IOException error=new java.io.IOException("identity");}
class BridgeBase {public <T extends Number> BridgeResult<T> read(Class<T> type,long n,double d,Object token)throws java.io.IOException {throw new AssertionError("wrong owner");}}
public class BridgeOuter extends BridgeBase {
 static class Leaf extends BridgeOuter {public Leaf(){}}
 public <T extends Number> BridgeNarrow<T> read(Class<T> type,long n,double d,Object token)throws java.io.IOException {
 BridgeEffects.calls++;BridgeEffects.actual=type;if(BridgeEffects.fail)throw BridgeEffects.error;
 if(n!=Long.MIN_VALUE||Double.doubleToRawLongBits(d)!=Double.doubleToRawLongBits(-0.0))throw new AssertionError("wide slots");return new BridgeNarrow<T>(token);}
 static Object leaf(){return new Leaf();}
}

class BridgeDerived extends BridgeOuter.Leaf {
 public <T extends Number> BridgeNarrow<T> read(Class<T> type,long n,double d,Object token){throw new AssertionError("super call became virtual");}
 public BridgeResult special(Class type,long n,double d,Object token)throws Throwable {
 java.lang.invoke.MethodHandle h=java.lang.invoke.MethodHandles.lookup().findSpecial(BridgeOuter.Leaf.class,"read",java.lang.invoke.MethodType.methodType(BridgeResult.class,Class.class,long.class,double.class,Object.class),BridgeDerived.class);
 return (BridgeResult)h.bindTo(this).invokeExact(type,n,d,token);}}
class BridgeDriver {public static void main(String[]args)throws Exception {
 java.lang.reflect.Method[] methods=Class.forName("BridgeOuter$Leaf").getDeclaredMethods();
 if(methods.length!=1)throw new AssertionError("exact original method universe");
 java.lang.reflect.Method bridge=methods[0];
 if(!bridge.isBridge()||!bridge.isSynthetic()||bridge.getReturnType()!=BridgeResult.class||bridge.getTypeParameters().length!=0)throw new AssertionError("original erased bridge");
 Object receiver=BridgeOuter.leaf();
 if(args.length>0&&args[0].equals("special")){receiver=new BridgeDerived();bridge=BridgeDerived.class.getDeclaredMethod("special",Class.class,long.class,double.class,Object.class);}
 bridge.setAccessible(true);int rows=0;
 for(Class type:new Class[]{null,Integer.class,Long.class,String.class})for(Object token:new Object[]{null,new Object(),"token"})for(boolean fail:new boolean[]{false,true}){
 BridgeEffects.calls=0;BridgeEffects.actual=new Object();BridgeEffects.fail=fail;boolean caught=false;
 try{BridgeResult result=(BridgeResult)bridge.invoke(receiver,type,Long.MIN_VALUE,-0.0,token);if(result.token!=token)throw new AssertionError("result identity");}
 catch(java.lang.reflect.InvocationTargetException e){if(e.getCause()!=BridgeEffects.error)throw e;caught=true;}
 if(caught!=fail||BridgeEffects.calls!=1||BridgeEffects.actual!=type)throw new AssertionError("dispatch, input or exception identity");rows++;
 }System.out.println(rows+":method:formal:bridge:identity:special:wide");}}
`

func TestAdversarialInheritedMethodFormalBridgeKeepsErasureAndSpecialDispatch(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original native compiler required")
	}
	for _, root := range []string{"BridgeOuter", "RenamedMethodScope"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(root+"/"+debug, func(t *testing.T) {
				fixture := strings.ReplaceAll(inheritedMethodFormalBridgeFixture, "BridgeOuter", root)
				testIndependentInheritedBridge(t, modern, java, native, fixture, root, debug, true, "24:method:formal:bridge:identity:special:wide\n")
			})
		}
	}
}

func TestInheritedMethodFormalBridgeRequiresOriginalInvariantDeclaration(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original native compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, inheritedMethodFormalBridgeFixture, "BridgeOuter", "none", false)
	for _, variant := range []string{"exact", "free argument", "direct parameter", "direct result", "dependent throws", "wrong descriptor", "trailing signature", "duplicate signature", "wrong class supers", "synchronized target", "virtual dispatch", "wrong wide slot", "missing parent", "work", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			for n, b := range files {
				objects[n] = mustParseLowering(t, bytes.Clone(b))
			}
			obj, parent := objects["BridgeOuter$Leaf.class"], objects["BridgeOuter.class"]
			var bridge, target *MemberInfo
			for _, m := range obj.Methods {
				if m.AccessFlags == 0x1041 {
					bridge = m
				}
			}
			for _, m := range parent.Methods {
				n, _ := sourceBridgeUTF8(parent, m.NameIndex)
				if n == "read" && m.AccessFlags == 1 {
					target = m
				}
			}
			if bridge == nil || target == nil {
				t.Fatal("authored original declarations missing")
			}
			var sig *SignatureAttribute
			for _, a := range target.Attributes {
				if s, ok := a.(*SignatureAttribute); ok {
					sig = s
				}
			}
			if sig == nil {
				t.Fatal("authored method scope missing")
			}
			originalSignature, _ := sourceBridgeUTF8(parent, sig.SignatureIndex)
			replace := func(s string) { sig.SignatureIndex = uint16(parent.ConstantPoolManager.AddUtf8Info(s)) }
			code := constructorLoweringCode(bridge)
			d := core.NewDecompiler(code.Code, nil)
			if d.ParseOpcode() != nil {
				t.Fatal("original bridge code")
			}
			ops := constructorMotionOps(d)
			var work *workbudget.Budget
			switch variant {
			case "free argument":
				replace(strings.Replace(originalSignature, "Ljava/lang/Class<TT;>;", "Ljava/lang/Class<TMissing;>;", 1))
			case "direct parameter":
				// The erasure still equals Class; substitution-freedom must fail.
				replace("<T:Ljava/lang/Class;>(TT;JDLjava/lang/Object;)LBridgeNarrow<TT;>;")
			case "direct result":
				replace("<T:LBridgeNarrow;>(Ljava/lang/Class<TT;>;JDLjava/lang/Object;)TT;")
			case "dependent throws":
				replace("<T:Ljava/io/IOException;>(Ljava/lang/Class<TT;>;JDLjava/lang/Object;)LBridgeNarrow<TT;>;^TT;")
			case "wrong descriptor":
				replace(strings.Replace(originalSignature, "Ljava/lang/Class<TT;>;", "Ljava/util/List<TT;>;", 1))
			case "trailing signature":
				replace(originalSignature + "junk")
			case "duplicate signature":
				target.Attributes = append(target.Attributes, sig)
			case "wrong class supers":
				parent.Attributes = append(parent.Attributes, &SignatureAttribute{AttrLen: 2, SignatureIndex: uint16(parent.ConstantPoolManager.AddUtf8Info("<X:Ljava/lang/Object;>Ljava/lang/Object;"))})
			case "synchronized target":
				target.AccessFlags |= 0x20
			case "virtual dispatch":
				code.Code[int(ops[len(ops)-2].CurrentOffset)] = core.OP_INVOKEVIRTUAL
			case "wrong wide slot":
				code.Code[int(ops[2].CurrentOffset)] = core.OP_LLOAD_1
			case "missing parent":
				delete(objects, "BridgeOuter.class")
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			reader := NewClassObjectDumper(obj)
			reader.Work = work
			reader.foldSiblingResolver = func(n string) ([]byte, bool) {
				o, known := objects[n+".class"]
				if !known {
					return nil, false
				}
				return o.Bytes(), true
			}
			p := reader.inheritedBridgeOriginal(bridge)
			if (p != nil) != (variant == "exact") {
				t.Fatalf("accepted=%v", p != nil)
			}
		})
	}
}
