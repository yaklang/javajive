package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeAnonymousClassLiteralRequiresOriginalSubjectAndPC(t *testing.T) {
	for _, name := range []string{"example/Subject", "[Lexample/Subject;", "[[I"} {
		for _, change := range []string{"original", "wide LDC", "different subject", "wrong PC", "mismatched op PC", "different opcode", "LDC2", "operand length", "no instruction", "no object", "old version", "bad index", "nil constant", "nonclass constant", "malformed class", "malformed array", "void array", "excessive rank", "missing type", "parameterized subject", "array subject cycle", "budget", "memory", "canceled"} {
			t.Run(name+"/"+change, func(t *testing.T) {
				descriptor := name
				if name[0] != '[' {
					descriptor = "L" + name + ";"
				}
				typ, err := types.ParseDescriptor(descriptor)
				if err != nil {
					t.Fatal(err)
				}
				literal := values.NewJavaClassValue(typ)
				literal.OriginPC, literal.HasOriginPC = 15, true
				obj := &ClassObject{MajorVersion: 52, ConstantPool: []ConstantInfo{&ConstantUtf8Info{Value: name}, &ConstantClassInfo{NameIndex: 1}}}
				op := &core.OpCode{CurrentOffset: 15, Instr: &core.Instruction{OpCode: core.OP_LDC}, Data: []byte{2}}
				plan := &nativeAnonymousExpressionInitializer{byPC: map[int]*core.OpCode{15: op}}
				var work *workbudget.Budget
				switch change {
				case "wide LDC":
					op.Instr.OpCode = core.OP_LDC_W
					op.Data = []byte{0, 2}
				case "different subject":
					literal.JavaType = types.NewJavaClass("example.Other")
				case "wrong PC":
					literal.OriginPC = 16
				case "mismatched op PC":
					op.CurrentOffset = 14
				case "different opcode":
					op.Instr.OpCode = core.OP_GETSTATIC
				case "LDC2":
					op.Instr.OpCode = core.OP_LDC2_W
					op.Data = []byte{0, 2}
				case "operand length":
					op.Data = []byte{0, 2}
				case "no instruction":
					op.Instr = nil
				case "no object":
					obj = nil
				case "old version":
					obj.MajorVersion = 48
				case "bad index":
					op.Data = []byte{99}
				case "nil constant":
					obj.ConstantPool[1] = (*ConstantClassInfo)(nil)
				case "nonclass constant":
					obj.ConstantPool[1] = &ConstantIntegerInfo{Value: 2}
				case "malformed class":
					obj.ConstantPool[0].(*ConstantUtf8Info).Value = "example/Bad;Name"
				case "malformed array":
					obj.ConstantPool[0].(*ConstantUtf8Info).Value = "[Lexample/Subject"
				case "void array":
					obj.ConstantPool[0].(*ConstantUtf8Info).Value = "[V"
				case "excessive rank":
					obj.ConstantPool[0].(*ConstantUtf8Info).Value = strings.Repeat("[", 256) + "I"
				case "missing type":
					literal.JavaType = nil
				case "parameterized subject":
					literal.JavaType = types.NewParameterizedType("example.Subject", []types.JavaType{types.NewJavaClass("java.lang.String")})
				case "array subject cycle":
					array := types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))
					array.RawType().(*types.JavaArrayType).JavaType = array
					literal.JavaType = array
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
				case "memory":
					work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				events := []int{}
				known := nativeAnonymousInitializerExpressionEvents(&nativeAnonymousClass{object: obj}, plan, literal, &events, nil, work)
				expected := change == "original" || change == "wide LDC"
				if known != expected {
					t.Fatalf("accepted=%v events=%v", known, events)
				}
				if known && (len(events) != 1 || events[0] != 15) {
					t.Fatalf("lost original class resolution %v", events)
				}
			})
		}
	}
}

// Static call owners are rendering metadata. Assigning an evaluated class
// literal to that slot must not pay for a discarded original LDC event.
func TestNativeAnonymousClassLiteralCannotBecomeStaticInvocationOwner(t *testing.T) {
	for _, change := range []string{"symbolic owner", "evaluated literal", "static flag mismatch"} {
		t.Run(change, func(t *testing.T) {
			obj := &ClassObject{MajorVersion: 52, ConstantPool: []ConstantInfo{
				&ConstantUtf8Info{Value: "example/Subject"}, &ConstantClassInfo{NameIndex: 1},
				&ConstantUtf8Info{Value: "read"}, &ConstantUtf8Info{Value: "()Ljava/lang/Class;"},
				&ConstantNameAndTypeInfo{NameIndex: 3, DescriptorIndex: 4},
				&ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: 2, NameAndTypeIndex: 5}},
			}}
			class := values.NewJavaClassValue(types.NewJavaClass("example.Subject"))
			class.OriginPC = 15
			class.HasOriginPC = change != "symbolic owner"
			method, err := types.ParseMethodDescriptor("()Ljava/lang/Class;")
			if err != nil {
				t.Fatal(err)
			}
			call := &values.FunctionCallExpression{Object: class, ClassName: "example.Subject", FunctionName: "read", Descriptor: "()Ljava/lang/Class;", Kind: values.InvokeStatic, IsStatic: change != "static flag mismatch", OriginPC: 18, HasOriginPC: true, FuncType: method.FunctionType()}
			plan := &nativeAnonymousExpressionInitializer{byPC: map[int]*core.OpCode{
				15: {CurrentOffset: 15, Instr: &core.Instruction{OpCode: core.OP_LDC}, Data: []byte{2}},
				18: {CurrentOffset: 18, Instr: &core.Instruction{OpCode: core.OP_INVOKESTATIC}, Data: []byte{0, 6}},
			}}
			events := []int{}
			known := nativeAnonymousInitializerExpressionEvents(&nativeAnonymousClass{object: obj}, plan, call, &events, nil, nil)
			if known != (change == "symbolic owner") {
				t.Fatalf("metadata borrowed evaluated literal: accepted=%v events=%v", known, events)
			}
			if known && (len(events) != 1 || events[0] != 18) {
				t.Fatalf("symbolic owner gained class resolution: %v", events)
			}
		})
	}
}
