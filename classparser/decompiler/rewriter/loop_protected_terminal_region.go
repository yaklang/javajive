package rewriter

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	utils2 "github.com/yaklang/javajive/internal/utils"
)

// Exceptional edges are represented at a try entry, so reverse reachability
// from a retry latch omits successful terminal arms. A split exception table
// (for example, a continue gap) does not make those arms normal continuations.
// Keep a closed normal subgraph in the loop when every path either retries
// this header, returns a protected expression, or reaches one fully unprotected
// terminal continuation. RETURN itself may be the exclusive boundary; NEW, constructor calls
// and nested arguments must still carry their own covered producer PCs.
//
// This changes region membership only. It neither widens an exception range
// nor changes the CFG, expression, handler order or transfer target. Unknown
// sinks, mixed domains, shared entrances and nested regions remain unproved.
type protectedRetryRegion struct {
	path         []*core.Node
	continuation *core.Node
}

func protectedRetryClosedRegion(region, header *core.Node, cyclic *utils2.Set[*core.Node]) *protectedRetryRegion {
	if region == nil || header == nil || cyclic == nil || !cyclic.Has(region) ||
		!region.HasProtectedRange || region.ProtectedStartPC < 0 || region.ProtectedStartPC >= region.ProtectedEndPC {
		return nil
	}
	middle, ok := region.Statement.(*statements.MiddleStatement)
	if !ok || middle == nil || middle.Flag != statements.MiddleTryStart || len(region.Next) != 2 {
		return nil
	}
	var start, handler *core.Node
	for _, next := range region.Next {
		if next == nil {
			return nil
		}
		if next.IsCatchStart {
			handler = next
		} else {
			start = next
		}
	}
	if start == nil || handler == nil || !cyclic.Has(handler) || len(handler.Source) != 1 || handler.Source[0] != region ||
		handler.CatchHandler == nil || handler.CatchHandler.CatchAll || !handler.HasOriginPC || handler.OriginPC != handler.CatchHandler.EntryPC {
		return nil
	}
	rows := [][2]int{{region.ProtectedStartPC, region.ProtectedEndPC}}
	if region.SharedProtectedHandler {
		if len(region.SharedProtectedRanges) < 2 || len(region.SharedProtectedRanges) > 32 {
			return nil
		}
		rows = nil
		catchType := region.SharedProtectedRanges[0].CatchType
		if catchType == 0 {
			return nil
		}
		for _, row := range region.SharedProtectedRanges {
			if int(row.HandlerPc) != handler.CatchHandler.EntryPC || row.CatchType != catchType {
				return nil
			}
			rows = append(rows, [2]int{int(row.StartPc), int(row.EndPc)})
		}
		if !slices.Contains(rows, [2]int{region.ProtectedStartPC, region.ProtectedEndPC}) {
			return nil
		}
	}
	ranges, known := canonicalHandlerRanges(rows)
	original, originalKnown := canonicalHandlerRanges(handler.CatchHandler.ProtectedRanges)
	if !known || !originalKnown || !slices.Equal(ranges, original) {
		return nil
	}
	covered := func(pc int) bool { return finallyContains(ranges, pc) }
	state := map[*core.Node]uint8{}
	var path []*core.Node
	var continuation *core.Node
	returns := 0
	var visit func(*core.Node) bool
	visit = func(node *core.Node) bool {
		if node == header {
			return true
		}
		if node == nil || node == region || node.IsCatchStart || node.IsJmp || node.HasProtectedRange ||
			!node.HasOriginPC || len(node.EncodedJumps) != 0 || len(state) >= 512 {
			return false
		}
		if state[node] != 0 {
			return state[node] == 2
		}
		// A downstream return is a real continuation only if its complete
		// operand is outside EVERY row. A NEW outside one row with a protected
		// constructor (or vice versa) is mixed, and cannot prove either role.
		if ret, ok := node.Statement.(*statements.ReturnStatement); ok && ret != nil &&
			ret.HasOriginPC && ret.OriginPC == node.OriginPC && !covered(node.OriginPC) {
			terminal := true
			for _, next := range node.Next {
				terminal = terminal && IsEndNode(next)
			}
			proof := outsideHandlerDomainProof(ranges)
			if terminal && proof.block([]statements.Statement{ret}, func(pc int) bool { return pc >= 0 && !covered(pc) }, 0) {
				if continuation != nil && continuation != node {
					return false
				}
				continuation = node
				return true
			}
		}
		state[node] = 1
		path = append(path, node)
		switch statement := node.Statement.(type) {
		case *statements.ReturnStatement:
			if statement == nil || statement.JavaValue == nil || !statement.HasOriginPC || statement.OriginPC != node.OriginPC ||
				!finallyCoveredValue(statement.JavaValue, covered) {
				return false
			}
			if !covered(node.OriginPC) && !slices.ContainsFunc(ranges, func(row [2]int) bool { return row[1] == node.OriginPC }) {
				return false
			}
			effects, _ := values.InspectValue(statement.JavaValue)
			if effects&values.EffectThrow == 0 {
				return false // Pure returns do not require this ownership repair.
			}
			for _, next := range node.Next {
				if !IsEndNode(next) {
					return false
				}
			}
			returns++
			state[node] = 2
			return true
		case *statements.ConditionStatement:
			if statement == nil || len(node.Next) != 2 || !covered(node.OriginPC) || !finallyCoveredValue(statement.Condition, covered) {
				return false
			}
		case *statements.AssignStatement, *statements.ExpressionStatement:
			proof := handlerLayerProof{remaining: 512, active: map[statements.Statement]bool{}}
			if len(node.Next) != 1 || !covered(node.OriginPC) || !proof.block([]statements.Statement{statement}, covered, 0) {
				return false
			}
		default:
			return false
		}
		for _, next := range node.Next {
			if !visit(next) {
				return false
			}
		}
		state[node] = 2
		return true
	}
	// The private handler already belongs to the reverse-reachable cyclic
	// body. It proves a retry path even when every NORMAL path returns; an
	// additional explicit continue in the try is not required.
	if !visit(start) || returns == 0 && continuation == nil {
		return nil
	}
	for _, node := range path {
		if len(node.Source) == 0 {
			return nil
		}
		for _, source := range node.Source {
			if source != region && state[source] != 2 {
				return nil
			}
		}
	}
	return &protectedRetryRegion{path: path, continuation: continuation}
}
