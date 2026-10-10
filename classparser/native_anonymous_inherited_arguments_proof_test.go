package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeAnonymousInheritedArgumentsRequireExactLookup(t *testing.T) {
	files := nativeCompileClasses(t, anonymousInheritedArgumentFixture)
	variants := []string{"original", "argument drift", "return drift", "return alternative", "varargs", "bridge", "static", "private", "protected", "abstract", "generic", "resolved parent", "missing resolver", "resolver failure", "resolver nil", "resolver wrong name", "resolved cycle", "wrong opcode", "short operand", "budget", "memory", "canceled"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			f := &nativeAnonymousForest{units: map[string]*nativeAnonymousClass{}, objects: map[string]*ClassObject{}}
			for name, raw := range files {
				o, e := Parse(raw)
				if e != nil {
					t.Fatal(e)
				}
				f.objects[strings.TrimSuffix(name, ".class")] = o
			}
			caller, child, parent := f.objects["InheritedArgOwner"], f.objects["InheritedArgOwner$1"], f.objects["InheritedArgOwner$Layer"]
			f.units[child.GetClassName()] = &nativeAnonymousClass{object: child}
			var op *core.OpCode
			for _, m := range caller.Methods {
				name, _ := sourceBridgeUTF8(caller, m.NameIndex)
				if name != "get" {
					continue
				}
				for _, a := range m.Attributes {
					code, ok := a.(*CodeAttribute)
					if !ok {
						continue
					}
					d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(caller.ConstantPool, i) })
					if d.ParseOpcode() != nil {
						t.Fatal("original decoder")
					}
					for _, candidate := range constructorMotionOps(d) {
						symbol := constructorMotionMember(caller, candidate, core.OP_INVOKEVIRTUAL)
						if symbol != nil && symbol.Name == child.GetClassName() && symbol.Member == "run" && symbol.Description == "(II)Ljava/lang/Object;" {
							op = candidate
						}
					}
				}
			}
			var target *MemberInfo
			for _, m := range parent.Methods {
				name, _ := sourceBridgeUTF8(parent, m.NameIndex)
				desc, _ := sourceBridgeUTF8(parent, m.DescriptorIndex)
				if name == "run" && desc == "(II)Ljava/lang/Object;" {
					target = m
				}
			}
			if op == nil || target == nil {
				t.Fatal("original inherited parameter family missing")
			}
			var work *workbudget.Budget
			var resolver func(string) (*ClassObject, bool)
			descriptor := func(desc string) uint16 {
				return uint16(NewConstantPoolWithConstant(&parent.ConstantPool).AddUtf8Info(desc))
			}
			switch variant {
			case "argument drift":
				target.DescriptorIndex = descriptor("(JJ)Ljava/lang/Object;")
			case "return drift":
				target.DescriptorIndex = descriptor("(II)Ljava/lang/String;")
			case "return alternative":
				alternative := *target
				alternative.DescriptorIndex = descriptor("(II)Ljava/lang/String;")
				parent.Methods = append(parent.Methods, &alternative)
			case "varargs":
				target.AccessFlags |= 0x0080
			case "bridge":
				target.AccessFlags |= 0x0040
			case "static":
				target.AccessFlags |= 0x0008
			case "private":
				target.AccessFlags |= 0x0002
			case "protected":
				target.AccessFlags |= 0x0004
			case "abstract":
				target.AccessFlags |= 0x0400
			case "generic":
				target.Attributes = append(target.Attributes, &SignatureAttribute{})
			case "resolved parent", "missing resolver", "resolver failure", "resolver nil", "resolver wrong name", "resolved cycle":
				delete(f.objects, parent.GetClassName())
				if variant != "missing resolver" {
					resolver = func(name string) (*ClassObject, bool) { return parent, name == parent.GetClassName() }
				}
				switch variant {
				case "resolver failure":
					resolver = func(string) (*ClassObject, bool) { return parent, false }
				case "resolver nil":
					resolver = func(string) (*ClassObject, bool) { return nil, true }
				case "resolver wrong name":
					resolver = func(string) (*ClassObject, bool) { return caller, true }
				case "resolved cycle":
					parent.SuperClass = parent.ThisClass
					kept := []*MemberInfo{}
					for _, m := range parent.Methods {
						if m != target {
							kept = append(kept, m)
						}
					}
					parent.Methods = kept
				}
			case "wrong opcode":
				op.Instr = &core.Instruction{OpCode: core.OP_INVOKESPECIAL}
			case "short operand":
				op.Data = op.Data[:1]
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := nativeAnonymousInheritedCall(f, caller, op, work, resolver)
			if got != (variant == "original" || variant == "resolved parent") {
				t.Fatalf("accepted=%v", got)
			}
			// A declaration resolver supplies ancestry, never emitted source ownership.
			if variant == "resolved parent" && f.objects[parent.GetClassName()] != nil {
				t.Fatal("foreign declaration promoted into lexical source forest")
			}
		})
	}
}
