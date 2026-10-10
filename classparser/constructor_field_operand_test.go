package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestAdversarialConstructorFieldOperandEvidence(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "FieldProofOwner.java")
	if err := os.WriteFile(file, []byte(`class FieldProofBox {Object value;static Object shared;}class FieldProofBase {FieldProofBase(Object a,Object b){}}class FieldProofOwner {class Member extends FieldProofBase {Member(FieldProofBox b){super(b.value,FieldProofBox.shared);}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v\n%s", err, out)
	}
	raw := readClassBytes(t, dir, "FieldProofOwner$Member")
	for _, variant := range []string{"original", "THIS receiver", "missing receiver", "primitive receiver", "unrelated receiver", "truncated field", "wrong field tag", "nil field", "invalid descriptor", "wrong static tag", "work budget"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			var code *CodeAttribute
			var desc string
			for _, m := range obj.Methods {
				name, _ := obj.getUtf8(m.NameIndex)
				if name != "<init>" {
					continue
				}
				desc, _ = obj.getUtf8(m.DescriptorIndex)
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						code = c
					}
				}
			}
			if code == nil {
				t.Fatal("missing original code")
			}
			params, _, err := callbinding.Descriptor(desc)
			if err != nil {
				t.Fatal(err)
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(decoder)
			if len(ops) != 9 || ops[5].Instr.OpCode != core.OP_GETFIELD || ops[6].Instr.OpCode != core.OP_GETSTATIC {
				t.Fatal("missing original field operands")
			}
			cp := core.Convert2bytesToInt(ops[5].Data)
			switch variant {
			case "THIS receiver":
				copy := *ops[3].Instr
				ops[4].Instr = &copy
			case "missing receiver":
				ops = append(ops[:4], ops[5:]...)
			case "primitive receiver":
				copy := *ops[4].Instr
				copy.OpCode = core.OP_ICONST_0
				ops[4].Instr = &copy
			case "unrelated receiver":
				params[1] = "Ljava/lang/String;"
			case "truncated field":
				ops[5].Data = []byte{0}
			case "wrong field tag":
				obj.ConstantPool[cp-1] = &ConstantUtf8Info{Value: "value"}
			case "nil field":
				obj.ConstantPool[cp-1] = (*ConstantFieldrefInfo)(nil)
			case "invalid descriptor":
				ref := obj.ConstantPool[cp-1].(*ConstantFieldrefInfo)
				nt := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				obj.ConstantPool[nt.DescriptorIndex-1] = &ConstantUtf8Info{Value: "V"}
			case "wrong static tag":
				cp := core.Convert2bytesToInt(ops[6].Data)
				obj.ConstantPool[cp-1] = &ConstantUtf8Info{Value: "shared"}
			case "work budget":
				copies := make([]*core.OpCode, 513)
				for i := range copies {
					copies[i] = ops[4]
				}
				ops = append(append(append([]*core.OpCode{}, ops[:4]...), copies...), ops[5:]...)
			}
			next, call := constructorMotionDelegation(obj, ops, 3, params, constructorParameterSlots(params), nil)
			if got := next != 0 && call != nil; got != (variant == "original") {
				t.Fatalf("delegation proof=%v next=%d call=%#v", got, next, call)
			}
		})
	}
}
