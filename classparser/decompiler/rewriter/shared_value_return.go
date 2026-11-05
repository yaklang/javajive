package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	utils2 "github.com/yaklang/javajive/classparser/decompiler/utils"
)

// A shared return is an abrupt completion on each incoming edge. An edge-local
// source copy preserves its evaluation count and bound operands, provided it
// cannot create a different lambda/anonymous-class identity and cannot move an
// effect into or out of any original handler. This is not value speculation:
// the return expression stays after the selected branch, including its calls.
func sharedReturnCanFollowEdge(manager *RewriteManager, from, target *core.Node, ret *statements.ReturnStatement) bool {
	if manager == nil || !manager.originalHandlersKnown || from == nil || target == nil ||
		ret == nil || ret.JavaValue == nil || !from.HasOriginPC || !forwardOriginalMethodExit(target) ||
		len(manager.originalHandlers) > 512 || !sharedReturnCopyStable(ret.JavaValue) {
		return false
	}
	// A raw try owner may still dominate its physically unprotected cleanup
	// tail. Original effect membership alone does not make that tail a source
	// body: cloning its shared return now would let the body collector consume
	// it and prevent the original finally from being recovered once. Wait for
	// that exception boundary to be structured before giving this edge a copy.
	if len(manager.TryNodes) > 256 || (len(manager.TryNodes) > 0 && manager.DominatorMap == nil) {
		return false
	}
	for _, owner := range manager.TryNodes {
		if owner == nil || !utils2.IsDominate(manager.DominatorMap, owner, from) {
			continue
		}
		if !owner.HasProtectedRange || owner.ProtectedStartPC < 0 || owner.ProtectedStartPC >= owner.ProtectedEndPC || owner.ProtectedEndPC > 65536 || len(owner.SharedProtectedRanges) > 512 {
			return false
		}
		covered := from.OriginPC >= owner.ProtectedStartPC && from.OriginPC < owner.ProtectedEndPC
		if len(owner.SharedProtectedRanges) > 0 {
			covered = false
			for _, row := range owner.SharedProtectedRanges {
				if row.StartPc >= row.EndPc {
					return false
				}
				covered = covered || from.OriginPC >= int(row.StartPc) && from.OriginPC < int(row.EndPc)
			}
		}
		if !covered {
			return false
		}
	}
	remaining := 65536
	covered := func(pc int) bool {
		if pc < 0 || pc > 65535 || from.OriginPC < 0 || from.OriginPC > 65535 {
			return false
		}
		remaining -= len(manager.originalHandlers) + 1
		if remaining < 0 {
			return false
		}
		for _, row := range manager.originalHandlers {
			inside := func(at int) bool { return at >= int(row.StartPc) && at < int(row.EndPc) }
			if inside(pc) != inside(from.OriginPC) {
				return false
			}
		}
		return true
	}
	return covered(target.OriginPC) && finallyCoveredValue(ret.JavaValue, covered)
}

func sharedReturnCopyStable(value values.JavaValue) bool {
	// Original primitive conversions and checkcasts retain their captured
	// operand; an arbitrary CustomValue/render callback supplies no proof.
	for depth := 0; depth < 16; depth++ {
		switch v := value.(type) {
		case *values.SlotValue:
			if v == nil {
				return false
			}
			value = v.GetValue()
		case *values.CustomValue:
			operand, _, _, known := v.SourcePrimitiveConversion()
			if !known {
				return false
			}
			value = operand
		case *values.CastExpression:
			if v == nil || !v.OriginalCheckCast {
				return false
			}
			value = v.Value
		case *values.FunctionCallExpression:
			// A following primitive-conversion coverage query may inspect
			// this call's declared result. Missing type metadata is not a
			// certificate, even if a descriptor string happens to be present.
			return v != nil && v.FuncType != nil && v.FuncType.ReturnType != nil && switchStepCopyStableValue(value)
		default:
			return switchStepCopyStableValue(value)
		}
	}
	return false
}
