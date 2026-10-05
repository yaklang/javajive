package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A source-generated region must be reachable, dominate every normal return,
// and never execute again after its original consumer. Byte offsets do not
// establish this: an instrumented initializer may jump to a physically later
// prelude and then enter the region exactly once. Two graph cuts prove the
// property in linear work, retaining all original normal and exception edges.
func nativeEnumRegionExecutedOnce(ir *methodir.MethodIR, first, consumer uint16, work *workbudget.Budget, precedingConsumer ...uint16) bool {
	if ir == nil || int64(len(ir.Instrs)+len(ir.Edges)+1)*192 > 64<<20 || work != nil && work.CheckAlloc(int64(len(ir.Instrs)+len(ir.Edges)+1)*192) != nil || !nativeProofWork(work, int64(len(ir.Instrs)+len(ir.Edges))) {
		return false
	}
	adjacency := map[uint16][]uint16{}
	returns := map[uint16]bool{}
	for _, in := range ir.Instrs {
		if in.Opcode == core.OP_RETURN {
			returns[in.PC] = true
		}
	}
	for _, edge := range ir.Edges {
		adjacency[uint16(edge.From)] = append(adjacency[uint16(edge.From)], uint16(edge.To))
	}
	seen := map[uint16]bool{}
	stack := []uint16{ir.EntryPC}
	reachedFirst, reachedConsumer := false, false
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[n] {
			continue
		}
		seen[n] = true
		if !nativeProofWork(work, 1) {
			return false
		}
		if n == first {
			reachedFirst = true
		}
		if n == consumer {
			reachedConsumer = true
			continue
		}
		if returns[n] {
			return false
		}
		stack = append(stack, adjacency[n]...)
	}
	if !reachedFirst || !reachedConsumer {
		return false
	}
	seen = map[uint16]bool{}
	stack = append([]uint16(nil), adjacency[consumer]...)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[n] {
			continue
		}
		seen[n] = true
		if !nativeProofWork(work, 1) || n == first || n == consumer {
			return false
		}
		stack = append(stack, adjacency[n]...)
	}
	if len(precedingConsumer) > 1 {
		return false
	}
	if len(precedingConsumer) == 1 {
		// A canonical array evaluated too early contains null constants. The
		// preceding constant assignment must dominate its producer, not merely
		// share a source position or dominate the initializer's final return.
		seen = map[uint16]bool{}
		stack = []uint16{ir.EntryPC}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[n] {
				continue
			}
			seen[n] = true
			if !nativeProofWork(work, 1) || n == first {
				return false
			}
			if n == precedingConsumer[0] {
				continue
			}
			stack = append(stack, adjacency[n]...)
		}
	}

	return true
}
