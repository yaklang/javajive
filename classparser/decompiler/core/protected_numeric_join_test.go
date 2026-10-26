package core

import (
	"errors"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestProtectedNumericJoinUsesThrowingInstructionInputState(t *testing.T) {
	for _, scenario := range []string{"proved", "mutable graph changed", "missing graph", "failed graph", "analysis budget", "missing store", "missing handler use", "boolean producer", "entry definition"} {
		t.Run(scenario, func(t *testing.T) {
			// x=0; x=call(); x=call(); return x;
			// catch: return x. The handler sees the initial store and FIRST
			// call's successful store, never the second call's output store.
			d := auditCFG(t, []byte{OP_ICONST_0, OP_ISTORE_0, OP_INVOKESTATIC, 0, 1, OP_ISTORE_0, OP_INVOKESTATIC, 0, 2, OP_ISTORE_0, OP_ILOAD_0, OP_IRETURN, OP_ASTORE_1, OP_ILOAD_0, OP_IRETURN})
			d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 2, EndPc: 10, HandlerPc: 12, CatchType: 1}}
			g, err := d.buildSemanticCFG()
			if err != nil {
				t.Fatal(err)
			}
			d.semanticCFG = g
			at := func(pc uint16) *OpCode { return d.opCodes[d.offsetToOpcodeIndex[pc]] }
			initial, first, last := at(1), at(5), at(9)
			normal, caught := at(10), at(13)
			typ := types.NewJavaPrimer(types.JavaInteger)
			x := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			y := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			z := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			x.Id.SetName("before")
			y.Id.SetName("between")
			z.Id.SetName("after")
			call1 := &values.FunctionCallExpression{Descriptor: "()I", HasOriginPC: true, OriginPC: 2, FuncType: &types.JavaFuncType{ReturnType: typ}}
			call2 := &values.FunctionCallExpression{Descriptor: "()I", HasOriginPC: true, OriginPC: 6, FuncType: &types.JavaFuncType{ReturnType: typ}}
			d.invokeFuncCall = map[*OpCode]*values.FunctionCallExpression{at(2): call1, at(6): call2}
			initial.stackConsumed = []values.JavaValue{values.NewJavaLiteral(0, typ)}
			first.stackConsumed = []values.JavaValue{call1}
			last.stackConsumed = []values.JavaValue{call2}
			catchView, normalView := values.NewSlotValue(x, typ), values.NewSlotValue(z, typ)
			caught.stackProduced, normal.stackProduced = []values.JavaValue{catchView}, []values.JavaValue{normalView}
			d.opcodeIdToRef = map[*OpCode][][2]any{initial: {{x, true}}, first: {{y, true}}, last: {{z, true}}}
			defs, entry := g.ReachingDefinitions(caught, 0)
			if entry || len(defs) != 2 || defs[0] != initial || defs[1] != first {
				t.Fatal("fixture did not retain exceptional input state")
			}
			switch scenario {
			case "mutable graph changed":
				caught.Source = []*OpCode{last}
			case "missing graph":
				d.semanticCFG = nil
			case "failed graph":
				g.Err = errors.New("incomplete")
			case "analysis budget":
				g.slots = map[int]*slotDefinitions{}
				g.MaxUpdates = 1
			case "missing store", "missing handler use":
				missing := first
				if scenario == "missing handler use" {
					missing = caught
				}
				var decoded []*OpCode
				for _, node := range d.opCodes {
					if node != missing {
						decoded = append(decoded, node)
					}
				}
				d.opCodes = decoded
			case "boolean producer":
				call1.Descriptor = "()Z"
			case "entry definition":
				// Reopening a handler before a slot's first store must reject
				// a local-only canonicalization that would swallow live-in state.
				webs := d.slotWebs()
				webs.entryWeb[0] = webs.webOf[caught]
			}
			d.unifyDescriptorIntJoins()
			got := d.opcodeIdToRef[initial][0][0].(*values.JavaRef)
			want := scenario == "proved" || scenario == "mutable graph changed"
			if (got != x) != want {
				t.Fatalf("canonicalized=%v want=%v", got != x, want)
			}
			if want && (d.opcodeIdToRef[first][0][0] != got || catchView.GetValue() != got || got.SolvedWebIdentity != got.Id) {
				t.Fatal("handler input definitions do not share their proved binder")
			}
			if d.opcodeIdToRef[last][0][0] != z || normalView.GetValue() != z || first.stackConsumed[0] != call1 || last.stackConsumed[0] != call2 {
				t.Fatal("changed normal output lifetime or moved a throwing producer")
			}
		})
	}
}
