package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeLambdaCaptureSpellingRequiresRetainedOriginalBinding(t *testing.T) {
	for _, mode := range []string{"snapshot", "parameter", "wrong factory", "wrong index", "different capture", "changed seed", "missing declaration", "different declaration", "missing witness", "invisible binder", "name conflict", "method reference", "negative index", "foreign method"} {
		t.Run(mode, func(t *testing.T) {
			typ := types.NewJavaClass("java.lang.Object")
			parameter := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			parameter.IsParam = true
			parameter.MarkOriginalParameter(0)
			snapshot := values.NewJavaRef(utils.NewRootVariableId(), parameter, typ)
			snapshot.MarkOriginalDynamicOperand(12, 0, parameter)
			declarationRef := *snapshot
			declarationRef.MarkOriginalDynamicOperandDeclaration(12, parameter)
			declaration := &statements.AssignStatement{IsDeclare: true, LeftValue: &declarationRef, JavaValue: parameter}
			lambda := &values.CustomValue{Flag: "lambda", HasOriginPC: true, OriginPC: 12, CapturesKnown: true, Captures: []values.JavaValue{snapshot}}
			method := &MemberInfo{}
			site := &nativeLambdaLocalCaptureSite{method: method, code: &CodeAttribute{}, pc: 12, operands: []*nativeEnumSelectorProducer{{slot: 0, result: "Ljava/lang/Object;"}}}
			c := &ClassObjectDumper{CurrentMethod: method, FuncCtx: &class_context.ClassContext{LocalNames: map[*utils.VariableId]string{}}, nativeLambdaCaptureParameterNames: map[*MemberInfo][]nativeLambdaCaptureNameDemand{method: {{site: site, index: 0, name: "observed", value: snapshot}}}}
			body := []statements.Statement{declaration, &statements.ReturnStatement{JavaValue: lambda}}
			visible := map[*utils.VariableId]bool{parameter.Id: true, snapshot.Id: true}
			params := map[*utils.VariableId]bool{parameter.Id: true}
			switch mode {
			case "parameter":
				lambda.Captures[0] = parameter
				c.nativeLambdaCaptureParameterNames[method][0].value = parameter
				body = body[1:]
			case "wrong factory":
				snapshot2 := values.NewJavaRef(utils.NewRootVariableId(), parameter, typ)
				snapshot2.MarkOriginalDynamicOperand(13, 0, parameter)
				snapshot2.MarkOriginalDynamicOperandDeclaration(13, parameter)
				*snapshot = *snapshot2
				copy := *snapshot
				declaration.LeftValue = &copy
				visible[snapshot.Id] = true
			case "wrong index":
				snapshot2 := values.NewJavaRef(utils.NewRootVariableId(), parameter, typ)
				snapshot2.MarkOriginalDynamicOperand(12, 1, parameter)
				snapshot2.MarkOriginalDynamicOperandDeclaration(12, parameter)
				*snapshot = *snapshot2
				copy := *snapshot
				declaration.LeftValue = &copy
				visible[snapshot.Id] = true
			case "different capture":
				lambda.Captures[0] = parameter
			case "changed seed":
				snapshot.Val = values.NewJavaLiteral(nil, typ)
			case "missing declaration":
				body = body[1:]
			case "different declaration":
				copy := declarationRef
				copy.Id = utils.NewRootVariableId()
				declaration.LeftValue = &copy
			case "missing witness":
				replacement := values.NewJavaRef(snapshot.Id, parameter, typ)
				*snapshot = *replacement
			case "invisible binder":
				delete(visible, snapshot.Id)
			case "name conflict":
				c.FuncCtx.LocalNames[snapshot.Id] = "other"
			case "method reference":
				lambda.IsMethodRef = true
			case "negative index":
				c.nativeLambdaCaptureParameterNames[method][0].index = -1
			case "foreign method":
				c.nativeLambdaCaptureParameterNames[method][0].site.method = &MemberInfo{}
			}
			before := snapshot.Id
			seed := snapshot.Val
			got := c.bindNativeLambdaCaptureParameterNames(lambda, body, params, visible, &nativeCaptureNameBindings{})
			want := mode == "snapshot" || mode == "parameter" || mode == "foreign method"
			if got != want {
				t.Fatalf("got%v want%v", got, want)
			}
			target := snapshot.Id
			if mode == "parameter" {
				target = parameter.Id
			}
			if got && mode != "foreign method" && c.FuncCtx.LocalNames[target] != "observed" {
				t.Fatal("actual source binder was not named")
			}
			if snapshot.Id != before || snapshot.Val != seed || lambda.Captures[0] == nil {
				t.Fatal("name constraint changed capture value/identity")
			}
		})
	}
}

// Standalone named local classes have no anonymous family. A factory spelling
// demand still needs the full source proof, and must never certify allocation.
func TestNativeLambdaFactoryBinderWithoutAnonymousFamilyFailsClosed(t *testing.T) {
	for _, mode := range []string{"snapshot", "parameter", "no demand", "missing declaration", "changed seed", "name conflict", "invisible declaration", "nil context", "statement cycle", "budget", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			typ := types.NewJavaClass("java.lang.Object")
			parameter := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			parameter.IsParam = true
			parameter.MarkOriginalParameter(0)
			snapshot := values.NewJavaRef(utils.NewRootVariableId(), parameter, typ)
			snapshot.MarkOriginalDynamicOperand(12, 0, parameter)
			declarationRef := *snapshot
			declarationRef.MarkOriginalDynamicOperandDeclaration(12, parameter)
			declaration := &statements.AssignStatement{IsDeclare: true, LeftValue: &declarationRef, JavaValue: parameter}
			lambda := &values.CustomValue{Flag: "lambda", HasOriginPC: true, OriginPC: 12, CapturesKnown: true, Captures: []values.JavaValue{snapshot}}
			method := &MemberInfo{}
			site := &nativeLambdaLocalCaptureSite{method: method, code: &CodeAttribute{}, pc: 12, operands: []*nativeEnumSelectorProducer{{slot: 0, result: "Ljava/lang/Object;"}}}
			ctx := &class_context.ClassContext{LocalNames: map[*utils.VariableId]string{}}
			family := &nativeMemberFamily{}
			c := &ClassObjectDumper{CurrentMethod: method, FuncCtx: ctx, nativeMemberRoot: family, nativeLambdaCaptureParameterNames: map[*MemberInfo][]nativeLambdaCaptureNameDemand{method: {{site: site, index: 0, name: "observed", value: snapshot}}}}
			body := []statements.Statement{declaration, &statements.ReturnStatement{JavaValue: lambda}}
			switch mode {
			case "parameter":
				lambda.Captures[0] = parameter
				c.nativeLambdaCaptureParameterNames[method][0].value = parameter
				body = body[1:]
			case "no demand":
				c.nativeLambdaCaptureParameterNames = nil
			case "missing declaration":
				body = body[1:]
			case "changed seed":
				snapshot.Val = values.NewJavaLiteral(nil, typ)
			case "name conflict":
				ctx.LocalNames[snapshot.Id] = "other"
			case "invisible declaration":
				body = []statements.Statement{&statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{declaration}}, body[1]}
			case "nil context":
				c.FuncCtx = nil
			case "statement cycle":
				cycle := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))}
				cycle.IfBody = []statements.Statement{cycle}
				body = append([]statements.Statement{cycle}, body...)
			case "budget":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				cancelCtx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Work = workbudget.New(cancelCtx, workbudget.Limits{})
			}
			seed := snapshot.Val
			id := snapshot.Id
			c.prepareNativeCaptureBindings(body, []values.JavaValue{parameter})
			valid := mode == "snapshot" || mode == "parameter" || mode == "no demand"
			if c.nativeCaptureFailed == valid || family.failed == valid {
				t.Fatalf("capture failed=%v family failed=%v expected valid=%v", c.nativeCaptureFailed, family.failed, valid)
			}
			if ctx.SourceCaptureStable != nil {
				t.Fatal("name-only proof granted allocation permission")
			}
			if c.nativeAnonymousRoot != nil || snapshot.Val != seed || snapshot.Id != id {
				t.Fatal("binder constraint changed source ownership or captured value")
			}
			if mode == "snapshot" && ctx.LocalNames[snapshot.Id] != "observed" || mode == "parameter" && ctx.LocalNames[parameter.Id] != "observed" {
				t.Fatal("retained factory binder was not constrained")
			}
		})
	}
}

func TestNativeLambdaFactoryLocalBinderReopensOriginalTypedStore(t *testing.T) {
	for _, mode := range []string{"original", "renamed", "wrong store", "wrong slot", "wrong load", "wrong opcode", "wrong type", "changed seed", "joined stores", "missing snapshot declaration"} {
		t.Run(mode, func(t *testing.T) {
			typ := types.NewJavaClass("java.lang.Object")
			parameter := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			parameter.IsParam = true
			parameter.MarkOriginalParameter(0)
			local := values.NewJavaRef(utils.NewRootVariableId(), parameter, typ)
			local.MarkOriginalLocalDeclaration(5, 1, parameter)
			snapshot := values.NewJavaRef(utils.NewRootVariableId(), local, typ)
			snapshot.MarkOriginalDynamicOperand(12, 0, local)
			declarationRef := *snapshot
			declarationRef.MarkOriginalDynamicOperandDeclaration(12, local)
			declaration := &statements.AssignStatement{IsDeclare: true, LeftValue: &declarationRef, JavaValue: local}
			lambda := &values.CustomValue{Flag: "lambda", HasOriginPC: true, OriginPC: 12, CapturesKnown: true, Captures: []values.JavaValue{snapshot}}
			operand := &nativeEnumSelectorProducer{pc: 11, opcode: core.OP_ALOAD_1, slot: 1, result: "Ljava/lang/Object;", local: &nativeEnumLocalRead{pc: 11, opcode: core.OP_ALOAD_1, slot: 1, storePC: 5, descriptor: "Ljava/lang/Object;"}}
			method := &MemberInfo{}
			site := &nativeLambdaLocalCaptureSite{method: method, code: &CodeAttribute{}, pc: 12, operands: []*nativeEnumSelectorProducer{operand}}
			ctx := &class_context.ClassContext{LocalNames: map[*utils.VariableId]string{}}
			c := &ClassObjectDumper{CurrentMethod: method, FuncCtx: ctx, nativeLambdaCaptureParameterNames: map[*MemberInfo][]nativeLambdaCaptureNameDemand{method: {{site: site, index: 0, name: "observed", value: snapshot}}}}
			body := []statements.Statement{&statements.AssignStatement{IsDeclare: true, LeftValue: local, JavaValue: parameter}, declaration, &statements.ReturnStatement{JavaValue: lambda}}
			switch mode {
			case "renamed":
				local.Id.SetName("anotherName")
			case "wrong store":
				operand.local.storePC++
			case "wrong slot":
				operand.slot++
				operand.local.slot++
			case "wrong load":
				operand.local.pc++
			case "wrong opcode":
				operand.local.opcode = core.OP_ALOAD_2
			case "wrong type":
				operand.local.descriptor = "Ljava/lang/String;"
			case "changed seed":
				local.Val = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			case "joined stores":
				operand.local.storePCs = []int{5, 8}
			case "missing snapshot declaration":
				body = append(body[:1], body[2:]...)
			}
			seed := local.Val
			c.prepareNativeCaptureBindings(body, []values.JavaValue{parameter})
			valid := mode == "original" || mode == "renamed"
			if c.nativeCaptureFailed == valid {
				t.Fatalf("failed=%v expected valid=%v", c.nativeCaptureFailed, valid)
			}
			if valid && ctx.LocalNames[snapshot.Id] != "observed" {
				t.Fatal("factory snapshot did not own the name")
			}
			if local.Val != seed || snapshot.Val != local || ctx.SourceCaptureStable != nil {
				t.Fatal("name constraint changed a value or granted allocation permission")
			}
		})
	}
}
