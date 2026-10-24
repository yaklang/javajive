package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Move the same receiver-free factory after the actual superclass invocation.
// The original holder field becomes nonfinal so the subclass's PUTFIELD is
// legal. The unchanged binary driver verifies this new original, including
// the still-static implementation and anonymous metadata. A flag-only query
// would accept it; the original constructor frame must distinguish it.
func nativeConstructorLambdaFactoryAfterInit(t *testing.T, files map[string][]byte) {
	t.Helper()
	root, err := Parse(bytes.Clone(files["CtorLambdaOwner.class"]))
	if err != nil {
		t.Fatal(err)
	}
	var code *CodeAttribute
	for _, method := range root.Methods {
		name, _ := sourceBridgeUTF8(root, method.NameIndex)
		if name == "<init>" {
			for _, a := range method.Attributes {
				if body, ok := a.(*CodeAttribute); ok {
					code = body
				}
			}
		}
	}
	if code == nil {
		t.Fatal("original constructor Code")
	}
	decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(root.ConstantPool, index) })
	if err := decoder.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	super, dynamic := 0, 0
	for _, op := range decoder.Opcodes() {
		if op.Instr.OpCode == core.OP_INVOKEDYNAMIC {
			dynamic = int(core.Convert2bytesToInt(op.Data[:2]))
		}
		if target := constructorMotionMember(root, op, core.OP_INVOKESPECIAL); target != nil && target.Name == "CtorLambdaHolder" && target.Member == "<init>" {
			super = int(core.Convert2bytesToInt(op.Data[:2]))
		}
	}
	if super == 0 || dynamic == 0 {
		t.Fatal("original constructor delegation and factory")
	}
	cp := root.ConstantPoolManager
	field := cp.AppendConstantInfo(&ConstantFieldrefInfo{ConstantMemberrefInfo: *cp.newMemberrefInfo("CtorLambdaHolder", "factory", "Ljava/util/function/Supplier;")})
	code.Code = []byte{core.OP_ALOAD_0, core.OP_ACONST_NULL, core.OP_INVOKESPECIAL, byte(super >> 8), byte(super), core.OP_ALOAD_0, core.OP_ALOAD_1, core.OP_ILOAD_2, core.OP_INVOKEDYNAMIC, byte(dynamic >> 8), byte(dynamic), 0, 0, core.OP_PUTFIELD, byte(field >> 8), byte(field), core.OP_RETURN}
	code.MaxStack, code.ExceptionTable, code.Attributes = 3, nil, nil
	code.AttrLen = uint32(12 + len(code.Code))
	files["CtorLambdaOwner.class"] = root.Bytes()
	holder, err := Parse(bytes.Clone(files["CtorLambdaHolder.class"]))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range holder.Fields {
		name, _ := sourceBridgeUTF8(holder, field.NameIndex)
		if name == "factory" {
			field.AccessFlags &^= 0x0010
		}
	}
	files["CtorLambdaHolder.class"] = holder.Bytes()
}

func nativeConstructorLambdaOriginalRun(t *testing.T, files map[string][]byte, driver, want string) {
	t.Helper()
	dir := t.TempDir()
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	_, java := t04Tools(t)
	if got := t04RunJava(t, java, dir, driver); got != want {
		t.Fatal("independent original constructor-lambda JVM", got)
	}
}

func TestAdversarialNativeConstructorLambdaContextRequiresOriginalFactoryFrame(t *testing.T) {
	base := nativePrivateEnumCompile(t, nativeConstructorLambdaContextFixture, "CtorLambdaOwner", "none")
	nativeConstructorLambdaOriginalRun(t, base, "CtorLambdaDriver", "36:constructor:lambda:lexical:capture\n")
	after := map[string][]byte{}
	for name, raw := range base {
		after[name] = bytes.Clone(raw)
	}
	nativeConstructorLambdaFactoryAfterInit(t, after)
	nativeConstructorLambdaOriginalRun(t, after, "CtorLambdaDriver", "36:constructor:lambda:lexical:capture\n")
	sharedSource := strings.Replace(nativeConstructorLambdaContextFixture, "\n}\nclass CtorLambdaDriver", `
 CtorLambdaOwner(final Object input,final int number,boolean extra){super(()->new CtorLambdaBase(input,CtorLambdaEffects.input(number)){Object peek(){return input;}long wide(){return 0;}});}
}
class CtorLambdaDriver`, 1) + `
class CtorSharedDriver{public static void main(String[]args)throws Exception{new CtorLambdaOwner(null,1,true).factory.get();Class.forName("CtorLambdaOwner$2");CtorLambdaDriver.main(args);}}
`
	shared := nativePrivateEnumCompile(t, sharedSource, "CtorLambdaOwner", "none")
	object, err := Parse(bytes.Clone(shared["CtorLambdaOwner.class"]))
	if err != nil {
		t.Fatal(err)
	}
	child, err := Parse(bytes.Clone(shared["CtorLambdaOwner$1.class"]))
	if err != nil {
		t.Fatal(err)
	}
	_, lexical, known := originalAnonymousOwner(child)
	if !known {
		t.Fatal("original shared child owner")
	}
	handle := uint16(0)
	for i, constant := range object.ConstantPool {
		if ref, ok := constant.(*ConstantMethodHandleInfo); ok {
			if target, valid := nativeLambdaOriginalHandleTarget(object, ref); valid && target.owner == object.GetClassName() && target.name+target.descriptor == lexical {
				handle = uint16(i + 1)
			}
		}
	}
	if handle == 0 {
		t.Fatal("original implementation handle")
	}
	changed := 0
	for _, attribute := range object.Attributes {
		if table, ok := attribute.(*BootstrapMethodsAttribute); ok {
			for _, bootstrap := range table.BootstrapMethods {
				if len(bootstrap.BootstrapArguments) != 3 {
					t.Fatal("original metafactory schema")
				}
				bootstrap.BootstrapArguments[1] = handle
				changed++
			}
		}
	}
	if changed != 2 {
		t.Fatal("original two distinct factories", changed)
	}
	shared["CtorLambdaOwner.class"] = object.Bytes()
	nativeConstructorLambdaOriginalRun(t, shared, "CtorSharedDriver", "36:constructor:lambda:lexical:capture\n")
	for _, variant := range []string{"original", "after initialization", "shared factory", "modern profile", "wrong target", "wrong child owner", "wrong lexical", "missing implementation", "ordinary implementation", "missing bootstrap", "static constructor", "wrong self flags", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			files := base
			if variant == "after initialization" {
				files = after
			} else if variant == "shared factory" {
				files = shared
			}
			root, err := Parse(bytes.Clone(files["CtorLambdaOwner.class"]))
			if err != nil {
				t.Fatal(err)
			}
			child, err := Parse(bytes.Clone(files["CtorLambdaOwner$1.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(root)
			d.options = DecompileOptions{SourceCompiler: NativeJavac8, TargetSourceVersion: 8}
			d.foldSiblingResolver = func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return bytes.Clone(b), ok }
			_, lexical, known := originalAnonymousOwner(child)
			if !known {
				t.Fatal("original lexical method")
			}
			switch variant {
			case "modern profile":
				d.options.SourceCompiler = ModernJavac
			case "wrong target":
				d.options.TargetSourceVersion = 11
			case "wrong lexical":
				lexical += "changed"
			case "wrong child owner":
				for _, attribute := range child.Attributes {
					if enclosing, ok := attribute.(*UnparsedAttribute); ok && enclosing.Name == "EnclosingMethod" && len(enclosing.Info) == 4 {
						binary.BigEndian.PutUint16(enclosing.Info[:2], child.ThisClass)
					}
				}
			case "missing implementation", "ordinary implementation", "static constructor":
				for i, method := range root.Methods {
					name, _ := sourceBridgeUTF8(root, method.NameIndex)
					desc, _ := sourceBridgeUTF8(root, method.DescriptorIndex)
					if name+desc == lexical {
						if variant == "missing implementation" {
							root.Methods = append(root.Methods[:i], root.Methods[i+1:]...)
							break
						}
						if variant == "ordinary implementation" {
							method.AccessFlags &^= 0x1000
						}
					}
					if variant == "static constructor" && name == "<init>" {
						method.AccessFlags |= StaticFlag
					}
				}
			case "missing bootstrap":
				for i, attribute := range root.Attributes {
					if _, ok := attribute.(*BootstrapMethodsAttribute); ok {
						root.Attributes = append(root.Attributes[:i], root.Attributes[i+1:]...)
						break
					}
				}
			case "wrong self flags":
				for _, attribute := range child.Attributes {
					if table, ok := attribute.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							name, _ := sourceBridgeClassName(child, row.InnerClassInfoIndex)
							if name == child.GetClassName() {
								row.InnerClassAccessFlags |= StaticFlag
							}
						}
					}
				}
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			beforeRoot, beforeChild := root.Bytes(), child.Bytes()
			metadata := d.buildInvocationMetadata()
			got := d.nativeAnonymousPreinitLambdaContext(child, lexical, metadata)
			if got != (variant == "original" || variant == "wrong self flags") {
				t.Fatalf("original preinit factory context=%v for %s", got, variant)
			}
			if variant == "wrong self flags" && d.nativeAnonymousConstructorForCompiler(child, root.GetClassName(), lexical, "", nil, nil, metadata, nil) != nil {
				t.Fatal("context proof granted anonymous declaration flags")
			}
			if !bytes.Equal(beforeRoot, root.Bytes()) || !bytes.Equal(beforeChild, child.Bytes()) {
				t.Fatal("context query changed original objects")
			}
		})
	}
}
