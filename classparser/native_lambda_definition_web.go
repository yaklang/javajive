package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Physical slots can be reused by earlier disjoint lifetimes. Admit a branch
// definition web only if every original overlapping read has a closed frontier
// and none joins a captured definition with another lifetime. IINC's input and
// new definition participate together; wide interiors cannot borrow this proof.
// Final source publication still checks every retained captured STORE and its
// snapshot. This is not permission to treat equal types as equal values.
func nativeLambdaLocalDefinitionWebClosed(flow *nativeEnumParameterFlow, ops []*core.OpCode, read *nativeEnumLocalRead, work *workbudget.Budget) bool {
	if flow == nil || read == nil || read.storePC != -1 || len(read.storePCs) < 2 || len(read.storePCs) > 64 || len(ops) > 8192 || work != nil && work.CheckAlloc(int64(len(ops))*256+8192) != nil {
		return false
	}
	width := 1
	if read.descriptor == "J" || read.descriptor == "D" {
		width = 2
	}
	expected, found := map[int]bool{}, map[int]bool{}
	for _, pc := range read.storePCs {
		if pc < 0 || pc > 65535 || expected[pc] {
			return false
		}
		expected[pc] = true
	}
	for _, op := range ops {
		if op == nil || op.Instr == nil || !nativeProofWork(work, 1) {
			return false
		}
		access := core.LocalAccessOf(op.Instr.OpCode)
		if !access.Read && !access.Write {
			continue
		}
		slot := core.GetRetrieveIdx(op)
		if access.Write {
			slot = core.GetStoreIdx(op)
		}
		if slot < 0 || access.Width < 1 || access.Width > 2 {
			return false
		}
		if slot >= read.slot+width || read.slot >= slot+access.Width {
			continue
		}
		if slot != read.slot || access.Width != width {
			return false
		}
		pc := int(op.CurrentOffset)
		if access.Write && expected[pc] {
			if access.Read || found[pc] {
				return false
			}
			found[pc] = true
		}
		if !access.Read {
			continue
		}
		descriptor := ""
		for _, candidate := range []string{"I", "J", "F", "D", "Ljava/lang/Object;"} {
			if constructorMotionLoad(op, candidate) {
				descriptor = candidate
				break
			}
		}
		if access.Write {
			descriptor = "I"
		} // IINC reads and writes one int word.
		if descriptor == "" {
			return false
		}
		query := &nativeEnumSelectorProducer{pc: pc, opcode: op.Instr.OpCode, slot: slot, result: descriptor}
		frontier, closed := flow.reachingLocalStores(query, work, access.Write)
		if !closed {
			return false
		}
		inside, outside := false, access.Write && !expected[pc]
		for _, writer := range frontier {
			inside = inside || expected[writer]
			outside = outside || !expected[writer]
		}
		if inside && outside {
			return false
		}
	}
	return len(found) == len(expected)
}
