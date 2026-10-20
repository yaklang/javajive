package javaclassparser

import (
	"bytes"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"strings"
	"testing"
)

const unsupportedCondySourceFixture = `class ConstantFactory{static int links;public static Object bootstrap(java.lang.invoke.MethodHandles.Lookup lookup,String name,Class<?> type){links++;if(name.equals("nullValue"))return null;if(type==float.class)return Float.valueOf(-0.0f);if(type==double.class)return Double.valueOf(-0.0d);if(type==String.class)return new String("payload");if(type==int.class)return Integer.valueOf(37);if(type==long.class)return Long.valueOf(Long.MIN_VALUE+17);throw new IllegalArgumentException(type.getName());}}
class ConstantSubject{static String reference(){return "authored";}static int integer(){return 0;}static long wide(){return 0L;}static float single(){return 0f;}static double decimal(){return 0d;}static String nullValue(){return "unused";}}
class ConstantDriver{public static void main(String[]args){String a=ConstantSubject.reference(),b=ConstantSubject.reference();if(!"payload".equals(a)||a!=b||ConstantSubject.integer()!=37||ConstantSubject.integer()!=37||ConstantSubject.wide()!=Long.MIN_VALUE+17||ConstantSubject.wide()!=Long.MIN_VALUE+17||Float.floatToRawIntBits(ConstantSubject.single())!=Integer.MIN_VALUE||Float.floatToRawIntBits(ConstantSubject.single())!=Integer.MIN_VALUE||Double.doubleToRawLongBits(ConstantSubject.decimal())!=Long.MIN_VALUE||Double.doubleToRawLongBits(ConstantSubject.decimal())!=Long.MIN_VALUE||ConstantSubject.nullValue()!=null||ConstantSubject.nullValue()!=null||ConstantFactory.links!=6)throw new AssertionError("typed constant resolution/cache/origin");System.out.println("6:condy:cached:typed");}}
`

func installUnsupportedSourceConstants(t *testing.T, files map[string][]byte) map[string][]byte {
	t.Helper()
	object, e := Parse(bytes.Clone(files["ConstantSubject.class"]))
	if e != nil {
		t.Fatal(e)
	}
	pool := NewConstantPoolWithConstant(&object.ConstantPool)
	pool.AddUtf8Info("BootstrapMethods")
	ref := uint16(pool.AddNewMethodInfo("ConstantFactory", "bootstrap", "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/Class;)Ljava/lang/Object;"))
	handle := uint16(pool.AppendConstantInfo(&ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: ref}))
	object.Attributes = append(object.Attributes, &BootstrapMethodsAttribute{AttributeLength: 6, NumBootstrapMethods: 1, BootstrapMethods: []*BootstrapMethod{{BootstrapMethodRef: handle}}})
	patched := 0
	for _, m := range object.Methods {
		name, _ := sourceBridgeUTF8(object, m.NameIndex)
		descriptor, ret, load, stack := "", byte(0), byte(core.OP_LDC_W), uint16(1)
		switch name {
		case "reference":
			descriptor, ret = "Ljava/lang/String;", core.OP_ARETURN
		case "integer":
			descriptor, ret = "I", core.OP_IRETURN
		case "wide":
			descriptor, ret, load, stack = "J", core.OP_LRETURN, core.OP_LDC2_W, 2
		case "single":
			descriptor, ret = "F", core.OP_FRETURN
		case "decimal":
			descriptor, ret, load, stack = "D", core.OP_DRETURN, core.OP_LDC2_W, 2
		case "nullValue":
			descriptor, ret = "Ljava/lang/String;", core.OP_ARETURN
		default:
			continue
		}
		nat := uint16(pool.AppendConstantInfo(&ConstantNameAndTypeInfo{NameIndex: uint16(pool.AddUtf8Info(name)), DescriptorIndex: uint16(pool.AddUtf8Info(descriptor))}))
		index := uint16(pool.AppendConstantInfo(&ConstantDynamicInfo{BootstrapMethodAttrIndex: 0, NameAndTypeIndex: nat}))
		for i, a := range m.Attributes {
			if _, ok := a.(*CodeAttribute); ok {
				code := []byte{load, byte(index >> 8), byte(index), ret}
				if name == "reference" {
					if index > 255 {
						t.Fatal("narrow constant index not representable", index)
					}
					code = []byte{core.OP_LDC, byte(index), ret}
				}
				m.Attributes[i] = &CodeAttribute{AttrLen: uint32(12 + len(code)), MaxStack: stack, MaxLocals: 0, Code: code}
				patched++
			}
		}
	}
	if patched != 6 || object.MajorVersion != 55 {
		t.Fatal("original constant shape", patched, object.MajorVersion)
	}
	files["ConstantSubject.class"] = object.Bytes()
	return files
}
func TestAdversarialUnsupportedConstantsCannotPublishValuesOrSkipResolution(t *testing.T) {
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := installUnsupportedSourceConstants(t, nativeCompileSourceReleaseClasses(t, map[string]string{"ConstantSubject.java": unsupportedCondySourceFixture}, debug, "11"))
			original := adversarialAssertionClasses(t, files)
			if got := t04RunJava(t, java, original, "ConstantDriver"); got != "6:condy:cached:typed\n" {
				t.Fatal("valid original constant linkage/cache oracle", got)
			}
			before := bytes.Clone(files["ConstantSubject.class"])
			for _, mode := range []DecompileMode{Precision, Compatibility} {
				t.Run(string(mode), func(t *testing.T) {
					result, e := DecompileWithOptions(bytes.Clone(before), DecompileOptions{Mode: mode, SourceCompiler: ModernJavac, TargetSourceVersion: 11, EnvSnapshot: map[string]string{}, Resolve: func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return bytes.Clone(b), ok }})
					if e != nil {
						t.Fatal(e)
					}
					want := map[string]bool{"reference()Ljava/lang/String;": false, "integer()I": false, "wide()J": false, "single()F": false, "decimal()D": false, "nullValue()Ljava/lang/String;": false}
					for _, m := range result.StubMethods {
						if _, ok := want[m]; ok {
							want[m] = true
						}
					}
					unsupported := map[string]bool{}
					for _, d := range result.Diagnostics {
						if d.Code == core.DiagCondyUnsupported {
							unsupported[d.Method] = true
						}
						if d.Code == core.DiagCondyInvalid {
							t.Fatal("valid constant is not malformed", d)
						}
					}
					for m, stub := range want {
						if !stub || !unsupported[m] {
							t.Errorf("unavailable constant source %s: stub=%v diagnostic=%v", m, stub, unsupported[m])
						}
					}
					if result.Status != "partial" || strings.Contains(result.Source, "CONSTANT_Dynamic") || strings.Contains(result.Source, "unsupported condy") || !bytes.Equal(before, files["ConstantSubject.class"]) {
						t.Fatalf("unsupported constant became an expression: status=%s\n%s", result.Status, result.Source)
					}
				})
			}
		})
	}
}
