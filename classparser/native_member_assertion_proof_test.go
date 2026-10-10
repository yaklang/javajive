package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMemberAssertionRequiresOriginalCompilerProtocol(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileDebugClasses(t, nativeMemberAssertionFixture, debug)
		for _, variant := range []string{"original", "ordinary flag", "mutable flag", "private flag", "duplicate flag", "wrong descriptor", "field attribute", "no initializer", "duplicate initializer", "static initializer flags", "stack", "locals", "initializer handler", "partial assertion handler", "assertion handler entry", "status kind", "wrong status owner", "invert disabled value", "wrong join", "extra effect", "flag write outside initializer", "read branch sense", "wrong allocation", "wrong constructor", "methodhandle", "version", "budget", "canceled"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				obj, err := Parse(files["MemberAssertionOwner$Child.class"])
				if err != nil {
					t.Fatal(err)
				}
				var flag, init *MemberInfo
				var clinit, check *CodeAttribute
				for _, f := range obj.Fields {
					n, _ := sourceBridgeUTF8(obj, f.NameIndex)
					if n == nativeAssertionField {
						flag = f
					}
				}
				for _, m := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					for _, a := range m.Attributes {
						if code, ok := a.(*CodeAttribute); ok {
							if n == "<clinit>" {
								init = m
								clinit = code
							}
							if n == "check" {
								check = code
							}
						}
					}
				}
				if flag == nil || init == nil || clinit == nil || check == nil {
					t.Fatal("original protocol")
				}
				var work *workbudget.Budget
				switch variant {
				case "ordinary flag":
					flag.AccessFlags &^= 0x1000
				case "mutable flag":
					flag.AccessFlags &^= 16
				case "private flag":
					flag.AccessFlags |= 2
				case "duplicate flag":
					obj.Fields = append(obj.Fields, flag)
				case "wrong descriptor":
					flag.DescriptorIndex = init.DescriptorIndex
				case "field attribute":
					flag.Attributes = append(flag.Attributes, &DeprecatedAttribute{})
				case "no initializer":
					for i, m := range obj.Methods {
						if m == init {
							obj.Methods = append(obj.Methods[:i], obj.Methods[i+1:]...)
							break
						}
					}
				case "duplicate initializer":
					obj.Methods = append(obj.Methods, init)
				case "static initializer flags":
					init.AccessFlags |= 0x1000
				case "stack":
					clinit.MaxStack++
				case "locals":
					clinit.MaxLocals++
				case "initializer handler":
					clinit.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 2, HandlerPc: 16}}
				case "partial assertion handler":
					check.ExceptionTable = []*ExceptionTableEntry{{StartPc: 17, EndPc: 33, HandlerPc: 33}}
				case "assertion handler entry":
					check.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 33, HandlerPc: 17}}
				case "status kind":
					clinit.Code[2] = core.OP_INVOKESTATIC
				case "wrong status owner":
					clinit.Code[1] = byte(obj.ThisClass)
				case "invert disabled value":
					clinit.Code[8] = core.OP_ICONST_0
				case "wrong join":
					clinit.Code[11] = 3
				case "extra effect":
					clinit.Code = append([]byte{core.OP_ACONST_NULL, core.OP_POP}, clinit.Code...)
				case "flag write outside initializer":
					check.Code[0] = core.OP_PUTSTATIC
				case "read branch sense":
					check.Code[3] = core.OP_IFEQ
				case "wrong allocation":
					check.Code[18] = byte(obj.ThisClass >> 8)
					check.Code[19] = byte(obj.ThisClass)
				case "wrong constructor":
					check.Code[29] = core.OP_INVOKEVIRTUAL
				case "methodhandle":
					idx := uint16(check.Code[1])<<8 | uint16(check.Code[2])
					obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 2, ReferenceIndex: idx})
				case "version":
					obj.MajorVersion = 53
				case "budget":
					work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				p, ok := nativeMemberAssertionProof(obj, "MemberAssertionOwner", work)
				if (p != nil && ok) != (variant == "original") {
					t.Fatalf("protocol=%v/%v", p, ok)
				}
			})
		}
	}
}
func TestNativeMemberAssertionSourceReadClosureRejectsAmbiguity(t *testing.T) {
	boolType := types.NewJavaPrimer(types.JavaBoolean)
	for _, v := range []string{"original", "missing", "duplicated", "wrong owner", "wrong field", "wrong descriptor", "no origin", "wrong origin", "handle", "opaque", "cycle", "budget"} {
		t.Run(v, func(t *testing.T) {
			field := &values.JavaClassMember{Name: "p.Owner$Child", Member: nativeAssertionField, Description: "Z", JavaType: boolType, OriginPC: 7, HasOriginPC: true}
			var root values.JavaValue = field
			body := []statements.Statement{&statements.ReturnStatement{JavaValue: root}}
			var work *workbudget.Budget
			switch v {
			case "missing":
				body = nil
			case "duplicated":
				body = append(body, body[0])
			case "wrong owner":
				field.Name = "p.Other"
			case "wrong field":
				field.Member = "custom"
			case "wrong descriptor":
				field.Description = "I"
			case "no origin":
				field.HasOriginPC = false
			case "wrong origin":
				field.OriginPC++
			case "handle":
				field.RefKind = 2
			case "opaque":
				body = []statements.Statement{&statements.ReturnStatement{JavaValue: &values.CustomValue{}}}
			case "cycle":
				x := &values.JavaExpression{Op: values.Not, Typ: boolType}
				x.Values = []values.JavaValue{x}
				body = []statements.Statement{&statements.ReturnStatement{JavaValue: x}}
			case "budget":
				work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			}
			ok := nativeAssertionSourceReads(body, "p/Owner$Child", map[int]*nativeAssertionPacket{7: {}}, work)
			if ok != (v == "original") {
				t.Fatalf("read closure=%v", ok)
			}
		})
	}
}

func TestNativeMemberAssertionGuardRequiresFirstShortCircuitOperand(t *testing.T) {
	b := types.NewJavaPrimer(types.JavaBoolean)
	for _, variant := range []string{"original", "constant false packet", "nested first", "OR", "eager AND", "later flag", "positive flag", "cast flag", "foreign owner", "handle", "cycle", "depth"} {
		t.Run(variant, func(t *testing.T) {
			field := &values.JavaClassMember{Name: "p.Owner$Child", Member: nativeAssertionField, Description: "Z", JavaType: b, OriginPC: 7, HasOriginPC: true}
			flag := values.NewUnaryExpression(field, values.Not, b)
			condition := values.NewJavaLiteral(false, b)
			var v values.JavaValue = values.NewBinaryExpression(flag, condition, values.LOGICAL_AND, b)
			switch variant {
			case "constant false packet":
				v = flag
			case "nested first":
				v = values.NewBinaryExpression(v, condition, values.LOGICAL_AND, b)
			case "OR":
				v = values.NewBinaryExpression(flag, condition, values.LOGICAL_OR, b)
			case "eager AND":
				v = values.NewBinaryExpression(flag, condition, values.AND, b)
			case "later flag":
				v = values.NewBinaryExpression(condition, flag, values.LOGICAL_AND, b)
			case "positive flag":
				v = field
			case "cast flag":
				flag.Values[0] = &values.CastExpression{Value: field}
			case "foreign owner":
				field.Name = "p.Other"
			case "handle":
				field.RefKind = 2
			case "cycle":
				x := values.NewBinaryExpression(flag, condition, values.LOGICAL_AND, b)
				x.Values[0] = x
				v = x
			}
			depth := 64
			if variant == "depth" {
				depth = 1
			}
			f, _, known := nativeAssertionFailureGuard(v, "p.Owner$Child", nil, depth)
			want := variant == "original" || variant == "constant false packet" || variant == "nested first"
			if known != want || known && f != field {
				t.Fatalf("first operand=%v/%v", f, known)
			}
		})
	}
}
