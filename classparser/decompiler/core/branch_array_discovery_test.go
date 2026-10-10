package core

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestPrivateArrayCallDiscoveryUsesOriginalInvocationRoots(t *testing.T) {
	for _, scenario := range []string{"original", "slot", "reference chain", "ordinary call", "ordinary call inside constructor", "ordinary call also owns original ternary", "nested constructor", "not original invoke", "missing origin", "different origin", "no dup identity", "parameter", "opaque ref", "reference cycle", "missing allocation origin", "object allocation", "canceled", "memory", "budget"} {
		t.Run(scenario, func(t *testing.T) {
			typ := types.NewJavaArrayType(types.NewJavaClass("example.Token"))
			array := values.NewNewExpression(typ)
			array.HasOriginPC, array.OriginPC = true, 1
			ref := values.NewJavaRef(utils.NewRootVariableId(), array, typ)
			dup, invoke := op(OP_DUP, 2), op(OP_INVOKESPECIAL, 10)
			call := &values.FunctionCallExpression{Kind: values.InvokeSpecial, IsSpecialInvoke: true, ClassName: "example.Owner", FunctionName: "<init>", Descriptor: "([Lexample/Token;)V", HasOriginPC: true, OriginPC: 10, Arguments: []values.JavaValue{ref}}
			d := &Decompiler{opCodes: []*OpCode{dup, invoke}, opcodeIdToRef: map[*OpCode][][2]any{dup: {{ref, nil}}}, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{invoke: call}}
			d.opcodeToSimulateStack = map[*OpCode]*StackSimulationImpl{dup: nil, invoke: nil}
			switch scenario {
			case "slot":
				call.Arguments[0] = values.NewSlotValue(ref, typ)
			case "reference chain":
				ref.Val = values.NewJavaRef(utils.NewRootVariableId(), array, typ)
			case "ordinary call":
				call.Kind, call.IsSpecialInvoke, call.IsStatic, call.FunctionName = values.InvokeStatic, false, true, "consume"
				invoke.Instr.OpCode = OP_INVOKESTATIC
			case "ordinary call inside constructor", "ordinary call also owns original ternary", "nested constructor":
				if scenario != "nested constructor" {
					call.Kind, call.IsSpecialInvoke, call.IsStatic, call.FunctionName = values.InvokeStatic, false, true, "consume"
					invoke.Instr.OpCode = OP_INVOKESTATIC
				}
				outerOp := op(OP_INVOKESPECIAL, 12)
				outer := &values.FunctionCallExpression{Kind: values.InvokeSpecial, IsSpecialInvoke: true, ClassName: "example.Wrapper", FunctionName: "<init>", Descriptor: "(Ljava/lang/Object;)V", HasOriginPC: true, OriginPC: 12, Arguments: []values.JavaValue{call}}
				d.opCodes = append(d.opCodes, outerOp)
				d.invokeFuncCall[outerOp] = outer
				if scenario == "ordinary call also owns original ternary" {
					tern := values.NewTernaryExpression(nil, call, values.NewJavaLiteral(nil, types.NewJavaClass("java.lang.Object")))
					d.valueTernaryMerges = map[*values.TernaryExpression]*OpCode{tern: invoke}
				}
			case "not original invoke":
				invoke.Instr.OpCode = OP_NOP
			case "missing origin":
				call.HasOriginPC = false
			case "different origin":
				call.OriginPC = 9
			case "no dup identity":
				d.opcodeIdToRef = nil
			case "parameter":
				ref.IsParam = true
			case "opaque ref":
				ref.CustomValue = &values.CustomValue{}
			case "reference cycle":
				ref.Val = ref
			case "missing allocation origin":
				array.HasOriginPC = false
			case "object allocation":
				array.JavaType = types.NewJavaClass("example.Token")
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			d.RegisterNestedBranchArrayCalls()
			want := scenario == "original" || scenario == "slot" || scenario == "reference chain" || scenario == "ordinary call also owns original ternary" || scenario == "nested constructor"
			if (len(d.branchArrayCalls) == 1) != want {
				t.Fatalf("discovered=%d want=%v", len(d.branchArrayCalls), want)
			}
			if want {
				d.RegisterNestedBranchArrayCalls()
				if len(d.branchArrayCalls) != 1 || call.Arguments[0] == array {
					t.Fatal("discovery duplicated or moved operand without original transfer proof")
				}
			}
			if d.Work != nil && d.Work.Err() == nil {
				t.Fatal("lost resource classification")
			}
		})
	}
}

// A completed object tree recursively contains its constructor operands. The
// original NEW receiver's allocation precedes those operands; DUP is not NEW.
func TestPrivateArrayConstructorReceiverUsesOriginalAllocation(t *testing.T) {
	for _, scenario := range []string{"original", "provisional constructor node", "missing receiver origin", "different source constructor", "different invoke identity", "missing produced identity", "new after array", "wrong allocation opcode", "branch entry", "handler boundary"} {
		t.Run(scenario, func(t *testing.T) {
			n := values.NewNewExpression(types.NewJavaClass("example.Carrier"))
			n.OriginPC, n.HasOriginPC = 1, true
			call := &values.FunctionCallExpression{Object: n, ClassName: "example.Carrier", FunctionName: "<init>", Descriptor: "([Ljava/lang/Object;)V", Kind: values.InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: 10}
			n.ConstructorCall = call
			create, dup, allocation, invoke := op(OP_NEW, 1), op(OP_DUP, 2), op(OP_ANEWARRAY, 3), op(OP_INVOKESPECIAL, 10)
			create.stackProduced = []values.JavaValue{n}
			dup.stackProduced = []values.JavaValue{n, n}
			all := []*OpCode{create, dup, allocation, invoke}
			for i := 0; i < len(all)-1; i++ {
				all[i].Target = []*OpCode{all[i+1]}
				all[i+1].Source = []*OpCode{all[i]}
			}
			d := &Decompiler{opCodes: all, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{invoke: call}}
			d.opcodeToSimulateStack = map[*OpCode]*StackSimulationImpl{}
			for _, op := range all {
				d.opcodeToSimulateStack[op] = nil
			}
			switch scenario {
			case "provisional constructor node":
				copy := *call
				d.invokeFuncCall[invoke] = &copy
			case "missing receiver origin":
				n.HasOriginPC = false
			case "different source constructor":
				copy := *call
				n.ConstructorCall = &copy
			case "different invoke identity":
				copy := *call
				copy.Descriptor = "([Ljava/lang/String;)V"
				d.invokeFuncCall[invoke] = &copy
			case "missing produced identity":
				create.stackProduced = nil
			case "new after array":
				create.CurrentOffset = 5
			case "wrong allocation opcode":
				create.Instr.OpCode = OP_ANEWARRAY
			case "branch entry":
				dup.Target = append(dup.Target, op(OP_RETURN, 8))
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 2, EndPc: 6, HandlerPc: 12}}
			}
			want := scenario == "original" || scenario == "provisional constructor node"
			if got := d.branchArrayReceiverPrecedes(call, allocation); got != want {
				t.Fatalf("got=%v want=%v", got, want)
			}
		})
	}
}

func TestPrivateArrayMotionRequiresImmediateUniqueOrderedConsumer(t *testing.T) {
	for _, scenario := range []string{"direct", "wrapped pure", "earlier original effect", "intervening void call", "published later array", "dead end", "multiple successors", "catch entry", "try entry", "duplicated invocation", "opaque", "cycle", "hidden definition", "later effect", "canceled", "budget", "memory"} {
		t.Run(scenario, func(t *testing.T) {
			call := &values.FunctionCallExpression{IsStatic: true, Object: values.NewJavaClassValue(types.NewJavaClass("example.Probe")), FunctionName: "consume"}
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			consumer := &Node{Statement: statements.NewReturnStatement(call)}
			definition := &Node{Next: []*Node{consumer}}
			allocation := op(OP_ANEWARRAY, 4)
			d := &Decompiler{}
			wrap := func(first values.JavaValue) {
				consumer.Statement = statements.NewReturnStatement(&values.FunctionCallExpression{IsStatic: true, Arguments: []values.JavaValue{first, call}})
			}
			switch scenario {
			case "wrapped pure":
				wrap(values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger)))
			case "earlier original effect", "later effect":
				effect := &values.FunctionCallExpression{IsStatic: true, FunctionName: "effect"}
				wrap(effect)
				producer := op(OP_INVOKESTATIC, 1)
				producer.stackProduced = []values.JavaValue{effect}
				producer.Target = []*OpCode{allocation}
				allocation.Source = []*OpCode{producer}
				d.opCodes = []*OpCode{producer, allocation}
				if scenario == "later effect" {
					producer.CurrentOffset = 5
				}
			case "intervening void call":
				definition.Next = []*Node{{Statement: statements.NewExpressionStatement(&values.FunctionCallExpression{IsStatic: true, FunctionName: "independent"}), Next: []*Node{consumer}}}
			case "published later array":
				definition.Next = []*Node{{Statement: statements.NewAssignStatement(ref, values.NewNewExpression(types.NewJavaArrayType(types.NewJavaClass("java.lang.Object"))), true), Next: []*Node{consumer}}}
			case "dead end":
				definition.Next = nil
			case "multiple successors":
				definition.Next = append(definition.Next, &Node{})
			case "catch entry":
				consumer.IsCatchStart = true
			case "try entry":
				consumer.IsTryCatch = true
			case "duplicated invocation":
				wrap(call)
			case "opaque":
				consumer.Statement = statements.NewReturnStatement(&values.CustomValue{})
			case "cycle":
				cyclic := &values.FunctionCallExpression{}
				cyclic.Arguments = []values.JavaValue{cyclic, call}
				consumer.Statement = statements.NewReturnStatement(cyclic)
			case "hidden definition":
				ref.Val = call
				consumer.Statement = statements.NewReturnStatement(ref)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			want := scenario == "direct" || scenario == "wrapped pure" || scenario == "earlier original effect"
			if got := d.branchArrayImmediateConsumer(definition, call, allocation); got != want {
				t.Fatalf("got=%v want=%v", got, want)
			}
		})
	}
}

// Keeping an effect before the array is insufficient if a surrounding source
// NEW would be moved after it. Class initialization/allocation order matters.
func TestPrivateArrayNestedConstructorKeepsEarlierOperandBeforeOriginalNew(t *testing.T) {
	for _, scenario := range []string{"effect before NEW", "effect after NEW", "missing NEW witness"} {
		t.Run(scenario, func(t *testing.T) {
			n := values.NewNewExpression(types.NewJavaClass("example.Carrier"))
			n.OriginPC, n.HasOriginPC = 3, true
			target := &values.FunctionCallExpression{Object: n, FunctionName: "<init>"}
			n.ConstructorCall = target
			effect := &values.FunctionCallExpression{IsStatic: true, FunctionName: "effect"}
			root := &values.FunctionCallExpression{IsStatic: true, Arguments: []values.JavaValue{effect, n}}
			consumer := &Node{Statement: statements.NewReturnStatement(root)}
			definition := &Node{Next: []*Node{consumer}}
			effectOp, create, allocation := op(OP_INVOKESTATIC, 1), op(OP_NEW, 3), op(OP_ANEWARRAY, 5)
			effectOp.stackProduced = []values.JavaValue{effect}
			all := []*OpCode{effectOp, create, allocation}
			if scenario == "effect after NEW" {
				effectOp.CurrentOffset = 4
				all = []*OpCode{create, effectOp, allocation}
			}
			for i := 0; i < len(all)-1; i++ {
				all[i].Target = []*OpCode{all[i+1]}
				all[i+1].Source = []*OpCode{all[i]}
			}
			d := &Decompiler{opCodes: all, opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{effectOp: nil, create: nil, allocation: nil}}
			if scenario == "missing NEW witness" {
				delete(d.opcodeToSimulateStack, create)
			}
			want := scenario == "effect before NEW"
			if got := d.branchArrayImmediateConsumer(definition, target, allocation); got != want {
				t.Fatalf("got=%v want=%v", got, want)
			}
		})
	}
}

func TestPrivateArrayConditionalSourceArmUsesOriginalRouting(t *testing.T) {
	for _, scenario := range []string{"true arm", "false arm", "mutated structuring targets", "foreign condition", "not proved diamond", "duplicated call", "wrong original arm", "alternate entry", "exceptional entry", "missing graph", "changed handler"} {
		t.Run(scenario, func(t *testing.T) {
			// Original graph: NEW -> IF -> {other, allocation} -> merge.
			g := t09Graph(6, t09Edge{0, 1, EdgeFallthrough}, t09Edge{1, 2, EdgeTaken}, t09Edge{1, 3, EdgeFallthrough}, t09Edge{2, 5, EdgeFallthrough}, t09Edge{3, 4, EdgeFallthrough}, t09Edge{4, 5, EdgeFallthrough})
			for i, n := range g.Nodes {
				n.Id = i + 1
				n.CurrentOffset = uint16(i + 1)
				n.Instr = &Instruction{OpCode: OP_NOP}
			}
			branch, allocation := g.Nodes[1], g.Nodes[3]
			branch.Instr.OpCode = OP_IFEQ
			branch.TernaryChainArm = true
			branch.Target = []*OpCode{g.Nodes[2], g.Nodes[3]}
			call := &values.FunctionCallExpression{FunctionName: "consume"}
			other := values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))
			tern := values.NewTernaryExpression(nil, call, other)
			tern.ConditionFromOp = branch.Id
			d := &Decompiler{opCodes: g.Nodes, semanticCFG: g}
			if scenario == "false arm" {
				tern.TrueValue, tern.FalseValue = other, call
				branch.Target[0], branch.Target[1] = branch.Target[1], branch.Target[0]
				for i := range g.Edges {
					if g.Edges[i].From == branch {
						if g.Edges[i].Kind == EdgeTaken {
							g.Edges[i].Kind = EdgeFallthrough
						} else if g.Edges[i].Kind == EdgeFallthrough {
							g.Edges[i].Kind = EdgeTaken
						}
					}
				}
			}
			switch scenario {
			case "foreign condition":
				tern.ConditionFromOp = 100
			case "not proved diamond":
				branch.TernaryChainArm = false
			case "duplicated call":
				tern.FalseValue = call
			case "mutated structuring targets":
				branch.Target[0], branch.Target[1] = branch.Target[1], branch.Target[0]
			case "wrong original arm":
				tern.TrueValue, tern.FalseValue = other, call
			case "alternate entry", "exceptional entry":
				kind := EdgeFallthrough
				if scenario == "exceptional entry" {
					kind = EdgeException
				}
				g.Edges = append(g.Edges, SemanticEdge{From: g.Nodes[0], To: allocation, Kind: kind})
			case "missing graph":
				d.semanticCFG = nil
			case "changed handler":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: allocation.CurrentOffset, EndPc: 5, HandlerPc: 6}}
			}
			got, original := d.branchArraySelectedSourceArm(tern, call, allocation)
			want := scenario == "true arm" || scenario == "false arm" || scenario == "mutated structuring targets"
			if (got == call && original == branch) != want {
				t.Fatalf("selected=%v want=%v", got == call, want)
			}
		})
	}
}

func TestConstructorDiscoveryKeepsOrdinaryGenericSourceContexts(t *testing.T) {
	call := &values.FunctionCallExpression{ClassName: "example.Owner", FunctionName: "factory", Arguments: []values.JavaValue{values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))}}
	n := values.NewNewExpression(types.NewJavaClass("example.Carrier"))
	constructor := &values.FunctionCallExpression{FunctionName: "<init>", IsSpecialInvoke: true, Arguments: []values.JavaValue{call}, Object: n}
	n.ConstructorCall = constructor
	for _, scenario := range []string{"ordinary", "new", "nested new", "opaque", "cycle", "hidden definition", "canceled", "memory", "budget"} {
		t.Run(scenario, func(t *testing.T) {
			d := &Decompiler{}
			var root values.JavaValue = call
			switch scenario {
			case "new":
				root = n
			case "nested new":
				root = &values.FunctionCallExpression{Arguments: []values.JavaValue{n}}
			case "opaque":
				root = &values.CustomValue{}
			case "cycle":
				c := &values.FunctionCallExpression{}
				c.Arguments = []values.JavaValue{n, c}
				root = c
			case "hidden definition":
				root = values.NewJavaRef(utils.NewRootVariableId(), n, n.Type())
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
				root = n
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				root = n
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				root = n
			}
			got := d.branchArraySourceConstructors(root)
			want := scenario == "new" || scenario == "nested new"
			if (len(got) == 1 && got[0] == constructor) != want {
				t.Fatalf("roots=%d want=%v", len(got), want)
			}
			if d.Work != nil && d.Work.Err() == nil {
				t.Fatal("lost resource classification")
			}
		})
	}
}

func TestPrivateArrayEarlierOperandRequiresOriginalExceptionalDominance(t *testing.T) {
	for _, kind := range []string{"call", "completed NEW"} {
		for _, scenario := range []string{"dominating", "alternate entry", "exceptional entry", "handler change", "missing graph", "missing origin", "different binding", "missing produced identity", "late producer"} {
			t.Run(kind+"/"+scenario, func(t *testing.T) {
				g := t09Graph(7, t09Edge{0, 1, EdgeFallthrough}, t09Edge{1, 2, EdgeFallthrough}, t09Edge{2, 3, EdgeFallthrough}, t09Edge{3, 4, EdgeTaken}, t09Edge{3, 5, EdgeFallthrough}, t09Edge{4, 6, EdgeFallthrough}, t09Edge{5, 6, EdgeFallthrough})
				for i, n := range g.Nodes {
					n.Id = i + 1
					n.CurrentOffset = uint16(i + 1)
					n.Instr = &Instruction{OpCode: OP_NOP}
				}
				for _, e := range g.Edges {
					e.From.Target = append(e.From.Target, e.To)
					e.To.Source = append(e.To.Source, e.From)
				}
				create, invoke, array := g.Nodes[1], g.Nodes[2], g.Nodes[5]
				invoke.Instr.OpCode = OP_INVOKESTATIC
				g.Nodes[3].Instr.OpCode = OP_IFEQ
				array.Instr.OpCode = OP_ANEWARRAY
				call := &values.FunctionCallExpression{Kind: values.InvokeStatic, IsStatic: true, ClassName: "sample.Token", FunctionName: "produce", Descriptor: "()Ljava/lang/Object;", HasOriginPC: true, OriginPC: int(invoke.CurrentOffset)}
				var value values.JavaValue = call
				invoke.stackProduced = []values.JavaValue{call}
				if kind == "completed NEW" {
					n := values.NewNewExpression(types.NewJavaClass("sample.Token"))
					n.HasOriginPC, n.OriginPC = true, int(create.CurrentOffset)
					n.ConstructorCall = call
					call.Kind, call.IsStatic, call.IsSpecialInvoke, call.FunctionName, call.Descriptor, call.Object = values.InvokeSpecial, false, true, "<init>", "()V", n
					invoke.Instr.OpCode = OP_INVOKESPECIAL
					create.Instr.OpCode = OP_NEW
					create.stackProduced = []values.JavaValue{n}
					value = n
				}
				d := &Decompiler{opCodes: g.Nodes, semanticCFG: g, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{invoke: call}, opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{}}
				for _, n := range g.Nodes {
					d.opcodeToSimulateStack[n] = nil
				}
				switch scenario {
				case "alternate entry", "exceptional entry":
					edge := EdgeFallthrough
					if scenario == "exceptional entry" {
						edge = EdgeException
					}
					g.Edges = append(g.Edges, SemanticEdge{From: g.Nodes[0], To: array, Kind: edge})
				case "handler change":
					d.ExceptionTable = []*ExceptionTableEntry{{StartPc: array.CurrentOffset, EndPc: 7, HandlerPc: 8}}
				case "missing graph":
					d.semanticCFG = nil
				case "missing origin":
					call.HasOriginPC = false
				case "different binding":
					copy := *call
					copy.Descriptor = "()Ljava/lang/String;"
					d.invokeFuncCall[invoke] = &copy
				case "missing produced identity":
					invoke.stackProduced = nil
					create.stackProduced = nil
				case "late producer":
					invoke.CurrentOffset = array.CurrentOffset + 1
					call.OriginPC = int(invoke.CurrentOffset)
				}
				if got, want := d.branchArrayEarlierOperandPrecedes(value, array), scenario == "dominating"; got != want {
					t.Fatalf("proof=%v want=%v", got, want)
				}
			})
		}
	}
}
