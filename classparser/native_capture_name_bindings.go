package javaclassparser

import (
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A source name may denote distinct effectively-final declarations in disjoint
// lexical contours. Retain every prior identity and its visible declarations:
// neither binder may be visible in the other's contour.
// Checking both directions matters when an enclosing binder is captured after
// the nested declaration; checking all prior binders matters after a sibling.
type nativeCaptureNameBindings struct {
	bindings map[string]map[*coreutils.VariableId]map[*coreutils.VariableId]bool
	entries  int
}

func (names *nativeCaptureNameBindings) bind(name string, id *coreutils.VariableId, visible map[*coreutils.VariableId]bool, work *workbudget.Budget) bool {
	if names == nil || name == "" || id == nil || !visible[id] || len(visible) > 4096 || len(names.bindings) > 4096 || !nativeProofWork(work, int64(len(visible)+len(names.bindings[name])+1)) || work != nil && work.CheckAlloc(int64(len(visible)+1)*16) != nil {
		return false
	}
	for old, contour := range names.bindings[name] {
		if old != id && (visible[old] || contour[id]) {
			return false
		}
	}
	if len(names.bindings[name]) >= 4096 && names.bindings[name][id] == nil {
		return false
	}
	count := len(names.bindings[name][id])
	for declaration, present := range visible {
		if present && declaration == nil {
			return false
		}
		if present && !names.bindings[name][id][declaration] {
			count++
		}
	}
	added := count - len(names.bindings[name][id])
	if count > 4096 || names.entries+added > 65536 || work != nil && work.CheckAlloc(int64(names.entries+added)*64) != nil {
		return false
	}
	if names.bindings == nil {
		names.bindings = map[string]map[*coreutils.VariableId]map[*coreutils.VariableId]bool{}
	}
	if names.bindings[name] == nil {
		names.bindings[name] = map[*coreutils.VariableId]map[*coreutils.VariableId]bool{}
	}
	if names.bindings[name][id] == nil {
		names.bindings[name][id] = map[*coreutils.VariableId]bool{}
	}
	for declaration, present := range visible {
		if present {
			names.bindings[name][id][declaration] = true
		}
	}
	names.entries += added
	return true
}
