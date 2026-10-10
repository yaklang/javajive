package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeMemberAllocationIdentityRequiresOriginalTypedSites(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberSameOwnerAllocationFixture)
	for _, variant := range []string{"original", "wrong allocation owner", "uninitialized return", "wrong local category", "small locals", "small stack", "oversize frames", "bad handler", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(append([]byte(nil), files["SameAllocationOwner.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name == "make" {
					method = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if method == nil || code == nil {
				t.Fatal("original allocation method")
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			news, calls := []int{}, []int{}
			parameterPC := -1
			for _, op := range constructorMotionOps(decoder) {
				if op.Instr.OpCode == core.OP_NEW {
					news = append(news, int(op.CurrentOffset))
				}
				if call := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL); call != nil && call.Member == "<init>" {
					calls = append(calls, int(op.CurrentOffset))
				}
				if op.Instr.OpCode == core.OP_ALOAD_1 {
					parameterPC = int(op.CurrentOffset)
				}
			}
			if len(news) != 2 || len(calls) != 2 || parameterPC < 0 {
				t.Fatal("two distinct NEW sites and nested calls")
			}
			d := NewClassObjectDumper(obj)
			switch variant {
			case "wrong allocation owner":
				code.Code[news[1]+1], code.Code[news[1]+2] = byte(obj.ThisClass>>8), byte(obj.ThisClass)
			case "uninitialized return":
				for i := calls[1]; i < calls[1]+3; i++ {
					code.Code[i] = byte(core.OP_NOP)
				}
				// Discard both the initialized inner result and the outer
				// call's enclosing argument before returning its uninitialized
				// receiver. NOP/POP alone return a different reference.
				code.Code[calls[1]] = byte(core.OP_POP2)
			case "wrong local category":
				code.Code[parameterPC] = byte(core.OP_LLOAD_1)
			case "small locals":
				code.MaxLocals = 1
			case "small stack":
				code.MaxStack = 1
			case "oversize frames":
				code.MaxLocals, code.MaxStack = 65535, 65535
			case "bad handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: uint16(len(code.Code) + 1), HandlerPc: 0}}
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			bindings, accepted := d.nativeMemberAllocationInvocations(method, code)
			if accepted != (variant == "original") {
				t.Fatalf("original typed allocation accepted=%v code=%x new=%v calls=%v bindings=%+v", accepted, code.Code, news, calls, bindings)
			}
			if accepted && (len(bindings) != 2 || bindings[news[0]].pc != calls[1] || bindings[news[1]].pc != calls[0]) {
				t.Fatalf("outer/inner same-type identity swapped: %+v", bindings)
			}
		})
	}
}
