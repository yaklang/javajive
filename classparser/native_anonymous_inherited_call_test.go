package javaclassparser

import (
	"context"
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeAnonymousInheritedCallRequiresOriginalClassLookup(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousInheritedCallFixture)
	for _, kind := range []string{"original", "public", "two ancestors", "package-private inheritance gap", "public inheritance gap", "foreign package", "private", "protected", "static", "bridge", "synthetic", "abstract", "generic", "nil signature", "return alternative", "child override", "unknown parent", "cycle", "interface", "unknown child", "wrong opcode", "wrong CP tag", "short operand", "arguments", "budget", "memory", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			forest := &nativeAnonymousForest{units: map[string]*nativeAnonymousClass{}, objects: map[string]*ClassObject{}}
			for name, raw := range files {
				obj, e := Parse(raw)
				if e != nil {
					t.Fatal(e)
				}
				forest.objects[strings.TrimSuffix(name, ".class")] = obj
			}
			caller := forest.objects["ImmediateOwner"]
			child := forest.objects["ImmediateOwner$1"]
			parent := forest.objects["ImmediateOwner$Layer"]
			forest.units[child.GetClassName()] = &nativeAnonymousClass{object: child}
			var target *MemberInfo
			for _, m := range parent.Methods {
				n, _ := sourceBridgeUTF8(parent, m.NameIndex)
				if n == "run" {
					target = m
				}
			}
			if target == nil {
				t.Fatal("missing inherited declaration")
			}
			var op *core.OpCode
			for _, m := range caller.Methods {
				n, _ := sourceBridgeUTF8(caller, m.NameIndex)
				if n != "get" {
					continue
				}
				for _, a := range m.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(caller.ConstantPool, i) })
						if e := d.ParseOpcode(); e != nil {
							t.Fatal(e)
						}
						for _, candidate := range d.Opcodes() {
							if symbol := constructorMotionMember(caller, candidate, core.OP_INVOKEVIRTUAL); symbol != nil && symbol.Member == "run" {
								op = candidate
							}
						}
					}
				}
			}
			if op == nil {
				t.Fatal("missing original invocation")
			}
			var work *workbudget.Budget
			switch kind {
			case "public":
				target.AccessFlags |= 1
			case "two ancestors", "package-private inheritance gap", "public inheritance gap":
				copy := *parent
				copy.ConstantPool = append([]ConstantInfo(nil), parent.ConstantPool...)
				cp := NewConstantPoolWithConstant(&copy.ConstantPool)
				mid := "Intermediate"
				if kind != "two ancestors" {
					mid = "foreign/Intermediate"
				}
				if kind == "public inheritance gap" {
					target.AccessFlags |= 1
				}
				copy.ThisClass = uint16(cp.AddNewClassInfo(mid))
				copy.SuperClass = uint16(cp.AddNewClassInfo(parent.GetClassName()))
				copy.Methods = nil
				forest.objects[mid] = &copy
				cp2 := NewConstantPoolWithConstant(&child.ConstantPool)
				child.SuperClass = uint16(cp2.AddNewClassInfo(mid))
			case "foreign package":
				cp := NewConstantPoolWithConstant(&caller.ConstantPool)
				caller.ThisClass = uint16(cp.AddNewClassInfo("foreign/Caller"))
			case "private":
				target.AccessFlags |= 2
			case "protected":
				target.AccessFlags |= 4
			case "static":
				target.AccessFlags |= 8
			case "bridge":
				target.AccessFlags |= 0x40
			case "synthetic":
				target.AccessFlags |= 0x1000
			case "abstract":
				target.AccessFlags |= 0x0400
			case "nil signature":
				target.Attributes = append(target.Attributes, (*SignatureAttribute)(nil))
			case "generic":
				target.Attributes = append(target.Attributes, &SignatureAttribute{})
			case "return alternative":
				copy := *target
				cp := NewConstantPoolWithConstant(&parent.ConstantPool)
				copy.DescriptorIndex = uint16(cp.AddUtf8Info("()Ljava/lang/String;"))
				parent.Methods = append(parent.Methods, &copy)
			case "child override":
				copy := *target
				cp := NewConstantPoolWithConstant(&child.ConstantPool)
				copy.NameIndex = uint16(cp.AddUtf8Info("run"))
				copy.DescriptorIndex = uint16(cp.AddUtf8Info("()Ljava/lang/Object;"))
				child.Methods = append(child.Methods, &copy)
			case "unknown parent":
				delete(forest.objects, parent.GetClassName())
			case "cycle":
				parent.Methods = nil
				parent.SuperClass = parent.ThisClass
			case "interface":
				parent.AccessFlags |= 0x0200
			case "unknown child":
				forest.units = nil
			case "wrong opcode":
				copy := *op.Instr
				copy.OpCode = core.OP_INVOKESPECIAL
				op.Instr = &copy
			case "wrong CP tag":
				idx := int(binary.BigEndian.Uint16(op.Data))
				old := caller.ConstantPool[idx-1].(*ConstantMethodrefInfo)
				caller.ConstantPool[idx-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: old.ConstantMemberrefInfo}
			case "short operand":
				op.Data = op.Data[:1]
			case "arguments":
				target.DescriptorIndex = uint16(NewConstantPoolWithConstant(&parent.ConstantPool).AddUtf8Info("(I)Ljava/lang/Object;"))
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := nativeAnonymousInheritedCall(forest, caller, op, work)
			want := kind == "original" || kind == "public" || kind == "two ancestors" || kind == "public inheritance gap"
			if got != want {
				t.Fatalf("admitted=%v", got)
			}
		})
	}
}
