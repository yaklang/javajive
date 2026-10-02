package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func jumpTestNode(text string) *core.Node {
	return core.NewNode(statements.NewCustomStatement(func(*class_context.ClassContext) string { return text }, nil))
}

func TestEncodedJumpsRequireExclusivelyAbruptPaths(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		header, tail := jumpTestNode("header"), jumpTestNode("tail")
		jump := jumpTestNode("continue")
		jump.IsJmp = true
		jump.AddNext(header)
		normal := jumpTestNode("effect()")
		normal.AddNext(tail)
		if mixed {
			normal.AddNext(header)
		}
		owner := jumpTestNode("container")
		owner.AddNext(header)
		owner.AddNext(tail)
		markEncodedJumps(owner, []*core.Node{jump, normal})
		if owner.EncodedJumps[header] == mixed || owner.EncodedJumps[tail] {
			t.Fatalf("mixed=%t: abrupt targets=%v", mixed, owner.EncodedJumps)
		}
		outer := jumpTestNode("outer")
		outer.AddNext(header)
		outer.AddNext(tail)
		markEncodedJumps(outer, []*core.Node{owner})
		if outer.EncodedJumps[header] == mixed || outer.EncodedJumps[tail] {
			t.Fatalf("nested mixed=%t lost target classification", mixed)
		}
	}
}

func TestIfNormalJoinIgnoresEncodedContinue(t *testing.T) {
	boolean := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
	root, header := jumpTestNode("start"), jumpTestNode("header")
	condition := core.NewNode(&statements.ConditionStatement{Condition: boolean})
	inner := core.NewNode(statements.NewIfStatement(boolean, []statements.Statement{jumpTestNode("continue").Statement}, nil))
	tail := jumpTestNode("sharedEffect()")
	end := core.NewNode(&statements.ReturnStatement{})
	root.AddNext(header)
	header.AddNext(condition)
	condition.AddNext(inner)
	condition.AddNext(tail)
	inner.AddNext(tail)
	inner.AddNext(header)
	inner.EncodedJumps = map[*core.Node]bool{header: true}
	tail.AddNext(end)
	manager := NewRootStatementManager(root)
	manager.DominatorMap = GenerateDominatorTree(root)
	if err := IfRewriter(manager, condition); err != nil {
		t.Fatal(err)
	}
	result := header.Next[0]
	st := result.Statement.(*statements.IfStatement)
	if len(st.IfBody) != 1 || st.IfBody[0] != inner.Statement || len(st.ElseBody) != 0 {
		t.Fatal("shared continuation was captured in a conditional arm")
	}
	if !result.EncodedJumps[header] || result.EncodedJumps[tail] {
		t.Fatal("enclosing if lost abrupt versus normal target identity")
	}
	found := false
	for _, next := range result.Next {
		found = found || next == tail
	}
	if !found {
		t.Fatal("shared continuation lost")
	}
}

func TestLoopJumpClassificationPreservesOwnNormalExit(t *testing.T) {
	outer, tail := jumpTestNode("outer"), jumpTestNode("tail")
	circle, owner := jumpTestNode("circle"), jumpTestNode("loop")
	localBreak, outerContinue := jumpTestNode("break"), jumpTestNode("continue OUTER")
	localBreak.IsJmp, outerContinue.IsJmp = true, true
	localBreak.AddNext(circle)
	circle.AddNext(tail)
	outerContinue.AddNext(outer)
	owner.AddNext(tail)
	owner.AddNext(outer)
	markEncodedJumps(owner, []*core.Node{localBreak, outerContinue, circle})
	if owner.EncodedJumps[tail] || !owner.EncodedJumps[outer] {
		t.Fatal("local break must complete the loop normally; outer continue must not")
	}
}

func TestSharedVoidReturnSplitPreservesOtherEntrances(t *testing.T) {
	for _, kind := range []string{"void", "value", "nonterminal", "owned"} {
		t.Run(kind, func(t *testing.T) {
			root, external, tail := jumpTestNode("root"), jumpTestNode("external"), jumpTestNode("tail")
			condition := core.NewNode(&statements.ConditionStatement{})
			ret := &statements.ReturnStatement{}
			if kind == "value" {
				ret.JavaValue = values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger))
			}
			target := core.NewNode(ret)
			root.AddNext(condition)
			condition.AddNext(target)
			condition.AddNext(tail)
			if kind == "owned" {
				tail.AddNext(external)
			} else {
				root.AddNext(external)
			}
			external.AddNext(target)
			if kind == "nonterminal" {
				target.AddNext(jumpTestNode("effect()"))
			}
			manager := NewRootStatementManager(root)
			manager.DominatorMap = GenerateDominatorTree(root)
			splitSharedTerminalLeaves(manager, condition)
			if kind != "void" {
				if condition.Next[0] != target {
					t.Fatal("return split without a terminal void-return proof")
				}
				return
			}
			if condition.Next[0] == target || condition.Next[1] != tail || external.Next[0] != target {
				t.Fatal("private terminal must preserve branch polarity and other entrances")
			}
			private, ok := condition.Next[0].Statement.(*statements.ReturnStatement)
			if !ok || private.JavaValue != nil || len(target.Source) != 1 || target.Source[0] != external {
				t.Fatal("return split changed the terminal or original predecessor set")
			}
		})
	}
}

func TestSharedThrowSplitRequiresOperandAndOrigin(t *testing.T) {
	for _, kind := range []string{"throw", "opaque", "missing pc", "nonterminal"} {
		t.Run(kind, func(t *testing.T) {
			root, external, tail := jumpTestNode("root"), jumpTestNode("external"), jumpTestNode("tail")
			condition := core.NewNode(&statements.ConditionStatement{})
			operand := values.NewJavaRef(nil, nil, types.NewJavaClass("java.lang.RuntimeException"))
			thrown := statements.NewCustomStatement(func(*class_context.ClassContext) string { return "throw failure" }, nil)
			thrown.ThrownValue, thrown.OriginPC, thrown.HasOriginPC = operand, 42, true
			if kind == "opaque" {
				thrown.ThrownValue = nil
			}
			if kind == "missing pc" {
				thrown.HasOriginPC = false
			}
			target := core.NewNode(thrown)
			target.OriginPC, target.HasOriginPC = 42, true
			root.AddNext(condition)
			root.AddNext(external)
			condition.AddNext(target)
			condition.AddNext(tail)
			external.AddNext(target)
			if kind == "nonterminal" {
				target.AddNext(tail)
			}
			manager := NewRootStatementManager(root)
			manager.DominatorMap = GenerateDominatorTree(root)
			splitSharedTerminalLeaves(manager, condition)
			if kind != "throw" {
				if condition.Next[0] != target {
					t.Fatal("unproved terminal duplicated")
				}
				return
			}
			private, ok := condition.Next[0].Statement.(*statements.CustomStatement)
			if !ok || private == thrown || private.ThrownValue != operand || private.OriginPC != 42 || !private.HasOriginPC || external.Next[0] != target || condition.Next[1] != tail || len(target.Source) != 1 {
				t.Fatal("tail duplication lost operand, PC, polarity or other predecessor")
			}
		})
	}
}
