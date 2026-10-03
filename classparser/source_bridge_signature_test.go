package javaclassparser

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestAdversarialSourceBridgeReturnRequiresOriginalForwarder(t *testing.T) {
	t.Parallel()
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "SourceBridgeChild.java")
	source := `class SourceBridgeParent<T>{ public java.util.List<T> items(){return null;} }
public class SourceBridgeChild<T> extends SourceBridgeParent<T>{}`
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("original: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "SourceBridgeChild.class"))
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"proved", "not bridge", "not synthetic", "not public", "static", "synchronized", "native", "abstract", "extra final", "extra private", "extra protected", "no code", "duplicate code", "signature", "handler", "receiver", "virtual", "return", "extra instruction", "stack", "locals", "bad member", "bad name/type", "wrong name", "wrong descriptor", "self owner", "wrong parent", "duplicate declaration"} {
		t.Run(scenario, func(t *testing.T) {
			obj, err := Parse(append([]byte(nil), raw...))
			if err != nil {
				t.Fatal(err)
			}
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range obj.Methods {
				if isBridgeMethod(m.AccessFlags) {
					method = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if method == nil || code == nil {
				t.Fatal("original bridge missing")
			}
			cp := NewConstantPoolWithConstant(&obj.ConstantPool)
			ref := cp.IndexInfo(int(binary.BigEndian.Uint16(code.Code[2:4]))).(*ConstantMethodrefInfo)
			nt := cp.IndexInfo(int(ref.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
			switch scenario {
			case "not bridge":
				method.AccessFlags &^= 0x40
			case "not synthetic":
				method.AccessFlags &^= 0x1000
			case "not public":
				method.AccessFlags &^= 1
			case "static":
				method.AccessFlags |= 8
			case "synchronized":
				method.AccessFlags |= 0x20
			case "native":
				method.AccessFlags |= 0x100
			case "abstract":
				method.AccessFlags |= 0x400
			case "extra final":
				method.AccessFlags |= 0x10
			case "extra private":
				method.AccessFlags |= 2
			case "extra protected":
				method.AccessFlags |= 4
			case "no code":
				method.Attributes = nil
			case "duplicate code":
				method.Attributes = append(method.Attributes, code)
			case "signature":
				method.Attributes = append(method.Attributes, &SignatureAttribute{})
			case "handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 4, HandlerPc: 4}}
			case "receiver":
				code.Code[0] = core.OP_ACONST_NULL
			case "virtual":
				code.Code[1] = core.OP_INVOKEVIRTUAL
			case "return":
				code.Code[4] = core.OP_IRETURN
			case "extra instruction":
				code.Code = append([]byte{core.OP_NOP}, code.Code...)
			case "stack":
				code.MaxStack = 2
			case "locals":
				code.MaxLocals = 2
			case "bad member":
				code.Code[2], code.Code[3] = 0, 0
			case "bad name/type":
				ref.NameAndTypeIndex = 0
			case "wrong name":
				nt.NameIndex = uint16(cp.SearchUtf8Index("<init>"))
			case "wrong descriptor":
				nt.DescriptorIndex = uint16(cp.SearchUtf8Index("()V"))
			case "self owner":
				ref.ClassIndex = obj.ThisClass
			case "wrong parent":
				obj.SuperClass = obj.ThisClass
			case "duplicate declaration":
				obj.Methods = append(obj.Methods, method)
			}
			dumper := &ClassObjectDumper{obj: obj}
			get := dumper.buildSourceBridgeTargets()
			target, proved := get("SourceBridgeChild", "items", "()Ljava/util/List;")
			if proved != (scenario == "proved") || proved && target != "SourceBridgeParent" {
				t.Fatalf("target=%s proved=%v", target, proved)
			}
			if _, ok := get("SourceBridgeChild", "items", "()Ljava/lang/Object;"); ok {
				t.Fatal("descriptor-mismatched lookup borrowed bridge")
			}
		})
	}
}
