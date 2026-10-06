package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeMemberFreshEnclosingRequiresOriginalInitializedSites(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, `class FreshSiteOwner {class Branch {Branch(){}Branch(long n){}class Leaf {Leaf(long n,double d){}}}Object make(long n){return new Branch().new Leaf(n,4.0);}Object local(Branch b,long n){return b.new Leaf(n,4.0);}Object branched(long n){return new Branch(n>0?1L:2L).new Leaf(n,4.0);}}`, debug)
			for _, variant := range []string{"original", "nullable parameter", "branched qualifier", "wrong allocation owner", "uninitialized qualifier", "missing qualifier dup", "small stack", "small locals", "oversize frames", "bad handler", "budget", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					obj, e := Parse(append([]byte(nil), files["FreshSiteOwner.class"]...))
					if e != nil {
						t.Fatal(e)
					}
					name := "make"
					if variant == "nullable parameter" {
						name = "local"
					}
					if variant == "branched qualifier" {
						name = "branched"
					}
					var method *MemberInfo
					var code *CodeAttribute
					for _, m := range obj.Methods {
						n, _ := sourceBridgeUTF8(obj, m.NameIndex)
						if n == name {
							method = m
							for _, a := range m.Attributes {
								if c, ok := a.(*CodeAttribute); ok {
									code = c
								}
							}
						}
					}
					if method == nil || code == nil {
						t.Fatal("original method")
					}
					decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
					if e := decoder.ParseOpcode(); e != nil {
						t.Fatal(e)
					}
					target, fresh, init, dup := -1, -1, -1, -1
					ops := constructorMotionOps(decoder)
					for i, op := range ops {
						if op.Instr.OpCode == core.OP_NEW {
							owner, _ := sourceBridgeClassName(obj, core.Convert2bytesToInt(op.Data))
							if owner == "FreshSiteOwner$Branch$Leaf" {
								target = int(op.CurrentOffset)
							}
							if owner == "FreshSiteOwner$Branch" {
								fresh = int(op.CurrentOffset)
								if i+1 < len(ops) {
									dup = int(ops[i+1].CurrentOffset)
								}
							}
						}
						if call := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL); call != nil && call.Member == "<init>" && call.Name == "FreshSiteOwner$Branch" {
							init = int(op.CurrentOffset)
						}
					}
					if target < 0 || variant != "nullable parameter" && (fresh < 0 || init < 0 || dup < 0) {
						t.Fatal("original distinct NEW sites")
					}
					d := NewClassObjectDumper(obj)
					switch variant {
					case "wrong allocation owner":
						code.Code[fresh+1], code.Code[fresh+2] = byte(obj.ThisClass>>8), byte(obj.ThisClass)
					case "uninitialized qualifier":
						for i := init; i < init+3; i++ {
							code.Code[i] = byte(core.OP_NOP)
						}
					case "missing qualifier dup":
						code.Code[dup] = byte(core.OP_NOP)
					case "small stack":
						code.MaxStack = 1
					case "small locals":
						code.MaxLocals = 1
					case "oversize frames":
						code.MaxStack, code.MaxLocals = 65535, 65535
					case "bad handler":
						code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: uint16(len(code.Code) + 1), HandlerPc: uint16(fresh)}}
					case "budget":
						d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						d.Work = workbudget.New(ctx, workbudget.Limits{})
					}
					bindings, known := d.nativeMemberAllocationInvocations(method, code)
					if variant == "nullable parameter" || variant == "branched qualifier" {
						if !known || bindings[target].enclosing != nil {
							t.Fatal("nullable parameter or alternate entry gained fresh NEW proof")
						}
						return
					}
					if known != (variant == "original") {
						t.Fatalf("typed snapshot known=%v bindings=%+v", known, bindings)
					}
					if known {
						w := bindings[target].enclosing
						if w == nil || w.newPC != fresh || w.invokePC != init || w.owner != "FreshSiteOwner$Branch" || w.descriptor != "(LFreshSiteOwner;)V" {
							t.Fatalf("physical enclosing identity %+v", w)
						}
					}
				})
			}
		})
	}
}

func TestNativeMemberFreshEnclosingSourceRejectsForgedOrigins(t *testing.T) {
	for _, variant := range []string{"direct", "wrong allocation PC", "missing allocation PC", "wrong init PC", "missing init PC", "wrong owner", "wrong descriptor", "wrong source type", "array", "missing call", "wrong member", "wrong invoke kind", "not special", "different receiver", "mutable alias", "nullable parameter", "nil value", "wrapper depth", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			proof := &nativeMemberFreshEnclosing{newPC: 4, invokePC: 9, owner: "proof/Branch", descriptor: "(Lproof/Owner;)V"}
			n := &values.NewExpression{JavaType: types.NewJavaClass("proof.Branch"), HasOriginPC: true, OriginPC: 4}
			call := &values.FunctionCallExpression{ClassName: "proof.Branch", FunctionName: "<init>", Descriptor: proof.descriptor, Kind: values.InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: 9, Object: n}
			n.ConstructorCall = call
			var value any = n
			var work *workbudget.Budget
			switch variant {
			case "wrong allocation PC":
				n.OriginPC++
			case "missing allocation PC":
				n.HasOriginPC = false
			case "wrong init PC":
				call.OriginPC++
			case "missing init PC":
				call.HasOriginPC = false
			case "wrong owner":
				call.ClassName = "proof.Other"
			case "wrong descriptor":
				call.Descriptor = "()V"
			case "wrong source type":
				n.JavaType = types.NewJavaClass("proof.Other")
			case "array":
				n.JavaType = types.NewJavaArrayType(types.NewJavaClass("proof.Branch"))
			case "missing call":
				n.ConstructorCall = nil
			case "wrong member":
				call.FunctionName = "factory"
			case "wrong invoke kind":
				call.Kind = values.InvokeStatic
			case "not special":
				call.IsSpecialInvoke = false
			case "different receiver":
				copy := *n
				call.Object = &copy
			case "mutable alias", "nullable parameter":
				ref := values.NewJavaRef(utils.NewRootVariableId(), n, n.Type())
				ref.IsParam = variant == "nullable parameter"
				value = ref
			case "nil value":
				value = (*values.NewExpression)(nil)
			case "wrapper depth":
				var v values.JavaValue = n
				for i := 0; i < 40; i++ {
					v = values.NewSlotValue(v, n.Type())
				}
				value = v
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if accepted := nativeMemberFreshEnclosingOperand(value, proof, work); accepted != (variant == "direct") {
				t.Fatalf("source origin accepted=%v", accepted)
			}
		})
	}
}
