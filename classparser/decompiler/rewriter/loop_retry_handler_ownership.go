package rewriter

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

// Split exception-table rows can still have one private logical retry owner.
// That ownership is independent of the number of successful normal exits: a
// timed operation may either break to a continuation or return through cleanup.
// Reopen the complete original handler rows instead of requiring those normal
// arms to converge before admitting the protected path into the loop.
func splitRetryHandlerOwnershipClosed(region, handler *core.Node) bool {
	if region == nil || handler == nil || !region.SharedProtectedHandler || !region.HasProtectedRange || region.ProtectedStartPC < 0 || region.ProtectedEndPC <= region.ProtectedStartPC || len(region.SharedProtectedRanges) < 2 || len(region.SharedProtectedRanges) > 32 || !slices.Contains(region.Next, handler) || !handler.IsCatchStart || len(handler.Source) != 1 || handler.Source[0] != region || handler.CatchHandler == nil || handler.CatchHandler.CatchAll || !handler.HasOriginPC || handler.OriginPC != handler.CatchHandler.EntryPC {
		return false
	}
	statement, ok := region.Statement.(*statements.MiddleStatement)
	if !ok || statement == nil || statement.Flag != statements.MiddleTryStart {
		return false
	}
	catchType := region.SharedProtectedRanges[0].CatchType
	if catchType == 0 {
		return false
	}
	rows := make([][2]int, 0, len(region.SharedProtectedRanges))
	previousEnd := -1
	for _, row := range region.SharedProtectedRanges {
		if int(row.StartPc) < previousEnd || row.StartPc >= row.EndPc {
			return false
		}
		previousEnd = int(row.EndPc)
		if int(row.HandlerPc) != handler.CatchHandler.EntryPC || row.CatchType != catchType {
			return false
		}
		rows = append(rows, [2]int{int(row.StartPc), int(row.EndPc)})
	}
	if !slices.Contains(rows, [2]int{region.ProtectedStartPC, region.ProtectedEndPC}) {
		return false
	}
	ranges, valid := canonicalHandlerRanges(rows)
	original, known := canonicalHandlerRanges(handler.CatchHandler.ProtectedRanges)
	return valid && known && slices.Equal(ranges, original)
}

// Separate handler ownership from the successful path frontier. Only the
// original protected statements and empty gap transfers enter the cyclic
// region; cleanup and downstream calls stay outside. Multiple normal exits
// are legal, but foreign entries, nested regions and mixed-domain operands
// cannot establish this proof.
func splitRetryProtectedFrontiers(region, header *core.Node) ([]*core.Node, []*core.Node) {
	if region == nil {
		return nil, nil
	}
	var start, handler *core.Node
	for _, next := range region.Next {
		if next == nil {
			return nil, nil
		}
		if next.IsCatchStart {
			if next.CatchHandler != nil && !next.CatchHandler.CatchAll {
				if handler != nil {
					return nil, nil
				}
				handler = next
			}
		} else {
			if start != nil {
				return nil, nil
			}
			start = next
		}
	}
	if start == nil || !splitRetryHandlerOwnershipClosed(region, handler) {
		return nil, nil
	}
	ranges := handler.CatchHandler.ProtectedRanges
	covered := func(pc int) bool { return finallyContains(ranges, pc) }
	endPC := int(region.SharedProtectedRanges[len(region.SharedProtectedRanges)-1].EndPc)
	state := map[*core.Node]uint8{}
	var path, boundaries []*core.Node
	var visit func(*core.Node) bool
	visit = func(node *core.Node) bool {
		if node == nil || node == header || node == region || node.IsCatchStart || node.IsJmp || !node.HasOriginPC || node.HasProtectedRange || len(node.EncodedJumps) != 0 {
			return false
		}
		if node.OriginPC >= endPC {
			// A folded operand can still be protected even when its consumer
			// is after the exclusive end. Such a mixed-domain statement must
			// not become an outside frontier merely by its final opcode PC.
			outside := func(pc int) bool { return pc >= 0 && !covered(pc) }
			switch statement := node.Statement.(type) {
			case *statements.ConditionStatement:
				if statement == nil || !finallyCoveredValue(statement.Condition, outside) {
					return false
				}
			case *statements.GOTOStatement:
				if statement == nil || len(node.Next) != 1 {
					return false
				}
			default:
				proof := outsideHandlerDomainProof(ranges)
				if !proof.block([]statements.Statement{node.Statement}, outside, 0) {
					return false
				}
			}
			boundaries = append(boundaries, node)
			return len(boundaries) <= 64
		}
		if node.OriginPC < region.ProtectedStartPC || len(state) >= 512 {
			return false
		}
		if state[node] != 0 {
			return state[node] == 2
		}
		state[node] = 1
		switch statement := node.Statement.(type) {
		case *statements.ConditionStatement:
			if statement == nil || len(node.Next) != 2 || !covered(node.OriginPC) || !finallyCoveredValue(statement.Condition, covered) {
				return false
			}
		case *statements.AssignStatement, *statements.ExpressionStatement:
			proof := handlerLayerProof{remaining: 512, active: map[statements.Statement]bool{}}
			if len(node.Next) != 1 || !covered(node.OriginPC) || !proof.block([]statements.Statement{statement}, covered, 0) {
				return false
			}
		case *statements.GOTOStatement:
			if statement == nil || len(node.Next) != 1 {
				return false
			}
		default:
			return false
		}
		path = append(path, node)
		for _, next := range node.Next {
			if !visit(next) {
				return false
			}
		}
		state[node] = 2
		return true
	}
	if !visit(start) || len(boundaries) == 0 {
		return nil, nil
	}
	for _, node := range path {
		if len(node.Source) == 0 {
			return nil, nil
		}
		for _, source := range node.Source {
			if source != region && state[source] != 2 {
				return nil, nil
			}
		}
	}
	return path, NodeDeduplication(boundaries)
}
