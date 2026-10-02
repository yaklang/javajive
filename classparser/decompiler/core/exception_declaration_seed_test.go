package core

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestExceptionDeclarationSeedRequiresExactDeadDominatingDefinition(t *testing.T) {
	for _, name := range []string{"integer", "long", "reference", "observed seed", "entry web", "different type", "different category", "missing rhs", "typed nil rhs", "missing load", "incomplete snapshot", "stale load", "split target", "foreign alias", "unprotected target", "protected seed", "not handler read", "alternate seed", "entry path", "increment", "cycle", "budget"} {
		t.Run(name, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaInteger)
			storeCode, loadCode := OP_ISTORE_1, OP_ILOAD_1
			if name == "long" {
				typ = types.NewJavaPrimer(types.JavaLong)
				storeCode, loadCode = OP_LSTORE_1, OP_LLOAD_1
			}
			if name == "reference" {
				typ = types.NewJavaClass("java.lang.Object")
				storeCode, loadCode = OP_ASTORE_1, OP_ALOAD_1
			}
			seed, first, call, second, call2, done, handler, load, ret := op(storeCode, 1), op(storeCode, 3), op(OP_INVOKESTATIC, 5), op(storeCode, 7), op(OP_INVOKESTATIC, 9), op(OP_RETURN, 11), op(OP_ASTORE_2, 13), op(loadCode, 15), op(OP_IRETURN, 17)
			nodes := []*OpCode{seed, first, call, second, call2, done, handler, load, ret}
			g := &SemanticCFG{Nodes: nodes, incoming: map[*OpCode][]int{}, outgoing: map[*OpCode][]int{}}
			edge := func(a, b *OpCode, kind EdgeKind) {
				i := len(g.Edges)
				g.Edges = append(g.Edges, SemanticEdge{From: a, To: b, Kind: kind})
				g.incoming[b] = append(g.incoming[b], i)
				g.outgoing[a] = append(g.outgoing[a], i)
			}
			for _, pair := range [][2]*OpCode{{seed, first}, {first, call}, {call, second}, {second, call2}, {call2, done}, {handler, load}, {load, ret}} {
				edge(pair[0], pair[1], EdgeFallthrough)
			}
			edge(call, handler, EdgeException)
			edge(call2, handler, EdgeException)
			old := values.NewJavaRef(utils.NewRootVariableId(), nil, typ.Copy())
			canonical := values.NewJavaRef(utils.NewRootVariableId(), nil, typ.Copy())
			rhs := values.NewJavaLiteral(91, typ.Copy())
			seed.stackConsumed = []values.JavaValue{rhs}
			first.stackConsumed = []values.JavaValue{values.NewJavaLiteral(11, typ.Copy())}
			second.stackConsumed = []values.JavaValue{values.NewJavaLiteral(-7, typ.Copy())}
			use := values.NewSlotValue(canonical, typ.Copy())
			load.stackProduced = []values.JavaValue{use}
			webs := &slotWeb{webOf: map[*OpCode]int{seed: 1, first: 2, second: 2, load: 2}, entryWeb: map[int]int{}}
			row := &ExceptionTableEntry{StartPc: 3, EndPc: 11, HandlerPc: 13}
			d := &Decompiler{opCodes: nodes, semanticCFG: g, cachedSlotWebs: webs, ExceptionTable: []*ExceptionTableEntry{row}, opcodeIdToRef: map[*OpCode][][2]any{seed: {{old, true}}, first: {{canonical, true}}, second: {{canonical, false}}}}
			switch name {
			case "observed seed":
				g.Nodes = append(g.Nodes, op(loadCode, 2))
				webs.webOf[g.Nodes[len(g.Nodes)-1]] = 1
			case "entry web":
				webs.entryWeb[1] = 2
			case "different type":
				old.ResetVarType(types.NewJavaPrimer(types.JavaLong))
			case "different category":
				seed.Instr.OpCode = OP_LSTORE_1
			case "missing rhs":
				seed.stackConsumed = nil
			case "typed nil rhs":
				seed.stackConsumed = []values.JavaValue{(*values.JavaLiteral)(nil)}
			case "incomplete snapshot":
				webs.webOf[op(storeCode, 20)] = 2
			case "missing load":
				load.stackProduced = nil
			case "stale load":
				load.stackProduced = []values.JavaValue{old}
			case "split target":
				d.opcodeIdToRef[second][0][0] = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			case "foreign alias":
				other := op(storeCode, 19)
				webs.webOf[other] = 3
				d.opcodeIdToRef[other] = [][2]any{{canonical, true}}
			case "unprotected target":
				row.StartPc = 5
			case "protected seed":
				row.StartPc = 1
			case "not handler read":
				g.outgoing[handler] = nil
			case "alternate seed":
				other := op(storeCode, 2)
				edge(other, first, EdgeTaken)
			case "entry path":
				other := op(OP_NOP, 0)
				edge(other, first, EdgeTaken)
			case "increment":
				inc := op(OP_IINC, 2)
				inc.Data = []byte{1, 1}
				g.incoming[first] = nil
				edge(inc, first, EdgeFallthrough)
				edge(seed, inc, EdgeFallthrough)
			case "cycle":
				cycle := op(OP_NOP, 2)
				edge(cycle, first, EdgeTaken)
				edge(first, cycle, EdgeTaken)
			case "budget":
				g.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			}
			d.restoreExceptionDeclarationSeeds()
			repaired := d.opcodeIdToRef[seed][0][0] == canonical
			want := name == "integer" || name == "long" || name == "reference"
			if repaired != want {
				t.Fatalf("repair=%v want=%v", repaired, want)
			}
			if repaired && (d.opcodeIdToRef[first][0][1] != false || d.opcodeIdToRef[seed][0][1] != true || seed.stackConsumed[0] != rhs || use.GetValue() != canonical || webs.webOf[seed] == webs.webOf[first] || canonical.SolvedWebIdentity != canonical.Id) {
				t.Fatal("source seed moved/changed original RHS, semantic web, use binding, or declaration ownership")
			}
			if name == "budget" && g.Err == nil {
				t.Fatal("lost budget failure")
			}
		})
	}
}

// Source declaration seeding must not add a fake exceptional predecessor for
// the nonthrowing ISTORE. The handler still observes exactly stores at 3 and 8.
func TestExceptionSeedPreservesOriginalExceptionalBeforeState(t *testing.T) {
	d := auditCFG(t, []byte{OP_ICONST_0, OP_ISTORE_0, OP_ICONST_1, OP_ISTORE_0, OP_INVOKESTATIC, 0, 1, OP_ICONST_2, OP_ISTORE_0, OP_INVOKESTATIC, 0, 1, OP_RETURN, OP_ASTORE_1, OP_ILOAD_0, OP_IRETURN})
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 2, EndPc: 12, HandlerPc: 13, CatchType: 1}}
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	d.semanticCFG = g
	typ := types.NewJavaPrimer(types.JavaInteger)
	old := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	canon := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	d.opcodeIdToRef = map[*OpCode][][2]any{}
	at := func(pc uint16) *OpCode { return d.opCodes[d.offsetToOpcodeIndex[pc]] }
	for _, pc := range []uint16{1, 3, 8} {
		store := at(pc)
		ref := canon
		if pc == 1 {
			ref = old
		}
		store.stackConsumed = []values.JavaValue{values.NewJavaLiteral(int(pc), typ)}
		d.opcodeIdToRef[store] = [][2]any{{ref, pc != 8}}
	}
	at(14).stackProduced = []values.JavaValue{values.NewSlotValue(canon, typ)}
	before, entry := g.ReachingDefinitions(at(14), 0)
	if entry || len(before) != 2 || before[0] != at(3) || before[1] != at(8) {
		t.Fatal("bad original fixture reaching definitions")
	}
	webs := d.slotWebs()
	deadWeb, targetWeb := webs.webOf[at(1)], webs.webOf[at(3)]
	d.restoreExceptionDeclarationSeeds()
	after, entry := g.ReachingDefinitions(at(14), 0)
	if entry || len(after) != 2 || after[0] != before[0] || after[1] != before[1] || webs.webOf[at(1)] != deadWeb || webs.webOf[at(3)] != targetWeb || deadWeb == targetWeb {
		t.Fatal("changed original JVM exceptional state or partition")
	}
	if d.opcodeIdToRef[at(1)][0][0] != canon {
		t.Fatal("did not restore original source seed")
	}
}
