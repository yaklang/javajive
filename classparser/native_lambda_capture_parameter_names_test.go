package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
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
