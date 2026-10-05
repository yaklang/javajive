package core

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func monitorSnapshotBytecode() ([]byte, []*ExceptionTableEntry) {
	return []byte{OP_ALOAD_0, OP_DUP, OP_ASTORE_1, OP_MONITORENTER, OP_ALOAD_1, OP_MONITOREXIT, OP_RETURN, OP_ASTORE_2, OP_ALOAD_1, OP_MONITOREXIT, OP_ALOAD_2, OP_ATHROW}, []*ExceptionTableEntry{{StartPc: 4, EndPc: 6, HandlerPc: 7}, {StartPc: 7, EndPc: 10, HandlerPc: 7}}
}
func TestOriginalMonitorSnapshotNeedsUniqueHeldReachingDefinition(t *testing.T) {
	for _, scenario := range []string{"paired", "wrong normal slot", "wrong handler slot", "no snapshot DUP", "snapshot type differs", "missing handler", "acquisition protected", "release unprotected", "exit replaced by POP", "double release", "no CFG", "work cap", "memory cap", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			code, table := monitorSnapshotBytecode()
			switch scenario {
			case "wrong normal slot":
				code[4] = OP_ALOAD_0
			case "wrong handler slot":
				code[8] = OP_ALOAD_0
			case "no snapshot DUP":
				code[1] = OP_NOP
			case "snapshot type differs":
				code[2] = OP_ISTORE_1
			case "missing handler":
				table = nil
			case "acquisition protected":
				table[0].StartPc = 3
			case "release unprotected":
				table[0].EndPc = 5
			case "exit replaced by POP":
				code[5] = OP_POP
			case "double release":
				code = []byte{OP_ALOAD_0, OP_DUP, OP_ASTORE_1, OP_MONITORENTER, OP_ALOAD_1, OP_MONITOREXIT, OP_ALOAD_1, OP_MONITOREXIT, OP_RETURN, OP_ASTORE_2, OP_ALOAD_1, OP_MONITOREXIT, OP_ALOAD_2, OP_ATHROW}
				table = []*ExceptionTableEntry{{StartPc: 4, EndPc: 8, HandlerPc: 9}, {StartPc: 9, EndPc: 12, HandlerPc: 9}}
			}
			d := NewDecompiler(code, nil)
			if e := d.ParseOpcode(); e != nil {
				t.Fatal(e)
			}
			d.ExceptionTable = table
			g, e := d.buildSemanticCFG()
			if e != nil {
				t.Fatal(e)
			}
			d.semanticCFG = g
			switch scenario {
			case "no CFG":
				d.semanticCFG = nil
			case "work cap":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory cap":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := d.originalMonitorOwners()
			if scenario != "paired" {
				if len(got) != 0 {
					t.Fatalf("unproved ownership admitted: %v", got)
				}
				return
			}
			if len(got) != 3 {
				t.Fatalf("ownership cardinality %d", len(got))
			}
			expected := map[uint16]bool{3: true, 5: true, 9: true}
			for op, owner := range got {
				if !expected[op.CurrentOffset] || owner != 3 {
					t.Fatalf("wrong PC/owner %d/%d", op.CurrentOffset, owner)
				}
			}
		})
	}
}
func TestOriginalNestedMonitorSnapshotHonorsFirstCatchAll(t *testing.T) {
	code := []byte{OP_ALOAD_0, OP_DUP, OP_ASTORE_1, OP_MONITORENTER, OP_ALOAD_0, OP_DUP, OP_ASTORE_2, OP_MONITORENTER, OP_ALOAD_2, OP_MONITOREXIT, OP_ALOAD_1, OP_MONITOREXIT, OP_RETURN, OP_ASTORE_3, OP_ALOAD_2, OP_MONITOREXIT, OP_ALOAD_3, OP_ATHROW, OP_ASTORE_3, OP_ALOAD_1, OP_MONITOREXIT, OP_ALOAD_3, OP_ATHROW}
	table := []*ExceptionTableEntry{{StartPc: 8, EndPc: 10, HandlerPc: 13}, {StartPc: 13, EndPc: 16, HandlerPc: 13}, {StartPc: 4, EndPc: 12, HandlerPc: 18}, {StartPc: 13, EndPc: 18, HandlerPc: 18}, {StartPc: 18, EndPc: 21, HandlerPc: 18}}
	for _, shadowedFirst := range []bool{false, true} {
		d := NewDecompiler(code, nil)
		if e := d.ParseOpcode(); e != nil {
			t.Fatal(e)
		}
		if shadowedFirst {
			table = append([]*ExceptionTableEntry{table[2]}, append(table[:2:2], table[3:]...)...)
		}
		d.ExceptionTable = table
		g, e := d.buildSemanticCFG()
		if e != nil {
			t.Fatal(e)
		}
		d.semanticCFG = g
		got := d.originalMonitorOwners()
		byOwner := map[int]int{}
		for _, owner := range got {
			byOwner[owner]++
		}
		if !shadowedFirst && (byOwner[3] != 3 || byOwner[7] != 3) {
			t.Fatalf("nested owners: %v", byOwner)
		}
		if shadowedFirst && byOwner[7] != 0 {
			t.Fatal("shadowed inner handler invented a release")
		}
	}
}
