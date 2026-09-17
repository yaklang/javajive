package core

import (
	"reflect"
	"testing"
)

func auditCFG(t *testing.T, code []byte) *Decompiler {
	t.Helper()
	d := NewDecompiler(code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	return d
}
func TestSemanticCFGThrowSites(t *testing.T) {
	// x=0; x=1; invokestatic; x=2; invokestatic; return; handler: astore_1; iload_0; ireturn
	d := auditCFG(t, []byte{OP_ICONST_0, OP_ISTORE_0, OP_ICONST_1, OP_ISTORE_0, OP_INVOKESTATIC, 0, 1, OP_ICONST_2, OP_ISTORE_0, OP_INVOKESTATIC, 0, 1, OP_RETURN, OP_ASTORE_1, OP_ILOAD_0, OP_IRETURN})
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 2, EndPc: 12, HandlerPc: 13, CatchType: 1}}
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	load := d.opCodes[d.offsetToOpcodeIndex[14]]
	defs, entry := g.ReachingDefinitions(load, 0)
	offsets := []uint16{}
	for _, def := range defs {
		offsets = append(offsets, def.CurrentOffset)
	}
	if entry || !reflect.DeepEqual(offsets, []uint16{3, 8}) {
		t.Fatalf("handler definitions: %v entry=%v", offsets, entry)
	}
	updates := g.Updates
	g.ReachingDefinitions(load, 0)
	if g.Updates != updates {
		t.Fatal("unchanged graph recomputed")
	}
	var throwSites []uint16
	for _, edge := range g.Edges {
		if edge.Kind == EdgeException {
			throwSites = append(throwSites, edge.From.CurrentOffset)
		}
	}
	if !reflect.DeepEqual(throwSites, []uint16{4, 9}) {
		t.Fatalf("throw sites %v", throwSites)
	}
}

func TestSemanticCFGLoopIincAndBudget(t *testing.T) {
	// iconst_0; istore_0; iinc 0,1; iload_0; ifne -4; return
	d := auditCFG(t, []byte{OP_ICONST_0, OP_ISTORE_0, OP_IINC, 0, 1, OP_ILOAD_0, OP_IFNE, 255, 252, OP_RETURN})
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	inc := d.opCodes[d.offsetToOpcodeIndex[2]]
	defs, entry := g.ReachingDefinitions(inc, 0)
	if entry || len(defs) != 2 || defs[0].CurrentOffset != 1 || defs[1] != inc {
		t.Fatalf("loop phi inputs %v entry=%v", defs, entry)
	}
	// Query order and edge enumeration do not change the fixed point.
	for _, edges := range g.outgoing {
		for i, j := 0, len(edges)-1; i < j; i, j = i+1, j-1 {
			edges[i], edges[j] = edges[j], edges[i]
		}
	}
	g.slots = map[int]*slotDefinitions{}
	again, e := g.ReachingDefinitions(inc, 0)
	if e != entry || !reflect.DeepEqual(again, defs) {
		t.Fatal("unstable definitions")
	}
	g.slots = map[int]*slotDefinitions{}
	g.MaxUpdates = 1
	g.ReachingDefinitions(inc, 0)
	if g.Err == nil {
		t.Fatal("analysis budget did not report exhaustion")
	}
}

func TestSemanticCFGCategory2Overlap(t *testing.T) {
	d := auditCFG(t, []byte{OP_LCONST_0, OP_LSTORE_0, OP_ICONST_1, OP_ISTORE_1, OP_RETURN})
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	defs, invalid := g.ReachingDefinitions(g.Nodes[len(g.Nodes)-1], 0)
	if len(defs) != 0 || !invalid {
		t.Fatalf("overwritten category-2 value survives: %v invalid=%v", defs, invalid)
	}
}
