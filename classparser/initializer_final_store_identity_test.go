package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestAdversarialInitializerFinalStoreNeedsLosslessDeclarationIdentity(t *testing.T) {
	compiler, _ := t04Tools(t)
	files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, `public class FinalIdentity {static int value;static {value=42;}}`, "FinalIdentity", "none", false)
	for _, variant := range []string{"control", "exact unusual identity", "display collision", "constant value", "nonfinal", "nonstatic", "volatile", "different declaration", "foreign owner", "method flags", "wrong PC", "duplicate field"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(files["FinalIdentity.class"]))
			f := obj.Fields[0]
			f.AccessFlags |= 0x10
			m := sourceParameterTestMethod(t, obj, "<clinit>")
			c := constructorLoweringCode(m)
			ops, known := enumEntryInitializerOps(obj, m, nil)
			if !known {
				t.Fatal("initializer")
			}
			pc := -1
			for _, op := range ops {
				if op.Instr.OpCode == core.OP_PUTSTATIC {
					pc = int(op.CurrentOffset)
				}
			}
			if pc < 0 {
				t.Fatal("missing store")
			}
			index := binary.BigEndian.Uint16(c.Code[pc+1:])
			ref := obj.ConstantPool[index-1].(*ConstantFieldrefInfo)
			pair := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
			switch variant {
			case "exact unusual identity", "display collision":
				obj.ConstantPool[f.NameIndex-1].(*ConstantUtf8Info).SetUnits([]uint16{'x', 0xd800})
				if variant == "display collision" {
					obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{})
					other := obj.ConstantPool[len(obj.ConstantPool)-1].(*ConstantUtf8Info)
					other.SetString(`x\uD800`)
					f.NameIndex = uint16(len(obj.ConstantPool))
					if other.Value != obj.ConstantPool[pair.NameIndex-1].(*ConstantUtf8Info).Value {
						t.Fatal("inactive alias control")
					}
				}
			case "constant value":
				f.Attributes = append(f.Attributes, &ConstantValueAttribute{})
			case "nonfinal":
				f.AccessFlags &^= 0x10
			case "nonstatic":
				f.AccessFlags &^= 8
			case "volatile":
				f.AccessFlags |= 0x40
			case "different declaration":
				f.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("other"))
			case "foreign owner":
				ref.ClassIndex = uint16(obj.ConstantPoolManager.AddNewClassInfo("Other"))
			case "method flags":
				m.AccessFlags = 0
			case "wrong PC":
				pc++
			case "duplicate field":
				obj.Fields = append(obj.Fields, f)
			}
			name, _ := sourceBridgeUTF8(obj, pair.NameIndex)
			desc, _ := sourceBridgeUTF8(obj, pair.DescriptorIndex)
			want := variant == "control" || variant == "exact unusual identity"
			if got := originalOwnStaticFinalStore(obj, m, c, pc, obj.GetClassName(), name, desc, nil); got != want {
				t.Fatalf("nonthrowing own store proof %v want %v", got, want)
			}
		})
	}
}
