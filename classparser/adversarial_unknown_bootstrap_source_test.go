package javaclassparser

import (
	"bytes"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const unsupportedBootstrapSourceFixture = `class BootstrapEffects{static int links,hits;static String trace="";public static java.lang.invoke.CallSite bootstrap(java.lang.invoke.MethodHandles.Lookup lookup,String name,java.lang.invoke.MethodType type)throws Throwable{links++;trace+="B"+name+":";return new java.lang.invoke.ConstantCallSite(java.lang.invoke.MethodHandles.lookup().findStatic(BootstrapEffects.class,name,type));}public static String reference(){hits++;trace+="R";return "result:"+hits;}public static int integer(int value){hits++;trace+="I";return value*31+7;}public static long wide(long value){hits++;trace+="W";return value*37L-11L;}public static void effect(){hits++;trace+="V";}}
class DynamicSubject{static String reference(){return "authored";}static int integer(int value){return value;}static long wide(long value){return value;}static void effect(){}}
class DynamicDriver{public static void main(String[]args){if(!"result:1".equals(DynamicSubject.reference())||!"result:2".equals(DynamicSubject.reference()))throw new AssertionError("reference bootstrap result");for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){int expected=java.math.BigInteger.valueOf(value).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(7)).intValue();if(DynamicSubject.integer(value)!=expected)throw new AssertionError("integer bootstrap arithmetic");}for(long value:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){long expected=java.math.BigInteger.valueOf(value).multiply(java.math.BigInteger.valueOf(37)).subtract(java.math.BigInteger.valueOf(11)).longValue();if(DynamicSubject.wide(value)!=expected)throw new AssertionError("wide bootstrap arithmetic");}DynamicSubject.effect();DynamicSubject.effect();if(BootstrapEffects.links!=4||BootstrapEffects.hits!=14||!BootstrapEffects.trace.equals("Breference:RRBinteger:IIIIIBwide:WWWWWBeffect:VV"))throw new AssertionError("linkage/call effects or order:"+BootstrapEffects.trace);System.out.println("4:14:bootstrap:effects:typed");}}
`

func installUnknownBootstrapSourceCalls(t *testing.T, files map[string][]byte, subject, owner string) map[string][]byte {
	t.Helper()
	object, e := Parse(bytes.Clone(files[subject+".class"]))
	if e != nil {
		t.Fatal(e)
	}
	pool := NewConstantPoolWithConstant(&object.ConstantPool)
	pool.AddUtf8Info("BootstrapMethods")
	for _, a := range object.Attributes {
		if _, ok := a.(*BootstrapMethodsAttribute); ok {
			t.Fatal("authored subject already has bootstrap methods")
		}
	}
	ref := uint16(pool.AddNewMethodInfo(owner, "bootstrap", "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;)Ljava/lang/invoke/CallSite;"))
	handle := uint16(pool.AppendConstantInfo(&ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: ref}))
	object.Attributes = append(object.Attributes, &BootstrapMethodsAttribute{AttributeLength: 6, NumBootstrapMethods: 1, BootstrapMethods: []*BootstrapMethod{{BootstrapMethodRef: handle}}})
	patched := 0
	for _, m := range object.Methods {
		name, _ := sourceBridgeUTF8(object, m.NameIndex)
		descriptor, _ := sourceBridgeUTF8(object, m.DescriptorIndex)
		var before []byte
		var ret byte
		stack, locals := uint16(1), uint16(0)
		switch name {
		case "reference":
			ret = core.OP_ARETURN
		case "integer":
			before = []byte{core.OP_ILOAD_0}
			ret = core.OP_IRETURN
			locals = 1
		case "wide":
			before = []byte{core.OP_LLOAD_0}
			ret = core.OP_LRETURN
			stack, locals = 2, 2
		case "effect":
			ret = core.OP_RETURN
			stack = 0
		default:
			continue
		}
		nat := uint16(pool.AppendConstantInfo(&ConstantNameAndTypeInfo{NameIndex: uint16(pool.AddUtf8Info(name)), DescriptorIndex: uint16(pool.AddUtf8Info(descriptor))}))
		dynamic := uint16(pool.AppendConstantInfo(&ConstantInvokeDynamicInfo{BootstrapMethodAttrIndex: 0, NameAndTypeIndex: nat}))
		code := append(before, core.OP_INVOKEDYNAMIC, byte(dynamic>>8), byte(dynamic), 0, 0, ret)
		for i, a := range m.Attributes {
			if _, ok := a.(*CodeAttribute); ok {
				m.Attributes[i] = &CodeAttribute{AttrLen: uint32(12 + len(code)), MaxStack: stack, MaxLocals: locals, Code: code}
				patched++
			}
		}
	}
	if patched != 4 {
		t.Fatal("patched callsite count", patched)
	}
	files[subject+".class"] = object.Bytes()
	return files
}
func TestAdversarialUnknownBootstrapSourceCannotPublishValuesOrLoseVoidEffects(t *testing.T) {
	_, java := t04Tools(t)
	for _, rename := range []bool{false, true} {
		source, subject, owner, driver := unsupportedBootstrapSourceFixture, "DynamicSubject", "BootstrapEffects", "DynamicDriver"
		label := "original"
		if rename {
			label = "renamed"
			subject, owner, driver = "OpaqueSubject", "RuntimeFactory", "SeparateCallDriver"
			source = strings.NewReplacer("DynamicSubject", subject, "BootstrapEffects", owner, "DynamicDriver", driver).Replace(source)
		}
		t.Run(label, func(t *testing.T) {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := installUnknownBootstrapSourceCalls(t, nativeCompileDebugClasses(t, source, debug), subject, owner)
					original := adversarialAssertionClasses(t, files)
					if got := t04RunJava(t, java, original, driver); got != "4:14:bootstrap:effects:typed\n" {
						t.Fatal("valid original linkage/arithmetic/effect oracle", got)
					}
					before := bytes.Clone(files[subject+".class"])
					for _, mode := range []DecompileMode{Precision, Compatibility} {
						t.Run(string(mode), func(t *testing.T) {
							result, e := DecompileWithOptions(bytes.Clone(before), DecompileOptions{Mode: mode, SourceCompiler: ModernJavac, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return bytes.Clone(b), ok }})
							if e != nil {
								t.Fatal(e)
							}
							want := map[string]bool{"reference()Ljava/lang/String;": false, "integer(I)I": false, "wide(J)J": false, "effect()V": false}
							for _, m := range result.StubMethods {
								if _, ok := want[m]; ok {
									want[m] = true
								}
							}
							unknown := map[string]bool{}
							for _, d := range result.Diagnostics {
								if d.Code == core.DiagBootstrapUnknown {
									unknown[d.Method] = true
								}
								if d.Code == core.DiagBootstrapInvalid || d.Code == core.DiagBootstrapArgMismatch {
									t.Fatal("valid custom linkage is not malformed input", d)
								}
							}
							for m, stub := range want {
								if !stub || !unknown[m] {
									t.Errorf("unavailable source %s: stub=%v unknown=%v", m, stub, unknown[m])
								}
							}
							if result.Status != "partial" || strings.Contains(result.Source, "unsupported bootstrap") || !bytes.Equal(before, files[subject+".class"]) {
								t.Fatalf("unsupported value must not become a successful expression: status=%s\n%s", result.Status, result.Source)
							}
						})
					}
					z := nativeArchive(t, files)
					defer z.Close()
					src, e := z.ReadFile(subject + ".class")
					if e != nil || strings.Contains(string(src), "unsupported bootstrap") || strings.Count(string(src), "/* "+DecompileStubMarker) != 4 {
						t.Fatalf("archive body boundary: %v\n%s", e, src)
					}
					for n, b := range files {
						after, e := os.ReadFile(filepath.Join(original, n))
						if e != nil || !bytes.Equal(b, after) {
							t.Fatal("original byte origin changed", n, e)
						}
					}
				})
			}
		})
	}
}
