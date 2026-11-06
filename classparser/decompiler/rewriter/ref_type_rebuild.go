package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/jdecenv"
)

// rebuildRefDefinition is the collected first-pass evidence for one source
// variable: its declaration reference and every non-null value assigned to it.
type rebuildRefDefinition struct {
	decl     *values.JavaRef
	declared types.JavaType
	values   []types.JavaType
}

// RebuildRefDeclarationTypes is the second pass of source-level slot typing.
//
// Pass 1 collects, per VarUid, the declaration type and every assigned value
// type. Pass 2 computes a common supertype and retypes the declaration only
// when every structural use accepts that candidate. Member/array receivers,
// ternary arms, lambda captures and opaque values veto a retype; a variable
// whose uses require a narrower member surface is left unchanged.
//
// This is deliberately separate from the Object/Number widen passes: those
// answer a fixed candidate question, while this pass derives the candidate from
// the actual definition set and hierarchy.
func RebuildRefDeclarationTypes(sts *[]statements.Statement, funcCtx *class_context.ClassContext) {
	if sts == nil || len(*sts) == 0 || jdecenv.Get("JDEC_REF_TYPE_REBUILD_OFF") != "" {
		return
	}
	definitions := collectRebuildDefinitions(*sts)
	if len(definitions) == 0 {
		return
	}
	provider := types.SuperTypeProvider(nil)
	if funcCtx != nil {
		provider = funcCtx.SiblingSuperTypes
	}
	for uid, definition := range definitions {
		if definition == nil || definition.decl == nil || definition.declared == nil {
			continue
		}
		if definition.decl.WebDeclType != nil || definition.decl.IsParam || definition.decl.IsThis {
			// A solved web already owns the declaration type; a parameter's type
			// is part of the method ABI and cannot be widened.
			continue
		}
		if isPrimitiveType(definition.declared) || definition.declared.IsArray() {
			continue
		}
		if _, known := types.ClassFQNOf(definition.declared); !known {
			continue
		}
		candidate := commonAssignedType(definition, provider)
		if candidate == nil || typeFQNEquals(candidate, definition.declared) {
			continue
		}
		if isPrimitiveType(candidate) || candidate.IsArray() {
			continue
		}
		if !candidateAcceptedByUses(candidate, collectRefUses(*sts, definition.decl, funcCtx), provider) {
			continue
		}
		setRefTypeByVarUid(*sts, uid, candidate)
	}
}

func collectRebuildDefinitions(sts []statements.Statement) map[string]*rebuildRefDefinition {
	out := map[string]*rebuildRefDefinition{}
	walkAssignments(sts, func(as *statements.AssignStatement) {
		if as == nil || as.ArrayMember != nil || as.LeftValue == nil {
			return
		}
		ref := unwrapJavaRef(as.LeftValue)
		if ref == nil || ref.VarUid == "" || ref.IsThis || ref.IsParam {
			return
		}
		definition := out[ref.VarUid]
		if definition == nil {
			definition = &rebuildRefDefinition{}
			out[ref.VarUid] = definition
		}
		if (as.IsFirst || as.IsDeclare) && definition.decl == nil {
			definition.decl = ref
			definition.declared = ref.Type()
		}
		if as.JavaValue == nil || values.IsNullLiteral(as.JavaValue) || as.JavaValue == values.JavaNull {
			return
		}
		valueType := as.JavaValue.Type()
		if valueType == nil || isPrimitiveType(valueType) || valueType.IsArray() {
			return
		}
		if _, known := types.ClassFQNOf(valueType); !known {
			return
		}
		definition.values = append(definition.values, valueType)
	})
	return out
}

func commonAssignedType(definition *rebuildRefDefinition, provider types.SuperTypeProvider) types.JavaType {
	if definition == nil || definition.declared == nil {
		return nil
	}
	candidate := definition.declared.Copy()
	seen := map[string]bool{}
	for _, valueType := range definition.values {
		if valueType == nil {
			continue
		}
		name, known := types.ClassFQNOf(valueType)
		if !known || seen[name] {
			continue
		}
		seen[name] = true
		// valueType must be assignable to the candidate declaration.
		if typeAcceptsValueWithProvider(valueType, candidate, provider) {
			continue
		}
		next := types.BridgedCommonSuperType(candidate, valueType, provider)
		if next == nil {
			return nil
		}
		candidate = next
	}
	if candidate == nil || isPrimitiveType(candidate) || candidate.IsArray() {
		return nil
	}
	if _, known := types.ClassFQNOf(candidate); !known {
		return nil
	}
	return candidate
}

func candidateAcceptedByUses(candidate types.JavaType, uses []refUse, provider types.SuperTypeProvider) bool {
	if candidate == nil {
		return false
	}
	objectType := types.NewJavaClass("java.lang.Object")
	numberType := types.NewJavaClass("java.lang.Number")
	for _, use := range uses {
		switch use.kind {
		case refUseWrite, refUseInstanceOf, refUseCast, refUseMonitor, refUseStringConcat:
			// No member surface required.
		case refUseAssignmentValue, refUseCallArgument, refUseReturn, refUseNewArrayElement:
			if !typeAcceptsValueWithProvider(candidate, use.expected, provider) {
				return false
			}
		case refUseEquality:
			if use.expected != nil && isPrimitiveType(use.expected) {
				return false
			}
		case refUseCallReceiver:
			switch {
			case typeFQNEquals(candidate, objectType):
				if !callReceiverObjectSafe(use.owner, use.member) {
					return false
				}
			case typeFQNEquals(candidate, numberType):
				if !callReceiverNumberSafe(use.owner, use.member) {
					return false
				}
			default:
				// A non-Object/Number receiver needs its exact member surface; the
				// old declaration may have been the only proof for that member.
				return false
			}
		default:
			// Field/array receivers, array length, switch selectors, throw,
			// numeric comparisons, ternary arms, lambda captures and opaque
			// values all require a more specific source shape.
			return false
		}
	}
	return true
}

// setRefTypeByVarUid rewrites every occurrence of one variable identity. The
// walker covers all statement lists and known value children; unknown opaque
// values are not traversed, but a variable used inside one was already vetoed
// by candidateAcceptedByUses.
func setRefTypeByVarUid(sts []statements.Statement, uid string, target types.JavaType) {
	if uid == "" || target == nil {
		return
	}
	var walkValue func(values.JavaValue, int)
	walkValue = func(value values.JavaValue, depth int) {
		if value == nil || depth > 4096 {
			return
		}
		for {
			slot, ok := value.(*values.SlotValue)
			if !ok || slot == nil {
				break
			}
			value = slot.GetValue()
			if value == nil {
				return
			}
		}
		if ref, ok := value.(*values.JavaRef); ok {
			if ref == nil {
				return
			}
			if ref.VarUid == uid {
				if ref.Type() != nil && !typeFQNEquals(ref.Type(), target) {
					ref.ResetVarType(target)
				}
				return
			}
			if ref.CustomValue != nil {
				walkValue(ref.CustomValue, depth+1)
			}
			return
		}
		children, known := values.Children(value)
		if !known {
			return
		}
		for _, child := range children {
			walkValue(child, depth+1)
		}
	}
	walkStatementListValues(sts, func(value values.JavaValue) {
		walkValue(value, 0)
	})
}
