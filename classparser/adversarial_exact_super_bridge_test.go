package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
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
	for _, shape := range []string{"reference", "array", "boolean", "int", "long", "float", "double", "void"} {
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
						}, func(t *testing.T, owner string, original, fresh []byte, artifact *JarReconstructionArtifact) {
							if root == "BridgeOuter" && debug == "none" && major == 49 {
								checkExactSuperBridgeBoundaries(t, owner, original, fresh, artifact)
							}
						})
					})
				}
			}
		}
	}
}

func exactSuperBridgeFixture(shape string) string {
	result, body, forwarding, special, check := "Object", "return token;", "return super.read(n,d,token);", "return (Object)h.bindTo(this).invokeExact(n,d,v);", "result!=v"
	if shape == "array" {
		result, body = "Object[]", "return new Object[]{token};"
		special, check = "return (Object[])h.bindTo(this).invokeExact(n,d,v);", "!(result instanceof Object[])||((Object[])result).length!=1||((Object[])result)[0]!=v"
	} else if shape == "boolean" {
		result, body = "boolean", "return n<0;"
		special, check = "return Boolean.valueOf((boolean)h.bindTo(this).invokeExact(n,d,v));", "!(result instanceof Boolean)||((Boolean)result).booleanValue()!=(n<0)"
	} else if shape == "int" {
		result, body = "int", "return (int)n;"
		special, check = "return Integer.valueOf((int)h.bindTo(this).invokeExact(n,d,v));", "!(result instanceof Integer)||((Integer)result).intValue()!=(int)n"
	} else if shape == "float" {
		result, body = "float", "return (float)d;"
		special, check = "return Float.valueOf((float)h.bindTo(this).invokeExact(n,d,v));", "!(result instanceof Float)||Float.floatToRawIntBits(((Float)result).floatValue())!=Float.floatToRawIntBits((float)d)"
	} else if shape == "double" {
		result, body = "double", "return d;"
		special, check = "return Double.valueOf((double)h.bindTo(this).invokeExact(n,d,v));", "!(result instanceof Double)||Double.doubleToRawLongBits(((Double)result).doubleValue())!=Double.doubleToRawLongBits(d)"
	} else if shape == "long" {
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

func checkExactSuperBridgeBoundaries(t *testing.T, owner string, original, fresh []byte, artifact *JarReconstructionArtifact) {
	t.Helper()
	base := artifact.source.jar.sourceConstructorLowering
	for _, variant := range []string{"control", "unreturned", "pending", "stale original", "receiver", "wide slot", "virtual dispatch", "effect", "handler", "return category", "flags", "extra method", "duplicate method", "unknown attribute", "changed target"} {
		t.Run("proof/"+variant, func(t *testing.T) {
			want := bytes.Clone(original)
			object := mustParseLowering(t, bytes.Clone(fresh))
			authority := base
			var method *MemberInfo
			for _, m := range object.Methods {
				if n, _ := sourceBridgeUTF8(object, m.NameIndex); n == "read" {
					method = m
				}
			}
			if method == nil || method.AccessFlags != 1 {
				t.Fatal("fresh ordinary source forwarding declaration")
			}
			body := constructorLoweringCode(method)
			decoder := core.NewDecompiler(body.Code, nil)
			if decoder.ParseOpcode() != nil {
				t.Fatal("fresh typed forwarding instructions")
			}
			ops := constructorMotionOps(decoder)
			call := ops[len(ops)-2]
			switch variant {
			case "unreturned":
				authority = &sourceConstructorLoweringAuthority{jar: base.jar}
			case "pending":
				authority = &sourceConstructorLoweringAuthority{jar: base.jar, parent: base, bridgePending: map[string]constructorNativeSourceOrigin{owner: {}}}
			case "stale original":
				before := mustParseLowering(t, want)
				before.ConstantPoolManager.AddUtf8Info("different original")
				want = before.Bytes()
			case "receiver":
				body.Code[0] = core.OP_ALOAD_1
			case "wide slot":
				body.Code[int(ops[1].CurrentOffset)] = core.OP_LLOAD_2
			case "virtual dispatch":
				body.Code[int(call.CurrentOffset)] = core.OP_INVOKEVIRTUAL
			case "effect":
				body.Code = append([]byte{core.OP_ACONST_NULL, core.OP_POP}, body.Code...)
			case "handler":
				body.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: 0}}
			case "return category":
				body.Code[len(body.Code)-1] = core.OP_ATHROW
			case "flags":
				method.AccessFlags = 0x0011
			case "extra method":
				copy := *method
				copy.NameIndex = uint16(object.ConstantPoolManager.AddUtf8Info("unrelatedDeclaration"))
				object.Methods = append(object.Methods, &copy)
			case "duplicate method":
				object.Methods = append(object.Methods, method)
			case "unknown attribute":
				object.ConstantPoolManager.AddUtf8Info("UnknownBridgeProtocol")
				method.Attributes = append(method.Attributes, &UnparsedAttribute{Name: "UnknownBridgeProtocol"})
			case "changed target":
				index, ok := nativeEnumCPIndex(call)
				if !ok {
					t.Fatal("fresh call index")
				}
				reference := nativeConstantMember(object.ConstantPool[index-1])
				nt := object.ConstantPool[reference.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				nt.NameIndex = uint16(object.ConstantPoolManager.AddUtf8Info("anotherTarget"))
			}
			if len(body.Attributes) != 0 {
				t.Fatal("debug-free fresh code required for structural mutants")
			}
			body.AttrLen = uint32(12 + len(body.Code) + 8*len(body.ExceptionTable))
			input := object.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("negative must reach the forwarding proof", err)
			}
			saved := bytes.Clone(input)
			result, err := authority.lowerInheritedBridges(owner, want, input, nil)
			if !bytes.Equal(saved, input) {
				t.Fatal("caller input mutated")
			}
			if variant == "control" {
				if err != nil || bytes.Equal(result, input) || len(result) != len(input) {
					t.Fatal("exact forwarding bridge not completed", err)
				}
				reader, err := NewClassReaderWithBudget(input, nil)
				if err != nil {
					t.Fatal(err)
				}
				layout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
				parsed, err := parseWithMetadataLayout(reader, nil, layout)
				if err != nil {
					t.Fatal(err)
				}
				for _, m := range parsed.Methods {
					if n, _ := sourceBridgeUTF8(parsed, m.NameIndex); n == "read" {
						span := layout.methods[m]
						copy := bytes.Clone(result)
						copy[span.start], copy[span.start+1] = input[span.start], input[span.start+1]
						if !bytes.Equal(copy, input) {
							t.Fatal("bridge completion changed fresh bytes beyond its two flag bytes")
						}
					}
				}
				if again, err := authority.lowerInheritedBridges(owner, want, result, nil); err != nil || !bytes.Equal(again, result) {
					t.Fatal("bridge completion is not idempotent", err)
				}
			} else if err == nil && !bytes.Equal(result, input) || err != nil && result != nil {
				t.Fatal("unproved or partial bridge completion", err)
			}
		})
	}
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
