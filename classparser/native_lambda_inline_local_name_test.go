package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestNativeLambdaInlinedLocalSpellingReopensSourceDeclarationForEveryFactory(t *testing.T) {
	for _, change := range []string{"original", "explicit declaration alias", "unsealed declaration alias", "missing local declaration", "changed declaration seed", "changed store location", "changed store witness", "second assignment", "invisible declaration", "different same-typed local", "changed inline operand", "changed inline seed", "name conflict", "wrong inline factory", "changed capture", "changed load", "changed store", "changed slot"} {
		t.Run(change, func(t *testing.T) {
			typ := types.NewJavaClass("java.lang.Object")
			parameter := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			parameter.IsParam = true
			parameter.MarkOriginalParameter(0)
			local := values.NewJavaRef(utils.NewRootVariableId(), parameter, typ)
			local.MarkOriginalLocalDeclaration(5, 1, parameter)
			declaration := &statements.AssignStatement{IsDeclare: true, HasOriginPC: true, OriginPC: 5, LeftValue: local, JavaValue: parameter}
			declaration.MarkOriginalLocalStore(5, 1)
			method := &MemberInfo{}
			ctx := &class_context.ClassContext{LocalNames: map[*utils.VariableId]string{}}
			c := &ClassObjectDumper{CurrentMethod: method, FuncCtx: ctx, nativeLambdaCaptureParameterNames: map[*MemberInfo][]nativeLambdaCaptureNameDemand{}}
			var lambdas []*values.CustomValue
			var snapshots []*values.JavaRef
			for _, pc := range []int{12, 22} {
				seed := local
				storePC, slot := 5, 1
				if change == "different same-typed local" && pc == 22 {
					storePC, slot = 7, 2
					seed = values.NewJavaRef(utils.NewRootVariableId(), parameter, typ)
					seed.MarkOriginalLocalDeclaration(storePC, slot, parameter)
				}
				snapshot := values.NewJavaRef(utils.NewRootVariableId(), seed, typ)
				origin := pc
				if change == "wrong inline factory" && pc == 22 {
					origin++
				}
				snapshot.MarkOriginalDynamicOperand(origin, 0, seed)
				snapshot.StackVar = seed
				snapshot.MarkOriginalDynamicOperandInline(origin, seed)
				lambda := &values.CustomValue{Flag: "lambda", HasOriginPC: true, OriginPC: pc, CapturesKnown: true, Captures: []values.JavaValue{snapshot}}
				operand := &nativeEnumSelectorProducer{pc: pc - 1, opcode: core.OP_ALOAD, slot: slot, result: "Ljava/lang/Object;", local: &nativeEnumLocalRead{pc: pc - 1, opcode: core.OP_ALOAD, slot: slot, storePC: storePC, descriptor: "Ljava/lang/Object;"}}
				site := &nativeLambdaLocalCaptureSite{method: method, code: &CodeAttribute{}, pc: pc, operands: []*nativeEnumSelectorProducer{operand}}
				c.nativeLambdaCaptureParameterNames[method] = append(c.nativeLambdaCaptureParameterNames[method], nativeLambdaCaptureNameDemand{site: site, index: 0, name: "observed", value: snapshot})
				lambdas = append(lambdas, lambda)
				snapshots = append(snapshots, snapshot)
			}
			factoryResult := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			body := []statements.Statement{declaration, &statements.AssignStatement{IsDeclare: true, LeftValue: factoryResult, JavaValue: lambdas[0]}, &statements.ReturnStatement{JavaValue: lambdas[1]}}
			switch change {
			case "explicit declaration alias":
				declaration.LeftValue = local.CloneForSourceDeclaration()
			case "unsealed declaration alias":
				copy := *local
				declaration.LeftValue = &copy
			case "missing local declaration":
				body = body[1:]
			case "changed declaration seed":
				declaration.JavaValue = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			case "changed store location":
				declaration.OriginPC++
			case "changed store witness":
				declaration = &statements.AssignStatement{IsDeclare: true, HasOriginPC: true, OriginPC: 5, LeftValue: local, JavaValue: parameter}
				body[0] = declaration
			case "second assignment":
				body = append(body[:1], append([]statements.Statement{&statements.AssignStatement{LeftValue: local, JavaValue: values.JavaNull}}, body[1:]...)...)
			case "invisible declaration":
				body[0] = &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{declaration}}
			case "different same-typed local":
				other := snapshots[1].StackVar.(*values.JavaRef)
				otherDeclaration := &statements.AssignStatement{IsDeclare: true, HasOriginPC: true, OriginPC: 7, LeftValue: other, JavaValue: parameter}
				otherDeclaration.MarkOriginalLocalStore(7, 2)
				body = append(body[:1], append([]statements.Statement{otherDeclaration}, body[1:]...)...)
			case "changed inline operand":
				snapshots[1].StackVar = values.NewJavaRef(utils.NewRootVariableId(), parameter, typ)
			case "changed inline seed":
				snapshots[1].Val = parameter
			case "name conflict":
				ctx.LocalNames[local.Id] = "another"
			case "changed capture":
				lambdas[1].Captures[0] = local
			case "changed load":
				c.nativeLambdaCaptureParameterNames[method][1].site.operands[0].local.pc++
			case "changed store":
				c.nativeLambdaCaptureParameterNames[method][1].site.operands[0].local.storePC++
			case "changed slot":
				c.nativeLambdaCaptureParameterNames[method][1].site.operands[0].local.slot++
			}
			c.prepareNativeCaptureBindings(body, []values.JavaValue{parameter})
			want := change == "original" || change == "explicit declaration alias"
			if c.nativeCaptureFailed == want {
				t.Fatalf("binding failed=%v want valid=%v", c.nativeCaptureFailed, want)
			}
			if want && (ctx.LocalNames[local.Id] != "observed" || declaration.Final) {
				t.Fatal("original binder lost or an explicit final constant was invented")
			}
			if lambdas[0].Captures[0] != snapshots[0] || lambdas[1].Captures[0] != snapshots[1] && change != "changed capture" || ctx.SourceCaptureStable != nil || c.nativeAnonymousRoot != nil {
				t.Fatal("name proof changed a factory or granted ownership/allocation permission")
			}
		})
	}
}
