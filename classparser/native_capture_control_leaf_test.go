package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNativeCaptureStableIdentityUsesSealedOperandFreeSourceLeaves(t *testing.T) {
	for _, scenario := range []string{"parameter", "anchor", "monitor exit", "structured transfer", "local with monitor", "nil anchor", "unsealed monitor", "changed monitor payload", "changed monitor kind", "monitor enter", "opaque empty", "hidden callback", "invalid transfer label", "parameter overwritten", "local overwritten", "branch local escapes", "different declaration ID", "statement cycle"} {
		t.Run(scenario, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			ref.IsParam = true
			child := &nativeAnonymousClass{descriptor: "(I)V", method: "make(I)Ljava/lang/Object;", fields: map[string]int{"val$input": 0}}
			family := &nativeAnonymousFamily{owner: "LeafOwner", children: map[string]*nativeAnonymousClass{"LeafOwner$1": child}}
			c := &ClassObjectDumper{nativeAnonymousRoot: family, FuncCtx: &class_context.ClassContext{ClassName: "LeafOwner", FunctionName: "make", CurrentMethodDesc: "(I)Ljava/lang/Object;"}}
			call := &values.FunctionCallExpression{ClassName: "LeafOwner$1", FunctionName: "<init>", Descriptor: "(I)V", Arguments: []values.JavaValue{ref}, OriginPC: 8, HasOriginPC: true}
			allocation := &values.NewExpression{JavaType: types.NewJavaClass("LeafOwner$1"), ConstructorCall: call, OriginPC: 4, HasOriginPC: true}
			monitor := statements.NewOriginalMonitorStatement("monitor_exit", nil, 3, 0)
			var leaf statements.Statement = monitor
			switch scenario {
			case "parameter":
				leaf = &statements.MiddleStatement{Flag: "start"}
			case "anchor":
				leaf = statements.NewSourceAnchorStatement()
			case "structured transfer":
				leaf = statements.NewSourceTransferStatement("break", "OUTER")
			case "nil anchor":
				leaf = (*statements.SourceAnchorStatement)(nil)
			case "unsealed monitor":
				leaf = statements.NewMiddleStatement("monitor_exit", nil)
			case "changed monitor payload":
				monitor.Data = ref
			case "changed monitor kind":
				monitor.Flag = "monitor_enter"
			case "monitor enter":
				leaf = statements.NewOriginalMonitorStatement("monitor_enter", ref, 0, 0)
			case "opaque empty":
				leaf = statements.NewCustomStatement(func(*class_context.ClassContext) string { return "" }, nil)
			case "hidden callback":
				leaf = statements.NewCustomStatement(func(ctx *class_context.ClassContext) string { return ref.String(ctx) + "=1;" }, nil)
			case "invalid transfer label":
				s := statements.NewSourceTransferStatement("continue", "OUTER")
				s.LoopTargetLabel = "OUTER;input=2"
				s.Name = "continue"
				leaf = s
			}
			body := []statements.Statement{leaf, statements.NewReturnStatement(allocation)}
			params := []values.JavaValue{ref}
			switch scenario {
			case "parameter overwritten":
				body = append(body, statements.NewAssignStatement(ref, values.NewJavaLiteral(1, ref.Type()), false))
			case "local with monitor", "local overwritten", "branch local escapes":
				ref.IsParam = false
				params = nil
				c.FuncCtx.CurrentMethodDesc = "()Ljava/lang/Object;"
				child.method = "make()Ljava/lang/Object;"
				declaration := statements.NewAssignStatement(ref, values.NewJavaLiteral(7, ref.Type()), true)
				body = append([]statements.Statement{declaration}, body...)
				if scenario == "local overwritten" {
					body = append(body, statements.NewAssignStatement(ref, values.NewJavaLiteral(9, ref.Type()), false))
				}
				if scenario == "branch local escapes" {
					body = []statements.Statement{&statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{declaration}}, leaf, statements.NewReturnStatement(allocation)}
				}
			case "different declaration ID":
				other := values.NewJavaRef(utils.NewRootVariableId(), nil, ref.Type())
				call.Arguments[0] = other
			case "statement cycle":
				cycle := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))}
				cycle.IfBody = []statements.Statement{cycle}
				body = append(body, cycle)
			}
			want := scenario == "parameter" || scenario == "anchor" || scenario == "monitor exit" || scenario == "structured transfer" || scenario == "local with monitor"
			c.prepareNativeCaptureBindings(body, params)
			if family.failed == want {
				t.Fatalf("admitted=%v expected=%v", !family.failed, want)
			}
			if want && (c.FuncCtx.SourceCaptureStable == nil || !c.FuncCtx.SourceCaptureStable(8, ref.Id) || c.FuncCtx.SourceCaptureStable(9, ref.Id)) {
				t.Fatal("lost exact allocation/declaration binding")
			}
			// The same source leaf is still opaque to exception/control proofs.
			if scenario == "anchor" || scenario == "monitor exit" || scenario == "structured transfer" {
				if _, _, known := catchSourceChildren(leaf); known {
					t.Fatal("name binding granted control permission")
				}
			}
		})
	}
}
