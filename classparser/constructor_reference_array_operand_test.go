package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"testing"
)

func TestConstructorReferenceArrayOperandRequiresOriginalFreshPacket(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberArraySuperFixture)
	for _, variant := range []string{"original", "missing size", "wrong size type", "truncated allocation", "wrong component tag", "truncated store", "primitive store", "missing duplicate", "wrong index type", "wrong element type"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(append([]byte(nil), files["ArraySuperOwner$Child.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			reader := NewClassObjectDumper(obj)
			reader.foldSiblingResolver = func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok }
			var code *CodeAttribute
			var params []string
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name != "<init>" {
					continue
				}
				desc, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
				params, _, _ = callbinding.Descriptor(desc)
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						code = c
					}
				}
			}
			d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if e = d.ParseOpcode(); e != nil {
				t.Fatal(e)
			}
			ops := constructorMotionOps(d)
			alloc, store, dup := -1, -1, -1
			for i, op := range ops {
				switch op.Instr.OpCode {
				case core.OP_ANEWARRAY:
					alloc = i
				case core.OP_AASTORE:
					if store < 0 {
						store = i
					}
				case core.OP_DUP:
					if dup < 0 {
						dup = i
					}
				}
			}
			if alloc < 0 || store < 0 || dup < 0 {
				t.Fatal("original fresh array packet")
			}
			switch variant {
			case "missing size":
				ops = append(append([]*core.OpCode{}, ops[:alloc-1]...), ops[alloc:]...)
			case "wrong size type":
				ops[alloc-1].Instr = core.InstrInfos[core.OP_FCONST_0]
			case "truncated allocation":
				ops[alloc].Data = []byte{0}
			case "wrong component tag":
				obj.ConstantPool[core.Convert2bytesToInt(ops[alloc].Data)-1] = &ConstantUtf8Info{Value: "not a class"}
			case "truncated store":
				ops[store].Data = []byte{0}
			case "primitive store":
				ops[store].Instr = core.InstrInfos[core.OP_IASTORE]
			case "missing duplicate":
				ops = append(append([]*core.OpCode{}, ops[:dup]...), ops[dup+1:]...)
			case "wrong index type":
				ops[store-2].Instr = core.InstrInfos[core.OP_FCONST_0]
			case "wrong element type":
				ops[store-1].Instr = core.InstrInfos[core.OP_ICONST_0]
				ops[store-1].Data = nil
			}
			next, call := constructorMotionDelegation(obj, ops, 3, params, constructorParameterSlots(params), reader.buildInvocationMetadata())
			if got := next > 0 && call != nil; got != (variant == "original") {
				t.Fatalf("fresh packet proof=%v next=%d", got, next)
			}
		})
	}
}
