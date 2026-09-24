package core

import (
	"fmt"
	"strings"
	"testing"
)

func t10BuildCFG(t *testing.T, code []byte, table []*ExceptionTableEntry) *SemanticCFG {
	t.Helper()
	d := NewDecompiler(code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	d.ExceptionTable = table
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatalf("buildSemanticCFG: %v", err)
	}
	return g
}

func TestT10_C04_SharedHandlerAndTail(t *testing.T) {
	t.Run("T10-C04", testT10C04SharedHandler)
}
func testT10C04SharedHandler(t *testing.T) {
	// Two protected ranges share handler PC 10 and a normal tail (ireturn at 9).
	// 0 iconst_1; 1 istore_0; 2 iload_0; 3 iconst_1; 4 idiv; 5 istore_0;
	// 6 iload_0; 7 iconst_1; 8 idiv; 9 ireturn;
	// 10 astore_1; 11 iconst_2; 12 ireturn
	code := []byte{
		OP_ICONST_1, OP_ISTORE_0, OP_ILOAD_0, OP_ICONST_1, OP_IDIV, OP_ISTORE_0,
		OP_ILOAD_0, OP_ICONST_1, OP_IDIV, OP_IRETURN,
		OP_ASTORE_1, OP_ICONST_2, OP_IRETURN,
	}
	g := t10BuildCFG(t, code, []*ExceptionTableEntry{
		{StartPc: 2, EndPc: 6, HandlerPc: 10, CatchType: 0},
		{StartPc: 6, EndPc: 9, HandlerPc: 10, CatchType: 0},
	})
	ids := g.ExceptionEdgeIdentities()
	if len(ids) == 0 {
		t.Fatal("missing exception edges")
	}
	orders := map[int]int{}
	fromOrder := map[string]int{}
	for _, id := range ids {
		if id.ToPC != 10 {
			t.Fatalf("shared handler PC lost: %+v", id)
		}
		orders[id.HandlerOrder]++
		key := fmtPCOrder(id.FromPC, id.HandlerOrder)
		if _, dup := fromOrder[key]; dup {
			t.Fatalf("duplicated exception identity %+v", id)
		}
		fromOrder[key] = id.HandlerOrder
	}
	if orders[0] == 0 || orders[1] == 0 {
		t.Fatalf("HandlerOrder identity dropped: %v ids=%+v", orders, ids)
	}
	// Normal tail: ireturn at PC 9 is not swallowed into the handler.
	var hasTail bool
	for _, n := range g.Nodes {
		if n.CurrentOffset == 9 && n.Instr != nil && n.Instr.OpCode == OP_IRETURN {
			hasTail = true
		}
	}
	if !hasTail {
		t.Fatal("normal tail ireturn was dropped")
	}
}

func fmtPCOrder(pc uint16, order int) string {
	return fmt.Sprintf("%d:%d", pc, order)
}

func TestT10_C05_MonitorEnterNullHasNoExit(t *testing.T) {
	t.Run("T10-C05", testT10C05MonitorCFG)
}
func testT10C05MonitorCFG(t *testing.T) {
	// monitorenter on null; no handler and no monitorexit in the method.
	code := []byte{OP_ACONST_NULL, OP_MONITORENTER, OP_ICONST_1, OP_IRETURN}
	g := t10BuildCFG(t, code, nil)
	exits := 0
	enters := 0
	for _, n := range g.Nodes {
		if n.Instr == nil {
			continue
		}
		switch n.Instr.OpCode {
		case OP_MONITORENTER:
			enters++
		case OP_MONITOREXIT:
			exits++
		}
	}
	if enters != 1 {
		t.Fatalf("monitorenter count=%d", enters)
	}
	if exits != 0 {
		t.Fatal("invented monitorexit for an unacquired lock")
	}

	// Acquire-then-fail pair: monitorenter, athrow, handler monitorexit+athrow.
	// 0 aload_0; 1 astore_1; 2 aload_1; 3 monitorenter; 4 aconst_null; 5 athrow;
	// 6 aload_1; 7 monitorexit; 8 athrow
	pair := []byte{
		OP_ALOAD_0, OP_ASTORE_1, OP_ALOAD_1, OP_MONITORENTER, OP_ACONST_NULL, OP_ATHROW,
		OP_ALOAD_1, OP_MONITOREXIT, OP_ATHROW,
	}
	pg := t10BuildCFG(t, pair, []*ExceptionTableEntry{
		{StartPc: 4, EndPc: 6, HandlerPc: 6, CatchType: 0},
	})
	pairEnter, pairExit := 0, 0
	for _, n := range pg.Nodes {
		if n.Instr == nil {
			continue
		}
		switch n.Instr.OpCode {
		case OP_MONITORENTER:
			pairEnter++
		case OP_MONITOREXIT:
			pairExit++
		}
	}
	if pairEnter != 1 || pairExit != 1 {
		t.Fatalf("acquire-then-fail enter=%d exit=%d", pairEnter, pairExit)
	}
}

func TestT10_C06_OverlappingProtectedRangesDiagnostic(t *testing.T) {
	t.Run("T10-C06", testT10C06OverlapCFG)
}
func testT10C06OverlapCFG(t *testing.T) {
	// Crossing ranges: A=[2,10) B=[6,13) with throw sites unique to each side.
	// 0 iconst_1; 1 istore_0;
	// 2 iload_0; 3 iconst_1; 4 idiv; 5 istore_0;          // A only
	// 6 iload_0; 7 iconst_1; 8 idiv; 9 istore_0;          // A and B
	// 10 iload_0; 11 iconst_1; 12 idiv; 13 ireturn;       // B only
	// 14 astore_1; 15 iconst_1; 16 ireturn;
	// 17 astore_1; 18 iconst_2; 19 ireturn
	code := []byte{
		OP_ICONST_1, OP_ISTORE_0,
		OP_ILOAD_0, OP_ICONST_1, OP_IDIV, OP_ISTORE_0,
		OP_ILOAD_0, OP_ICONST_1, OP_IDIV, OP_ISTORE_0,
		OP_ILOAD_0, OP_ICONST_1, OP_IDIV, OP_IRETURN,
		OP_ASTORE_1, OP_ICONST_1, OP_IRETURN,
		OP_ASTORE_1, OP_ICONST_2, OP_IRETURN,
	}
	d := NewDecompiler(code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	d.ExceptionTable = []*ExceptionTableEntry{
		{StartPc: 2, EndPc: 10, HandlerPc: 14, CatchType: 1},
		{StartPc: 6, EndPc: 13, HandlerPc: 17, CatchType: 1},
	}
	_, err := d.buildSemanticCFG()
	if err == nil {
		t.Fatal("crossing protected ranges were silently accepted")
	}
	if !strings.Contains(err.Error(), "unsupported_overlapping_protected_ranges") &&
		!strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("missing unsupported diagnostic: %v", err)
	}
}
