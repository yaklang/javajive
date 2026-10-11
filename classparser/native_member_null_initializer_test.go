package javaclassparser

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeInterfaceInitializerOriginalNullIsNonconstant(t *testing.T) {
	for _, descriptor := range []string{"Ljava/lang/String;", "Ljava/lang/Object;", "[Ljava/lang/String;", "I", "J", "Z"} {
		t.Run(descriptor, func(t *testing.T) {
			if got := interfaceInitializerNonconstant(values.NewOriginalNullLiteral(0), descriptor); got != (len(descriptor) > 1) {
				t.Fatalf("original ACONST_NULL nonconstant=%v", got)
			}
		})
	}
	for _, variant := range []string{"string constant", "forged spelling", "copied witness", "changed value"} {
		t.Run(variant, func(t *testing.T) {
			v := values.NewOriginalNullLiteral(0)
			switch variant {
			case "string constant":
				v = values.NewJavaLiteral("null", types.NewJavaClass("java.lang.String"))
			case "forged spelling":
				v = values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
			case "copied witness":
				copy := *v
				v = &copy
			case "changed value":
				v.Data = "kept"
			}
			if interfaceInitializerNonconstant(v, "Ljava/lang/String;") {
				t.Fatal("a spelling or stale witness cannot establish original null")
			}
		})
	}
}

func TestNativeInterfaceInitializerOwnReadRequiresOriginalPartition(t *testing.T) {
	files := nativeCompileClasses(t, `class NullReadOwner{interface Contract{String early=Contract.later;String later=null;}}`)
	variants := []string{"original", "no code", "missing witness", "wrong owner", "wrong name", "wrong descriptor", "wrong PC", "handle alias", "after store", "missing partition", "too many writes", "ConstantValue", "mutable declaration", "duplicate declaration", "wrong opcode", "wrong CP tag", "budget", "canceled"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			object, err := Parse(append([]byte(nil), files["NullReadOwner$Contract.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(object)
			var code *CodeAttribute
			for _, method := range object.Methods {
				name, _ := sourceBridgeUTF8(object, method.NameIndex)
				if name == "<clinit>" {
					for _, attribute := range method.Attributes {
						if c, ok := attribute.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			writes, err := d.interfaceWrites(code)
			if err != nil || len(writes) != 2 || code.Code[0] != core.OP_GETSTATIC {
				t.Fatalf("original read/store packet %v %v", writes, err)
			}
			index := int(code.Code[1])<<8 | int(code.Code[2])
			member := GetValueFromCP(object.ConstantPool, index).(*values.JavaClassMember)
			field := *member
			field.OriginPC, field.HasOriginPC = 0, true
			field.MarkOriginalFieldRead(member, 0)
			var declaration *MemberInfo
			for _, f := range object.Fields {
				name, _ := sourceBridgeUTF8(object, f.NameIndex)
				if name == "later" {
					declaration = f
				}
			}
			if declaration == nil {
				t.Fatal("original destination declaration")
			}
			storePC := writes[0].pc
			switch variant {
			case "no code":
				code = nil
			case "missing witness":
				field = *member
				field.OriginPC, field.HasOriginPC = 0, true
			case "wrong owner":
				field.Name = "Other"
			case "wrong name":
				field.Member = "early"
			case "wrong descriptor":
				field.Description = "Ljava/lang/Object;"
			case "wrong PC":
				field.OriginPC++
			case "handle alias":
				field.RefKind = 2
			case "after store":
				storePC = 0
			case "missing partition":
				writes = writes[:1]
			case "too many writes":
				writes = make([]interfaceFieldWrite, 129)
			case "ConstantValue":
				declaration.Attributes = append(declaration.Attributes, &ConstantValueAttribute{})
			case "mutable declaration":
				declaration.AccessFlags &^= 0x10
			case "duplicate declaration":
				copy := *declaration
				object.Fields = append(object.Fields, &copy)
			case "wrong opcode":
				code.Code[0] = core.OP_PUTSTATIC
			case "wrong CP tag":
				ref := object.ConstantPool[index-1].(*ConstantFieldrefInfo)
				object.ConstantPool[index-1] = &ConstantMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := d.interfaceInitializerOwnNonconstantRead(&field, code, storePC, writes); got != (variant == "original") {
				t.Fatalf("original partition admitted=%v", got)
			}
		})
	}
}

// Keep the caller bytecode, nested ownership and all field/method descriptors.
// A null String is not a JLS constant variable: introducing an initializer
// method would unnecessarily change the interface ABI and reject its family.
const nativeNullInterfaceInitializerFixture = `class NullInitEffects{static String trace="";static String text(){trace+="A";return "kept";}}
class NullInitBase{final int code;NullInitBase(int n){code=n;}}
class NullInitOwner{interface Contract{String early=Contract.missing;String text=NullInitEffects.text();String missing=null;Object object=null;String[] array=null;default String value(){return missing;}}class Child extends NullInitBase implements Contract{Child(int n){super(n);}int code(){return code;}}}
class NullInitDriver{public static void main(String[]args)throws Exception{Class<?>api=Class.forName("NullInitOwner$Contract",false,NullInitDriver.class.getClassLoader());if(!NullInitEffects.trace.equals(""))throw new AssertionError("eager initialization");if(api.getDeclaredMethods().length!=1)throw new AssertionError("added initializer ABI");NullInitOwner.Child child=new NullInitOwner().new Child(Integer.MIN_VALUE);if(child.code()!=Integer.MIN_VALUE||child.value()!=null||NullInitOwner.Contract.early!=null||NullInitOwner.Contract.missing!=null||NullInitOwner.Contract.object!=null||NullInitOwner.Contract.array!=null||!NullInitOwner.Contract.text.equals("kept")||!NullInitEffects.trace.equals("A"))throw new AssertionError("original field binding or initialization");System.out.println("kept:"+child.code()+":"+api.getDeclaredMethods().length);}}`

func TestNativeMemberNullStringInitializerPreservesBindingAndABI(t *testing.T) {
	nativeNullInterfaceInitializerRoundTrip(t, nativeNullInterfaceInitializerFixture)
}

func TestNativeMemberNullInterfaceCyclesAndWideInitializersPreserveDefaults(t *testing.T) {
	fixture := strings.Replace(nativeNullInterfaceInitializerFixture, "default String value()", "String cycleA=Contract.cycleB;String cycleB=Contract.cycleA;long earlyNumber=Contract.laterNumber;long laterNumber=Long.parseLong(\"-9223372036854775808\");default String value()", 1)
	fixture = strings.Replace(fixture, "if(child.code()!=", "if(NullInitOwner.Contract.cycleA!=null||NullInitOwner.Contract.cycleB!=null||NullInitOwner.Contract.earlyNumber!=0L||NullInitOwner.Contract.laterNumber!=Long.MIN_VALUE)throw new AssertionError(\"cycles or wide default read\");if(child.code()!=", 1)
	nativeNullInterfaceInitializerRoundTrip(t, fixture)
}

func nativeNullInterfaceInitializerRoundTrip(t *testing.T, fixture string) {
	t.Helper()
	javac, java := t04Tools(t)

	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, fixture, debug)
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			want := t04RunJava(t, java, original, "NullInitDriver")
			if want != "kept:-2147483648:1\n" {
				t.Fatalf("independent original oracle %q", want)
			}
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					z := nativeArchive(t, files)
					defer z.Close()
					src, err := z.ReadFile("NullInitOwner.class")
					if err != nil || strings.Contains(string(src), DecompileStubMarker) || !strings.Contains(string(src), "interface Contract") || !strings.Contains(string(src), "class Child") {
						t.Fatalf("complete original family: %v\n%s", err, src)
					}
					output := t.TempDir()
					for name, raw := range files {
						if !strings.HasPrefix(name, "NullInitOwner") {
							if err := os.WriteFile(filepath.Join(output, name), raw, 0600); err != nil {
								t.Fatal(err)
							}
						}
					}
					file := filepath.Join(output, "NullInitOwner.java")
					if err := os.WriteFile(file, src, 0600); err != nil {
						t.Fatal(err)
					}
					if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", output, "-d", output, file).CombinedOutput(); err != nil {
						t.Fatalf("rebuilt: %v %s\n%s", err, out, src)
					}
					if got := t04RunJava(t, java, output, "NullInitDriver"); got != want {
						t.Fatalf("unchanged caller %q != %q", got, want)
					}
					for name, raw := range files {
						if strings.HasPrefix(name, "NullInitOwner") {
							rebuilt, err := os.ReadFile(filepath.Join(output, name))
							if err != nil {
								t.Fatal(err)
							}
							if nativeBinaryShape(t, rebuilt) != nativeBinaryShape(t, raw) {
								t.Fatalf("original ABI changed: %s\n%s\n%s", name, nativeBinaryShape(t, raw), nativeBinaryShape(t, rebuilt))
							}
						}
					}
				})
			}
		})
	}
}
