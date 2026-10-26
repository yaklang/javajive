package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestSharedBooleanPhiStoreRequiresInertTerminalIdentity(t *testing.T) {
	for _, kind := range []string{"true", "false", "same coverage", "different coverage", "missing store pc", "wrong store pc", "missing return pc", "wrong return pc", "int value", "int target", "invocation", "field target", "parameter", "declaration", "wrong return local", "effectful return", "hidden", "encoded", "loop", "owned", "nonterminal", "int zero", "int one", "int two", "int negative", "long target", "float value"} {
		t.Run(kind, func(t *testing.T) {
			root, external, tail := jumpTestNode("root"), jumpTestNode("external"), jumpTestNode("tail")
			condition := core.NewNode(&statements.ConditionStatement{})
			typ := types.NewJavaPrimer(types.JavaBoolean)
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			literal := values.NewJavaLiteral(kind != "false", typ)
			assign := statements.NewAssignStatement(ref, literal, false)
			assign.OriginPC, assign.HasOriginPC = 20, true
			target := core.NewNode(assign)
			target.OriginPC, target.HasOriginPC = 20, true
			ret := statements.NewReturnStatement(ref)
			ret.OriginPC, ret.HasOriginPC = 30, true
			exit := core.NewNode(ret)
			exit.OriginPC, exit.HasOriginPC = 30, true
			condition.OriginPC, condition.HasOriginPC = 10, true
			root.AddNext(condition)
			root.AddNext(external)
			condition.AddNext(target)
			condition.AddNext(tail)
			external.AddNext(target)
			target.AddNext(exit)
			switch kind {
			case "same coverage", "different coverage":
				root.HasProtectedRange = true
				root.ProtectedStartPC, root.ProtectedEndPC = 10, 40
				if kind == "different coverage" {
					root.ProtectedEndPC = 20
				}
			case "missing store pc":
				assign.HasOriginPC = false
			case "wrong store pc":
				assign.OriginPC = 21
			case "missing return pc":
				ret.HasOriginPC = false
			case "wrong return pc":
				ret.OriginPC = 31
			case "int zero", "int one", "int two", "int negative":
				ref.ResetVarType(types.NewJavaPrimer(types.JavaInteger))
				literal.JavaType = types.NewJavaPrimer(types.JavaInteger)
				word := 0
				switch kind {
				case "int one":
					word = 1
				case "int two":
					word = 2
				case "int negative":
					word = -2
				}
				literal.Data = word
			case "long target":
				ref.ResetVarType(types.NewJavaPrimer(types.JavaLong))
			case "float value":
				literal.JavaType = types.NewJavaPrimer(types.JavaFloat)
				literal.Data = float32(0)
			case "int value":
				assign.JavaValue = values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
			case "int target":
				ref.ResetVarType(types.NewJavaPrimer(types.JavaInteger))
			case "invocation":
				assign.JavaValue = &values.FunctionCallExpression{}
			case "field target":
				assign.LeftValue = values.NewRefMember(ref, "field", typ)
			case "parameter":
				ref.IsParam = true
			case "declaration":
				assign.IsDeclare = true
			case "wrong return local":
				ret.JavaValue = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			case "effectful return":
				ret.JavaValue = &values.FunctionCallExpression{}
			case "hidden":
				target.HideNext = tail
			case "encoded":
				condition.EncodedJumps = map[*core.Node]bool{target: true}
			case "loop":
				target.IsInCircle = true
			case "owned":
				root.RemoveNext(external)
				condition.AddNext(external)
			case "nonterminal":
				exit.AddNext(tail)
			}
			manager := NewRootStatementManager(root)
			manager.DominatorMap = GenerateDominatorTree(root)
			splitSharedLiteralPhiStores(manager, condition)
			accepted := kind == "true" || kind == "false" || kind == "same coverage" || kind == "int zero" || kind == "int one" || kind == "int two" || kind == "int negative"
			if !accepted {
				if condition.Next[0] != target {
					t.Fatal("split an unproved phi terminal")
				}
				return
			}
			edge := condition.Next[0]
			copy, ok := edge.Statement.(*statements.AssignStatement)
			if edge == target || !ok || copy == assign || copy.LeftValue != ref || copy.JavaValue != literal || copy.OriginPC != 20 || !copy.HasOriginPC || edge.OriginPC != 20 || !edge.HasOriginPC || len(edge.Next) != 1 || edge.Next[0] != exit || external.Next[0] != target || len(target.Source) != 1 || condition.Next[1] != tail {
				t.Fatal("phi store identity, origin, continuation or selected edge changed")
			}
		})
	}
}

func TestSharedLiteralPhiKeepsEffectfulContinuationOnce(t *testing.T) {
	for _, variant := range []string{"integer", "boolean", "handler interval", "unknown table", "cross handler", "missing boundary", "catch continuation", "opaque successor"} {
		t.Run(variant, func(t *testing.T) {
			root, external := jumpTestNode("root"), jumpTestNode("external")
			condition := core.NewNode(&statements.ConditionStatement{})
			condition.OriginPC, condition.HasOriginPC = 10, true
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			literal := values.NewJavaLiteral(-37, types.NewJavaPrimer(types.JavaInteger))
			if variant == "boolean" {
				ref.ResetVarType(types.NewJavaPrimer(types.JavaBoolean))
				literal = values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
			}
			assign := statements.NewAssignStatement(ref, literal, false)
			assign.OriginPC, assign.HasOriginPC = 20, true
			target := core.NewNode(assign)
			target.OriginPC, target.HasOriginPC = 20, true
			call := &values.FunctionCallExpression{}
			exit := core.NewNode(statements.NewExpressionStatement(call))
			exit.OriginPC, exit.HasOriginPC = 30, true
			root.AddNext(condition)
			root.AddNext(external)
			condition.AddNext(target)
			external.AddNext(target)
			target.AddNext(exit)
			manager := NewRootStatementManager(root)
			manager.SetOriginalExceptionTable(nil)
			switch variant {
			case "handler interval":
				manager.SetOriginalExceptionTable([]*core.ExceptionTableEntry{{StartPc: 10, EndPc: 25, HandlerPc: 40}})
			case "unknown table":
				manager.originalHandlersKnown = false
			case "cross handler":
				manager.SetOriginalExceptionTable([]*core.ExceptionTableEntry{{StartPc: 15, EndPc: 25, HandlerPc: 40}})
			case "missing boundary":
				condition.HasOriginPC = false
			case "catch continuation":
				exit.IsCatchStart = true
			case "opaque successor":
				exit.HideNext = external
			}
			manager.DominatorMap = GenerateDominatorTree(root)
			splitSharedLiteralPhiStores(manager, condition)
			want := variant == "integer" || variant == "boolean" || variant == "handler interval"
			if (condition.Next[0] != target) != want {
				t.Fatalf("split=%v want=%v", condition.Next[0] != target, want)
			}
			if !want {
				return
			}
			edge := condition.Next[0]
			copied := edge.Statement.(*statements.AssignStatement)
			if copied == assign || copied.JavaValue != literal || copied.LeftValue != ref || copied.OriginPC != 20 || len(edge.Next) != 1 || edge.Next[0] != exit || external.Next[0] != target || exit.Statement.(*statements.ExpressionStatement).Expression != call {
				t.Fatal("store identity, original path or continuation effect changed")
			}
		})
	}
}
