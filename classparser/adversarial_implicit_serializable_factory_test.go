package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// A Serializable ancestor does not turn the normal metafactory protocol into
// FLAG_SERIALIZABLE. Java source would implicitly add writeReplace and a new
// capturing-class helper route, changing the original executable contract.
// The classfile is valid and the original JVM actually invokes its factory.
func TestAdversarialInheritedSerializableTargetsCannotInventSerializationProtocol(t *testing.T) {
	for _, factoryKind := range []string{"normal", "alternate", "serializable marker"} {
		t.Run(factoryKind, func(t *testing.T) {
			for _, indirect := range []bool{false, true} {
				for _, mixed := range []bool{false, true} {
					t.Run(fmt.Sprintf("indirect=%t/mixed=%t", indirect, mixed), func(t *testing.T) {
						port := `interface InheritedPort extends java.util.function.Function<String,String>,java.io.Serializable{}`
						if indirect {
							port = `interface GenericPort<T> extends java.util.function.Function<T,String>,java.io.Serializable{} interface InheritedPort extends GenericPort<String>{}`
						}
						explicit, check := "", ""
						if mixed {
							explicit = `static java.util.function.Function<String,String> explicit(){return (java.util.function.Function<String,String>&java.io.Serializable)ExternalLinks::Aa;}`
							check = `Object good=LinkedOwner.explicit();java.lang.reflect.Method replacement=good.getClass().getDeclaredMethod("writeReplace");if(replacement==null)throw new AssertionError("explicit serialization control");`
						}
						returnType, returnDescriptor := "InheritedPort", "LInheritedPort;"
						if factoryKind == "serializable marker" {
							returnType, returnDescriptor = "java.util.function.Function<String,String>", "Ljava/util/function/Function;"
						}
						source := port + `class ExternalLinks{static String Aa(String value){return value+":A";}static String BB(String value){return value+":B";}}
class LinkedOwner{` + explicit + `static ` + returnType + ` legacy(){return null;}}
class LinkedDriver{public static void main(String[]args)throws Exception{java.util.function.Function<String,String> value=LinkedOwner.legacy();if(!"x:B".equals(value.apply("x"))||!(value instanceof java.io.Serializable))throw new AssertionError("valid original inherited factory");try{value.getClass().getDeclaredMethod("writeReplace");throw new AssertionError("invented serialization replacement");}catch(NoSuchMethodException expected){}` + check + `System.out.println("original:inherited-serializable:without-flag");}}`
						files := nativeCompileDebugClasses(t, source, "none")
						object, e := Parse(bytes.Clone(files["LinkedOwner.class"]))
						if e != nil {
							t.Fatal(e)
						}
						pool := NewConstantPoolWithConstant(&object.ConstantPool)
						pool.AddUtf8Info("BootstrapMethods")
						var bootstrap *BootstrapMethodsAttribute
						for _, a := range object.Attributes {
							if b, ok := a.(*BootstrapMethodsAttribute); ok {
								if bootstrap != nil {
									t.Fatal("duplicate original bootstraps")
								}
								bootstrap = b
							}
						}
						if bootstrap == nil {
							bootstrap = &BootstrapMethodsAttribute{}
							object.Attributes = append(object.Attributes, bootstrap)
						}
						bootstrapName, bootstrapDescriptor := "metafactory", "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodHandle;Ljava/lang/invoke/MethodType;)Ljava/lang/invoke/CallSite;"
						if factoryKind != "normal" {
							bootstrapName, bootstrapDescriptor = "altMetafactory", "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;[Ljava/lang/Object;)Ljava/lang/invoke/CallSite;"
						}
						method := uint16(pool.AddNewMethodInfo("java/lang/invoke/LambdaMetafactory", bootstrapName, bootstrapDescriptor))
						handle := uint16(pool.AppendConstantInfo(&ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: method}))
						target := uint16(pool.AddNewMethodInfo("ExternalLinks", "BB", "(Ljava/lang/String;)Ljava/lang/String;"))
						targetHandle := uint16(pool.AppendConstantInfo(&ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: target}))
						sam := uint16(pool.AppendConstantInfo(&ConstantMethodTypeInfo{DescriptorIndex: uint16(pool.AddUtf8Info("(Ljava/lang/Object;)Ljava/lang/Object;"))}))
						instantiated := uint16(pool.AppendConstantInfo(&ConstantMethodTypeInfo{DescriptorIndex: uint16(pool.AddUtf8Info("(Ljava/lang/String;)Ljava/lang/String;"))}))
						ordinal := uint16(len(bootstrap.BootstrapMethods))
						arguments := []uint16{sam, targetHandle, instantiated}
						if factoryKind == "alternate" {
							arguments = append(arguments, uint16(pool.AppendConstantInfo(&ConstantIntegerInfo{Value: 0})))
						}
						if factoryKind == "serializable marker" {
							arguments = append(arguments, uint16(pool.AppendConstantInfo(&ConstantIntegerInfo{Value: 2})), uint16(pool.AppendConstantInfo(&ConstantIntegerInfo{Value: 1})), uint16(pool.AppendConstantInfo(&ConstantClassInfo{NameIndex: uint16(pool.AddUtf8Info("InheritedPort"))})))
						}
						bootstrap.BootstrapMethods = append(bootstrap.BootstrapMethods, &BootstrapMethod{BootstrapMethodRef: handle, NumBootstrapArguments: uint16(len(arguments)), BootstrapArguments: arguments})
						bootstrap.NumBootstrapMethods = uint16(len(bootstrap.BootstrapMethods))
						bootstrap.AttributeLength = 2
						for _, b := range bootstrap.BootstrapMethods {
							bootstrap.AttributeLength += 4 + 2*uint32(len(b.BootstrapArguments))
						}
						nat := uint16(pool.AppendConstantInfo(&ConstantNameAndTypeInfo{NameIndex: uint16(pool.AddUtf8Info("apply")), DescriptorIndex: uint16(pool.AddUtf8Info("()" + returnDescriptor))}))
						site := uint16(pool.AppendConstantInfo(&ConstantInvokeDynamicInfo{BootstrapMethodAttrIndex: ordinal, NameAndTypeIndex: nat}))
						patched := false
						for _, m := range object.Methods {
							name, _ := sourceBridgeUTF8(object, m.NameIndex)
							if name != "legacy" {
								continue
							}
							for i, a := range m.Attributes {
								if _, ok := a.(*CodeAttribute); ok {
									code := []byte{core.OP_INVOKEDYNAMIC, byte(site >> 8), byte(site), 0, 0, core.OP_ARETURN}
									m.Attributes[i] = &CodeAttribute{AttrLen: uint32(12 + len(code)), MaxStack: 1, MaxLocals: 0, Code: code}
									patched = true
								}
							}
						}
						if !patched {
							t.Fatal("original placeholder method missing")
						}
						files["LinkedOwner.class"] = object.Bytes()
						javac, java := t04Tools(t)
						original := adversarialAssertionClasses(t, files)
						if got := t04RunJava(t, java, original, "LinkedDriver"); got != "original:inherited-serializable:without-flag\n" {
							t.Fatal("original factory/protocol oracle", got)
						}
						z := nativeArchive(t, files)
						defer z.Close()
						body, e := z.ReadFile("LinkedOwner.class")
						published := e == nil && len(body) != 0 && !strings.Contains(string(body), DecompileStubMarker) && !strings.Contains(string(body), "decompile dump failed")
						if !published {
							return
						}
						// Demonstrate the changed behavior with the unchanged original caller,
						// without an original owned class on the compilation classpath.
						out := t.TempDir()
						for name, data := range files {
							if name == "LinkedOwner.class" {
								continue
							}
							if e := os.WriteFile(filepath.Join(out, name), data, 0600); e != nil {
								t.Fatal(e)
							}
						}
						path := filepath.Join(out, "LinkedOwner.java")
						if e := os.WriteFile(path, body, 0600); e != nil {
							t.Fatal(e)
						}
						if b, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", out, "-d", out, path).CombinedOutput(); e != nil {
							t.Fatalf("published source already broke reconstruction: %v\n%s\n%s", e, b, body)
						}
						observed, runErr := exec.Command(java, "-Xverify:all", "-cp", out, "LinkedDriver").CombinedOutput()
						t.Fatalf("source invented a serialization protocol: rebuilt err=%v output=%s\n%s", runErr, observed, body)
					})
				}
			}
		})
	}
}
