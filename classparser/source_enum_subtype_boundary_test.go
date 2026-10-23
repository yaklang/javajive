package javaclassparser

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialEnumSubtypeArrayBoundaryRequiresExactDeclarationsAndOrigins(t *testing.T) {
	compiler, _ := t04Tools(t)
	const source = `public class SubtypeBoundaryOwner {public enum Mode {A{public long read(long x){return x+1;}},B;public long read(long x){return x-1;}}}`
	files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, source, "SubtypeBoundaryOwner", "none", false)
	const owner = "SubtypeBoundaryOwner$Mode"
	const subtype = owner + "$1"
	for _, variant := range []string{"control", "renamed subtype", "unknown subtype", "wrong superclass", "ordinary class", "wrong resolved identity", "missing constructor", "duplicate constructor", "different constructor descriptor", "new owner differs", "invocation owner differs", "wrong hidden ordinal", "residual stack", "canceled", "work"} {
		t.Run(variant, func(t *testing.T) {
			o := mustParseLowering(t, bytes.Clone(files[owner+".class"]))
			body := mustParseLowering(t, bytes.Clone(files[subtype+".class"]))
			var init *MemberInfo
			for _, m := range o.Methods {
				if name, _ := sourceBridgeUTF8(o, m.NameIndex); name == "<clinit>" {
					init = m
				}
			}
			if init == nil {
				t.Fatal("actual fresh initializer")
			}
			code := constructorLoweringCode(init)
			ops, known := nativeEnumMethodOps(o, init, nil)
			if !known {
				t.Fatal("fresh instructions")
			}
			var ctor *MemberInfo
			for _, m := range body.Methods {
				if name, _ := sourceBridgeUTF8(body, m.NameIndex); name == "<init>" {
					ctor = m
				}
			}
			if ctor == nil {
				t.Fatal("actual subclass constructor")
			}
			var work *workbudget.Budget
			switch variant {
			case "renamed subtype":
				for _, object := range []*ClassObject{o, body} {
					for _, c := range object.ConstantPool {
						if u, ok := c.(*ConstantUtf8Info); ok {
							u.Value = strings.ReplaceAll(u.Value, subtype, "UnrelatedAllocationSymbol")
						}
					}
				}
			case "wrong superclass":
				body.SuperClass = uint16(body.ConstantPoolManager.AddNewClassInfo("java/lang/Object"))
			case "ordinary class":
				body.AccessFlags &^= 0x4000
			case "wrong resolved identity":
				body.ThisClass = uint16(body.ConstantPoolManager.AddNewClassInfo("UnrelatedAllocationSymbol"))
			case "missing constructor":
				var methods []*MemberInfo
				for _, m := range body.Methods {
					if m != ctor {
						methods = append(methods, m)
					}
				}
				body.Methods = methods
			case "duplicate constructor":
				body.Methods = append(body.Methods, ctor)
			case "different constructor descriptor":
				ctor.DescriptorIndex = uint16(body.ConstantPoolManager.AddUtf8Info("(Ljava/lang/String;IJ)V"))
			case "new owner differs":
				pc := int(ops[0].CurrentOffset)
				index := uint16(o.ConstantPoolManager.AddNewClassInfo(owner))
				code.Code[pc+1], code.Code[pc+2] = byte(index>>8), byte(index)
			case "invocation owner differs":
				changed := false
				for _, op := range ops {
					if op.Instr.OpCode != core.OP_INVOKESPECIAL {
						continue
					}
					index, ok := nativeEnumCPIndex(op)
					if !ok {
						t.Fatal("constructor reference")
					}
					ref := nativeConstantMember(o.ConstantPool[index-1])
					name, _ := sourceBridgeClassName(o, ref.ClassIndex)
					if name == subtype {
						ref.ClassIndex = o.ThisClass
						changed = true
						break
					}
				}
				if !changed {
					t.Fatal("actual subclass call")
				}
			case "wrong hidden ordinal":
				changed := false
				for _, op := range ops {
					if op.Instr.OpCode == core.OP_ICONST_0 {
						code.Code[int(op.CurrentOffset)] = byte(core.OP_ICONST_1)
						changed = true
						break
					}
				}
				if !changed {
					t.Fatal("actual hidden ordinal")
				}
			case "residual stack":
				code.Code = append([]byte{byte(core.OP_ICONST_0)}, code.Code...)
				code.AttrLen++
				code.MaxStack++
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			}
			// Reparse actual bytes so flags, lengths and raw symbol identities agree.
			o = mustParseLowering(t, o.Bytes())
			body = mustParseLowering(t, body.Bytes())
			for _, m := range o.Methods {
				if name, _ := sourceBridgeUTF8(o, m.NameIndex); name == "<clinit>" {
					init = m
				}
			}
			ops, known = nativeEnumMethodOps(o, init, nil)
			if !known {
				t.Fatal("valid decoded counterexample")
			}
			fields := map[sourceFieldIdentity]*MemberInfo{}
			var constants []string
			for _, field := range o.Fields {
				name, nk := sourceBridgeUTF8(o, field.NameIndex)
				desc, dk := sourceBridgeUTF8(o, field.DescriptorIndex)
				if !nk || !dk {
					t.Fatal("actual declarations")
				}
				fields[sourceFieldIdentity{name, desc}] = field
				if field.AccessFlags&0x4000 != 0 {
					constants = append(constants, name)
				}
			}
			resolve := func(name string) (*ClassObject, bool) {
				if variant == "unknown subtype" {
					return nil, false
				}
				expected := subtype
				if variant == "renamed subtype" {
					expected = "UnrelatedAllocationSymbol"
				}
				return body, name == expected
			}
			before := o.Bytes()
			boundary, ok := enumArrayConstantBoundary(o, init, ops, constants, fields, resolve, work)
			positive := variant == "control" || variant == "renamed subtype"
			if ok != positive {
				t.Fatal("typed allocation boundary", variant, boundary, ok)
			}
			if positive && (boundary <= 0 || boundary >= len(ops) || ops[boundary].Instr.OpCode != core.OP_INVOKESTATIC || ops[boundary-1].Instr.OpCode != core.OP_PUTSTATIC) {
				t.Fatal("cut must follow all constant stores and precede the fresh array helper")
			}
			if work != nil && work.Err() == nil {
				t.Fatal("budget/cancellation not classified")
			}
			if !bytes.Equal(before, o.Bytes()) {
				t.Fatal("read-only boundary proof mutated instructions")
			}
		})
	}
}
