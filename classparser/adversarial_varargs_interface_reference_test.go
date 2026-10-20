package javaclassparser

import (
	"bytes"
	"os"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

const varargsInterfaceReferenceFixture = `import java.util.function.Consumer;
interface VarargsInterfaceTarget {Object[] apply(Object...values);void wide(long a,double b,Object...values);}
class VarargsInterfaceImplementation implements VarargsInterfaceTarget {
 static Object[] last;static VarargsInterfaceTarget receiver;static int calls;static long wideLong,wideDouble;
 public Object[] apply(Object...values){if(last==values)throw new AssertionError("fresh interface pack");last=values;receiver=this;calls++;return values;}
 public void wide(long a,double b,Object...values){apply(values);wideLong=a;wideDouble=Double.doubleToRawLongBits(b);}
}
public class VarargsInterfaceOwner {
 interface Wide{void accept(long a,double b,Object value);}
 static Consumer<Object> make(VarargsInterfaceTarget target){return target::apply;}
 static Wide wide(VarargsInterfaceTarget target){return target::wide;}
}
class VarargsInterfaceDriver {
 public static void main(String[]args){Object shared=new Object();int rows=0;
  for(VarargsInterfaceTarget target:new VarargsInterfaceTarget[]{null,new VarargsInterfaceImplementation(),new VarargsInterfaceImplementation()})for(Object value:new Object[]{null,shared})for(int route=0;route<2;route++){
   boolean made=false;Throwable caught=null;int before=VarargsInterfaceImplementation.calls;
   try{if(route==0){Consumer<Object>action=VarargsInterfaceOwner.make(target);made=true;action.accept(value);}else{VarargsInterfaceOwner.Wide action=VarargsInterfaceOwner.wide(target);made=true;action.accept(Long.MIN_VALUE,-0.0d,value);}}catch(Throwable error){caught=error;}
   if(target==null){if(made||!(caught instanceof NullPointerException)||VarargsInterfaceImplementation.calls!=before)throw new AssertionError("interface eager check",caught);}
   else{if(caught!=null||!made||VarargsInterfaceImplementation.calls!=before+1||VarargsInterfaceImplementation.receiver!=target||VarargsInterfaceImplementation.last.length!=1||VarargsInterfaceImplementation.last[0]!=value)throw new AssertionError("interface typed transfer",caught);if(route==1&&(VarargsInterfaceImplementation.wideLong!=Long.MIN_VALUE||VarargsInterfaceImplementation.wideDouble!=0x8000000000000000L))throw new AssertionError("wide interface ports");}rows++;
  }
  System.out.println(rows+":interface:varargs:wide:identity");
 }
}`

func TestAdversarialVarargsInterfaceReferencePreservesDispatchAndWidePorts(t *testing.T) {
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
			for _, named := range []bool{false, true} {
				label := "absent"
				if named {
					label = "named"
				}
				t.Run(label, func(t *testing.T) {
					testNativeIndependentCompilerFamilyWithParameterFlag(t, func(debug string) map[string][]byte {
						return compileLambdaParameterMetadataFixture(t, compiler, profile, varargsInterfaceReferenceFixture, "VarargsInterfaceOwner", debug, named)
					}, profile, compiler, []string{"VarargsInterfaceOwner"}, "VarargsInterfaceDriver", "12:interface:varargs:wide:identity\n", nil, func(t *testing.T, files map[string][]byte) *JarFS { return nativeEnumOwnUnitArchive(t, files, profile) }, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, named, nativeLexicalExactSignatures, func(t *testing.T, name string, a, b []byte) {
						if x, y := lambdaParameterMetadataTables(t, a), lambdaParameterMetadataTables(t, b); x != y {
							t.Fatalf("original interface reference parameter roles changed %s\n%s\n%s", name, x, y)
						}
					})
				})
			}
		})
	}
}

// The redundant JVM operand count includes category-2 words. An invalid count
// cannot acquire a source proof merely because the method descriptor is valid.
// Mutated Code is never run on the JVM.
func TestAdversarialVarargsInterfaceAdapterRejectsWrongPhysicalInvokeCount(t *testing.T) {
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
			files := compileLambdaParameterMetadataFixture(t, compiler, profile, varargsInterfaceReferenceFixture, "VarargsInterfaceOwner", "none", true)
			for _, wanted := range []string{"apply", "wide"} {
				t.Run(wanted, func(t *testing.T) {
					for _, variant := range []string{"original", "wrong interface count", "reserved interface byte"} {
						t.Run(variant, func(t *testing.T) {
							object, err := Parse(bytes.Clone(files["VarargsInterfaceOwner.class"]))
							if err != nil {
								t.Fatal(err)
							}
							d := NewClassObjectDumper(object)
							var method *MemberInfo
							var code *CodeAttribute
							invoke := -1
							for _, m := range object.Methods {
								shape, known := d.originalVarargsAdapterShape(m)
								if known && shape.member.Member == wanted {
									method, code = m, shape.code
									decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
									if decoder.ParseOpcode() != nil {
										t.Fatal("original interface code")
									}
									for _, op := range constructorMotionOps(decoder) {
										if op.Instr.OpCode == core.OP_INVOKEINTERFACE {
											invoke = int(op.CurrentOffset)
										}
									}
								}
							}
							if method == nil || code == nil || invoke < 0 {
								t.Fatal("original interface adapter")
							}
							if variant == "wrong interface count" {
								code.Code[invoke+3] = 1
							}
							if variant == "reserved interface byte" {
								code.Code[invoke+4] = 1
							}
							before := bytes.Clone(code.Code)
							_, known := d.originalVarargsAdapterShape(method)
							if known != (variant == "original") {
								t.Fatalf("%s count proof=%v", variant, known)
							}
							if !bytes.Equal(before, code.Code) {
								t.Fatal("shape proof changed original Code")
							}
						})
					}
				})
			}
		})
	}
}
