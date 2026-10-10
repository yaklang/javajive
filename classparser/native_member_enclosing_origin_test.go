package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMemberLexicalEnclosingOriginRejectsForeignAndUnstableValues(t *testing.T) {
	files := nativeCompileClasses(t, `class OriginOwner{class Destination{}class Actor{Destination make(){return new Destination();}}}`)
	for _, scenario := range []string{"direct", "unique local", "same ID copy", "mutable Val ignored", "wrong mutable Val cannot rescue", "other receiver", "computed receiver", "missing field origin", "wrong field origin", "wrong field", "missing family", "foreign current identity", "foreign owner", "static current", "no declaration", "later declaration", "branch declaration escapes", "local reassigned", "parameter alias", "call origin mismatch", "duplicate source call", "opaque value", "value cycle", "statement cycle", "nil field", "budget", "canceled", "depth"} {
		t.Run(scenario, func(t *testing.T) {
			root, _ := Parse(files["OriginOwner.class"])
			d := NewClassObjectDumper(root)
			d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
			family := d.planNativeMemberFamily()
			if family == nil {
				t.Fatal("original family proof")
			}
			current := "OriginOwner$Actor"
			actor := family.children[current]
			obj := actor.object
			reader := NewClassObjectDumper(obj)
			plans, ok := reader.nativeMemberAllocations(family)
			if !ok {
				t.Fatal("original allocation proof")
			}
			var plan *nativeMemberAllocation
			for _, p := range plans["make()LOriginOwner$Destination;"] {
				plan = p
			}
			if plan == nil || plan.enclosingReadPC < 0 {
				t.Fatal("original enclosing read")
			}
			receiver := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass(current))
			receiver.IsThis = true
			field := values.NewRefMember(receiver, actor.field, types.NewJavaClass(actor.owner))
			field.HasOriginPC = true
			field.OriginPC = plan.enclosingReadPC
			local := values.NewJavaRef(coreutils.NewRootVariableId(), nil, field.Type())
			local.Id.SetName("outer")
			declaration := statements.NewAssignStatement(local, field, true)
			call := &values.FunctionCallExpression{ClassName: plan.child.object.GetClassName(), FunctionName: "<init>", Descriptor: plan.descriptor, HasOriginPC: true, OriginPC: plan.invokePC, Arguments: []values.JavaValue{local}}
			allocation := &values.NewExpression{JavaType: types.NewJavaClass(call.ClassName), ConstructorCall: call, HasOriginPC: true, OriginPC: plan.newPC}
			use := &statements.ReturnStatement{JavaValue: allocation}
			body := []statements.Statement{declaration, use}
			var operand any = local
			var work *workbudget.Budget
			good := scenario == "direct" || scenario == "unique local" || scenario == "same ID copy" || scenario == "mutable Val ignored"
			switch scenario {
			case "direct":
				operand = field
			case "same ID copy":
				copy := *local
				operand = &copy
			case "mutable Val ignored":
				local.Val = values.JavaNull
			case "wrong mutable Val cannot rescue":
				local.Val = field
				declaration.JavaValue = values.JavaNull
			case "other receiver":
				receiver.IsThis = false
			case "computed receiver":
				field.Object = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not execute opaque value") }, func() types.JavaType { return receiver.Type() })
			case "missing field origin":
				field.HasOriginPC = false
			case "wrong field origin":
				field.OriginPC++
			case "wrong field":
				field.Member = "different"
			case "missing family":
				family = nil
			case "foreign current identity":
				actor.object = root
			case "foreign owner":
				actor.owner = "Different"
			case "static current":
				actor.static = true
			case "no declaration":
				body = []statements.Statement{use}
			case "later declaration":
				body = []statements.Statement{use, declaration}
			case "branch declaration escapes":
				body = []statements.Statement{&statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{declaration}}, use}
			case "local reassigned":
				body = append(body, statements.NewAssignStatement(local, values.JavaNull, false))
			case "parameter alias":
				local.IsParam = true
			case "call origin mismatch":
				call.OriginPC++
			case "duplicate source call":
				body = append(body, use)
			case "opaque value":
				body = append(body, &statements.ExpressionStatement{Expression: values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not execute opaque value") }, func() types.JavaType { return receiver.Type() })})
			case "value cycle":
				cycle := &values.JavaExpression{}
				cycle.Values = []values.JavaValue{cycle}
				body = append(body, &statements.ExpressionStatement{Expression: cycle})
			case "statement cycle":
				cycle := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))}
				cycle.IfBody = []statements.Statement{cycle}
				body = append(body, cycle)
			case "nil field":
				operand = (*values.RefMember)(nil)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "depth":
				work = workbudget.New(nil, workbudget.Limits{MaxASTDepth: 1})
			}
			if got := nativeMemberLexicalEnclosingOperand(operand, plan, family, current, body, work); got != good {
				t.Fatalf("lexical origin proof=%v want%v", got, good)
			}
		})
	}
}

func TestNativeMemberEnclosingTypeUsesOriginalFormalDeclarationIdentity(t *testing.T) {
	files := nativeCompileClasses(t, `class FormalOwner<K>{class Open<Z>{void plain(){}<K>void shadow(){}}class Closed<K>{}}`)
	root, _ := Parse(files["FormalOwner.class"])
	for _, scenario := range []string{"original", "method shadow", "class shadow", "wrong owner", "wrong formal", "missing outer context", "budget", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			name := "FormalOwner$Open"
			method := "plain"
			if scenario == "class shadow" {
				name = "FormalOwner$Closed"
				method = "<init>"
			}
			if scenario == "method shadow" {
				method = "shadow"
			}
			obj, _ := Parse(files[name+".class"])
			c := NewClassObjectDumper(obj)
			c.FuncCtx = &class_context.ClassContext{ClassName: obj.GetClassName()}
			c.FuncCtx.FunctionName = method
			c.FuncCtx.CurrentMethodDesc = "()V"
			c.nativeOuterContext = &class_context.ClassContext{ClassTypeParams: []string{"K"}}
			c.nativeMemberCurrent = nativeMemberProofWithOwner(obj, root, nil, c.buildInvocationMetadata())
			if c.nativeMemberCurrent == nil {
				t.Fatal("original complete member proof")
			}
			view := types.NewParameterizedType("FormalOwner", []types.JavaType{types.NewJavaClass("K")})
			switch scenario {
			case "wrong owner":
				view = types.NewParameterizedType("Wrong", []types.JavaType{types.NewJavaClass("K")})
			case "wrong formal":
				view = types.NewParameterizedType("FormalOwner", []types.JavaType{types.NewJavaClass("Z")})
			case "missing outer context":
				c.nativeOuterContext = nil
			case "budget":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberEnclosingTypeDenotable(c, view); got != (scenario == "original") {
				t.Fatalf("enclosing formal identity=%v", got)
			}
		})
	}
}
