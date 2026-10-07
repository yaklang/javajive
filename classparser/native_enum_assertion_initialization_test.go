package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeEnumAssertionInitializationRequiresBothOriginalProtocols(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileDebugClasses(t, enumAssertionInitializationFixture, debug)
		for _, variant := range []string{"original", "no resolver", "missing owner", "wrong owner object", "owner cycle", "wrong status owner", "wrong status invoke", "wrong negation", "duplicate flag", "mutable flag", "unknown synthetic field", "missing assertion read", "prefix handler", "prefix reentry", "harmless nop", "intervening effect", "wrong status CP tag", "wrong store CP tag", "later flag write", "wrong ordinal", "wrong backing array", "budget", "canceled"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				objects := map[string]*ClassObject{}
				for _, data := range files {
					obj, err := Parse(append([]byte(nil), data...))
					if err != nil {
						t.Fatal(err)
					}
					objects[obj.GetClassName()] = obj
				}
				obj := objects["EnumAssertionOwner$Mode"]
				if obj == nil {
					t.Fatal("original enum")
				}
				resolve := func(name string) (*ClassObject, bool) { o, ok := objects[name]; return o, ok }
				_, _, flags, known := originalMemberOwner(obj)
				if !known {
					t.Fatal("original member declaration")
				}
				var initializer *CodeAttribute
				var flag *MemberInfo
				var check *CodeAttribute
				for _, f := range obj.Fields {
					n, _ := sourceBridgeUTF8(obj, f.NameIndex)
					if n == nativeAssertionField {
						flag = f
					}
				}
				for _, m := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							if n == "<clinit>" {
								initializer = c
							}
							if n == "mask" {
								check = c
							}
						}
					}
				}
				if initializer == nil || check == nil || flag == nil {
					t.Fatal("original assertion protocol")
				}
				var work *workbudget.Budget
				switch variant {
				case "no resolver":
					resolve = nil
				case "missing owner":
					delete(objects, "EnumAssertionOwner")
				case "wrong owner object":
					objects["EnumAssertionOwner"] = obj
				case "owner cycle":
					objects["EnumAssertionOwner"].Attributes = obj.Attributes
				case "wrong status owner":
					initializer.Code[1] = byte(obj.ThisClass)
				case "wrong status invoke":
					initializer.Code[2] = core.OP_INVOKESTATIC
				case "wrong negation":
					initializer.Code[8] = core.OP_ICONST_0
				case "duplicate flag":
					obj.Fields = append(obj.Fields, flag)
				case "mutable flag":
					flag.AccessFlags &^= 16
				case "unknown synthetic field":
					copy := *flag
					copy.NameIndex = obj.Fields[0].NameIndex
					obj.Fields = append(obj.Fields, &copy)
				case "missing assertion read":
					for i, m := range obj.Methods {
						n, _ := sourceBridgeUTF8(obj, m.NameIndex)
						if n == "mask" {
							obj.Methods = append(obj.Methods[:i], obj.Methods[i+1:]...)
							break
						}
					}
				case "prefix handler":
					initializer.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 16, HandlerPc: 16}}
				case "prefix reentry":
					initializer.Code = append(initializer.Code[:len(initializer.Code)-1], core.OP_GOTO, 0, 0)
					pc := len(initializer.Code) - 3
					offset := uint16(-pc)
					initializer.Code[pc+1] = byte(offset >> 8)
					initializer.Code[pc+2] = byte(offset)
				case "harmless nop":
					initializer.Code = append(append(append([]byte{}, initializer.Code[:16]...), core.OP_NOP), initializer.Code[16:]...)
				case "intervening effect":
					var effect []byte
					decoder := core.NewDecompiler(initializer.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
					if err := decoder.ParseOpcode(); err != nil {
						t.Fatal(err)
					}
					for _, op := range constructorMotionOps(decoder) {
						if nativeEnumMemberOperand(obj, op, core.OP_INVOKESTATIC, "EnumAssertionEffects", "finish", "()J") {
							effect = append([]byte{core.OP_INVOKESTATIC}, op.Data...)
							effect = append(effect, core.OP_POP2)
						}
					}
					if len(effect) != 4 {
						t.Fatal("original suffix effect")
					}
					initializer.Code = append(append(append([]byte{}, initializer.Code[:16]...), effect...), initializer.Code[16:]...)
				case "wrong status CP tag":
					index := uint16(initializer.Code[3])<<8 | uint16(initializer.Code[4])
					ref, ok := obj.ConstantPool[index-1].(*ConstantMethodrefInfo)
					if !ok {
						t.Fatal("original status CP tag")
					}
					obj.ConstantPool[index-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
				case "wrong store CP tag":
					index := uint16(initializer.Code[14])<<8 | uint16(initializer.Code[15])
					ref, ok := obj.ConstantPool[index-1].(*ConstantFieldrefInfo)
					if !ok {
						t.Fatal("original store CP tag")
					}
					obj.ConstantPool[index-1] = &ConstantMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
				case "later flag write":
					check.Code[0] = core.OP_PUTSTATIC
				case "wrong ordinal":
					initializer.Code[22] = core.OP_ICONST_1
				case "wrong backing array":
					for _, f := range obj.Fields {
						n, _ := sourceBridgeUTF8(obj, f.NameIndex)
						if n == "$VALUES" {
							f.AccessFlags &^= 16
						}
					}
				case "budget":
					work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				plan := nativeMemberEnumSynthesisWithDeclarations(obj, flags, resolve, work)
				if (plan != nil) != (variant == "original" || variant == "harmless nop") {
					t.Fatalf("composed protocol admitted=%v", plan != nil)
				}
				if plan != nil && (plan.assertions == nil || plan.assertions.pureInitializer || plan.assertions.statusOwner != "EnumAssertionOwner" || plan.assertions.initializerEndPC != 16 && variant != "harmless nop") {
					t.Fatalf("missing independent assertion prefix: %+v", plan.assertions)
				}
			})
		}
	}
}

func TestNativeEnumAssertionInitializerSourceProjectionRequiresExactOccurrence(t *testing.T) {
	boolType := types.NewJavaPrimer(types.JavaBoolean)
	for _, variant := range []string{"original", "ternary", "equality", "missing", "wrong store PC", "no store PC", "declaration", "wrong field", "wrong descriptor", "wrong owner", "field handle", "wrong call PC", "no call PC", "wrong invoke kind", "wrong method", "wrong call descriptor", "wrong status class", "no class origin", "wrong class origin", "not inverted", "constant replacement", "duplicate status evaluation", "opaque", "cycle", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj := &ClassObject{}
			// The name is obtained from the original constant pool, never a source alias.
			obj.ConstantPool = []ConstantInfo{&ConstantUtf8Info{Value: "p/Owner$Choice"}, &ConstantClassInfo{NameIndex: 1}}
			obj.ThisClass = 2
			d := NewClassObjectDumper(obj)
			plan := &nativeMemberAssertion{initializerStorePC: 13, statusInvokePC: 2, statusOwner: "p/Owner"}
			field := &values.JavaClassMember{Name: "p.Owner$Choice", Member: nativeAssertionField, Description: "Z", JavaType: boolType}
			classLiteral := &values.JavaClassValue{JavaType: types.NewJavaClass("p.Owner"), OriginPC: 0, HasOriginPC: true}
			call := &values.FunctionCallExpression{FuncType: &types.JavaFuncType{ReturnType: boolType}, Object: classLiteral, ClassName: "java.lang.Class", FunctionName: "desiredAssertionStatus", Descriptor: "()Z", Kind: values.InvokeVirtual, OriginPC: 2, HasOriginPC: true}
			value := values.NewUnaryExpression(call, values.Not, boolType)
			assignment := &statements.AssignStatement{LeftValue: field, JavaValue: value, OriginPC: 13, HasOriginPC: true}
			suffix := &statements.ReturnStatement{}
			body := []statements.Statement{assignment, suffix}
			switch variant {
			case "ternary":
				assignment.JavaValue = &values.TernaryExpression{Condition: call, TrueValue: values.NewJavaLiteral(false, boolType), FalseValue: values.NewJavaLiteral(true, boolType)}
			case "equality":
				assignment.JavaValue = values.NewBinaryExpression(call, values.NewJavaLiteral(false, boolType), values.EQ, boolType)
			case "missing":
				body = nil
			case "wrong store PC":
				assignment.OriginPC++
			case "no store PC":
				assignment.HasOriginPC = false
			case "declaration":
				assignment.IsDeclare = true
			case "wrong field":
				field.Member = "custom"
			case "wrong descriptor":
				field.Description = "I"
			case "wrong owner":
				field.Name = "p.Other"
			case "field handle":
				field.RefKind = 2
			case "wrong call PC":
				call.OriginPC++
			case "no call PC":
				call.HasOriginPC = false
			case "wrong invoke kind":
				call.Kind = values.InvokeStatic
			case "wrong method":
				call.FunctionName = "custom"
			case "wrong call descriptor":
				call.Descriptor = "()I"
			case "wrong status class":
				classLiteral.JavaType = types.NewJavaClass("p.Other")
			case "no class origin":
				classLiteral.HasOriginPC = false
			case "wrong class origin":
				classLiteral.OriginPC++
			case "not inverted":
				assignment.JavaValue = call
			case "constant replacement":
				assignment.JavaValue = values.NewJavaLiteral(true, boolType)
			case "duplicate status evaluation":
				assignment.JavaValue = values.NewBinaryExpression(call, values.NewUnaryExpression(call, values.Not, boolType), values.NEQ, boolType)
			case "opaque":
				assignment.JavaValue = &values.CustomValue{}
			case "cycle":
				value.Values[0] = value
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got, ok := d.projectNativeAssertionInitializer(body, plan)
			accepted := variant == "original" || variant == "ternary" || variant == "equality"
			if ok != accepted {
				t.Fatalf("source projection admitted=%v", ok)
			}
			if ok && (len(got) != 1 || got[0] != suffix) {
				t.Fatal("ordinary initialization suffix changed")
			}
		})
	}
}
