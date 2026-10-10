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

func TestNativeAnonymousInitializerVoidStatementRequiresOriginalInvocation(t *testing.T) {
	files := nativeCompileClasses(t, orderedVoidInitializerFixture)
	obj, err := Parse(files["OrderedVoidOwner$1.class"])
	if err != nil {
		t.Fatal(err)
	}
	var code *CodeAttribute
	for _, m := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if name == "<init>" {
			for _, a := range m.Attributes {
				if c, ok := a.(*CodeAttribute); ok {
					code = c
				}
			}
		}
	}
	if code == nil {
		t.Fatal("original constructor")
	}
	d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	byPC := map[int]*core.OpCode{}
	pc := -1
	for _, op := range constructorMotionOps(d) {
		byPC[int(op.CurrentOffset)] = op
		if f := constructorMotionMember(obj, op, core.OP_INVOKESTATIC); f != nil && f.Member == "finish" {
			pc = int(op.CurrentOffset)
		}
	}
	if pc < 0 {
		t.Fatal("original void invoke")
	}
	for _, change := range []string{"original", "no invoke witness", "wrong PC", "wrong owner", "wrong member", "wrong descriptor", "constructor", "nonvoid", "wrong return view", "missing function type", "wrong parameter view", "static flag mismatch", "wrong invoke kind", "wrong static receiver", "evaluated class receiver", "extra argument", "cyclic argument", "forwarding root", "noncall expression", "nil call", "nil statement", "nil child", "nil plan", "nil events", "budget", "memory", "canceled"} {
		t.Run(change, func(t *testing.T) {
			mt, err := types.ParseMethodDescriptor("()V")
			if err != nil {
				t.Fatal(err)
			}
			call := &values.FunctionCallExpression{ClassName: "OrderedVoidEffects", FunctionName: "finish", Descriptor: "()V", Kind: values.InvokeStatic, IsStatic: true, OriginPC: pc, HasOriginPC: true, FuncType: mt.FunctionType(), Object: values.NewJavaClassValue(types.NewJavaClass("OrderedVoidEffects"))}
			stmt := &statements.ExpressionStatement{Expression: call}
			child := &nativeAnonymousClass{object: obj}
			plan := &nativeAnonymousExpressionInitializer{byPC: byPC}
			events := []int{}
			ep := &events
			var work *workbudget.Budget
			switch change {
			case "no invoke witness":
				call.HasOriginPC = false
			case "wrong PC":
				call.OriginPC = -1
			case "wrong owner":
				call.ClassName = "Unknown"
			case "wrong member":
				call.FunctionName = "other"
			case "wrong descriptor":
				call.Descriptor = "(I)V"
				call.Arguments = []values.JavaValue{values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))}
				call.FuncType.ParamTypes = []types.JavaType{types.NewJavaPrimer(types.JavaInteger)}
			case "constructor":
				call.FunctionName = "<init>"
			case "nonvoid":
				call.Descriptor = "()I"
			case "wrong return view":
				call.FuncType.ReturnType = types.NewJavaPrimer(types.JavaInteger)
			case "missing function type":
				call.FuncType = nil
			case "wrong parameter view":
				call.FuncType.ParamTypes = []types.JavaType{types.NewJavaPrimer(types.JavaInteger)}
			case "static flag mismatch":
				call.IsStatic = false
			case "wrong invoke kind":
				call.Kind = values.InvokeVirtual
				call.IsStatic = false
			case "wrong static receiver":
				call.Object = values.NewJavaClassValue(types.NewJavaClass("Other"))
			case "evaluated class receiver":
				call.Object.(*values.JavaClassValue).HasOriginPC = true
			case "extra argument":
				call.Arguments = []values.JavaValue{call}
			case "cyclic argument":
				call.Descriptor = "(Ljava/lang/Object;)V"
				call.FuncType.ParamTypes = []types.JavaType{types.NewJavaClass("java.lang.Object")}
				call.Arguments = []values.JavaValue{call}
			case "forwarding root":
				stmt.Expression = values.NewSlotValue(call, types.NewJavaPrimer(types.JavaVoid))
			case "noncall expression":
				stmt.Expression = values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))
			case "nil call":
				stmt.Expression = (*values.FunctionCallExpression)(nil)
			case "nil statement":
				stmt = nil
			case "nil child":
				child = nil
			case "nil plan":
				plan = nil
			case "nil events":
				ep = nil
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := nativeAnonymousInitializerVoidStatement(child, plan, stmt, ep, nil, work, nil, nil)
			if got != (change == "original") {
				t.Fatalf("void statement accepted=%v", got)
			}
			if got && (len(events) != 1 || events[0] != pc) {
				t.Fatalf("original void invoke event=%v", events)
			}
		})
	}
}
