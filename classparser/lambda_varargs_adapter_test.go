package javaclassparser

import (
	"bytes"
	"os"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

const varargsAdapterShapeFixture = `import java.util.function.*;
class VarargsShapeTarget {
 Object[] objects(Object... values){return values;}
 int ints(int... values){return values.length;}
 long longs(long... values){return values.length;}
 boolean bools(boolean... values){return values.length==1;}
 Object[][] arrays(Object[]... values){return values;}
 void mixed(long a,double b,Object... values){}
}
class VarargsShapeOwner {
 interface Mixed {void apply(long a,double b,Object c);}
 interface Bool {boolean apply(boolean value);}
 static Runnable empty(VarargsShapeTarget x){return x::objects;}
 static Consumer<Object> one(VarargsShapeTarget x){return x::objects;}
 static BiConsumer<Object,Object> two(VarargsShapeTarget x){return x::objects;}
 static Function<Object,Object[]> retained(VarargsShapeTarget x){return x::objects;}
 static IntUnaryOperator ints(VarargsShapeTarget x){return x::ints;}
 static LongUnaryOperator longs(VarargsShapeTarget x){return x::longs;}
 static Bool bools(VarargsShapeTarget x){return x::bools;}
 static BiFunction<Object[],Object[],Object[][]> arrays(VarargsShapeTarget x){return x::arrays;}
 static Mixed mixed(VarargsShapeTarget x){return x::mixed;}
}`

func TestAdversarialVarargsAdapterShapeRequiresOriginalOrderedTypedPack(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
			}
			files := compileLambdaParameterMetadataFixture(t, compiler, profile, varargsAdapterShapeFixture, "VarargsShapeOwner", "none", true)
			obj, err := Parse(bytes.Clone(files["VarargsShapeOwner.class"]))
			if err != nil {
				t.Fatal(err)
			}
			positive := 0
			for _, method := range obj.Methods {
				if method.AccessFlags != 0x100a {
					continue
				}
				name, _ := sourceBridgeUTF8(obj, method.NameIndex)
				t.Run(name, func(t *testing.T) {
					d := NewClassObjectDumper(obj)
					packet, known := d.originalVarargsAdapterShape(method)
					if !known || packet == nil || packet.object != obj || packet.method != method || !bytes.Equal(packet.codeBytes, packet.code.Code) {
						t.Fatalf("original compiler's typed adapter not recognized: %s", name)
					}
					positive++
				})
			}
			if positive != 9 {
				t.Fatalf("original adapter inventory %d want 9", positive)
			}
			for _, variant := range []string{"detached declaration", "ordinary declaration", "wrong receiver port", "wrong pack length", "wrong store index", "wrong element port", "wrong store opcode", "missing result discard", "wrong return", "unproved stack limit", "unproved local limit", "protected adapter", "duplicate Code", "extra executable tail"} {
				t.Run(variant, func(t *testing.T) {
					object, err := Parse(bytes.Clone(files["VarargsShapeOwner.class"]))
					if err != nil {
						t.Fatal(err)
					}
					var implementation *MemberInfo
					var code *CodeAttribute
					for _, method := range object.Methods {
						desc, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
						if method.AccessFlags == 0x100a && desc == "(LVarargsShapeTarget;Ljava/lang/Object;Ljava/lang/Object;)V" {
							implementation = method
							for _, attribute := range method.Attributes {
								if original, ok := attribute.(*CodeAttribute); ok {
									code = original
								}
							}
						}
					}
					if code == nil {
						t.Fatal("missing original two-element adapter")
					}
					switch variant {
					case "detached declaration":
						copy := *implementation
						implementation = &copy
					case "ordinary declaration":
						implementation.AccessFlags &^= 0x1000
					case "wrong receiver port":
						code.Code[0] = core.OP_ALOAD_1
					case "wrong pack length":
						code.Code[1] = core.OP_ICONST_1
					case "wrong store index":
						code.Code[6] = core.OP_ICONST_1
					case "wrong element port":
						code.Code[7] = core.OP_ALOAD_2
					case "wrong store opcode":
						code.Code[8] = core.OP_IASTORE
					case "missing result discard":
						code.Code = append(code.Code[:len(code.Code)-2], core.OP_RETURN)
					case "wrong return":
						code.Code[len(code.Code)-1] = core.OP_ARETURN
					case "unproved stack limit":
						code.MaxStack = 0
					case "unproved local limit":
						code.MaxLocals = 0
					case "protected adapter":
						code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: uint16(len(code.Code) - 1), CatchType: 0}}
					case "duplicate Code":
						implementation.Attributes = append(implementation.Attributes, code)
					case "extra executable tail":
						code.Code = append(code.Code, core.OP_ACONST_NULL, core.OP_ARETURN)
					}
					before := bytes.Clone(code.Code)
					d := NewClassObjectDumper(object)
					if _, known := d.originalVarargsAdapterShape(implementation); known {
						t.Fatal("changed original adapter borrowed a transfer proof")
					}
					if !bytes.Equal(before, code.Code) {
						t.Fatal("shape proof changed original Code")
					}
				})
			}
		})
	}
}
