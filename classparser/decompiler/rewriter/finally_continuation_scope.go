package rewriter

import (
	"reflect"
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A source-only finally view can expose a continuation after declaration
// placement has finished. Move only a bare declaration across that lexical
// boundary. Its original initializer and all subsequent stores stay inside
// the original handlers, at the same positions and with the same identities.
// A top-level initializing definition is reached on every normal try exit;
// retained catches must exit the method rather than acquire its continuation.
func finallyContinuationDeclarationScope(tr *statements.TryCatchStatement, tail []statements.Statement, catches [][]statements.Statement) (*statements.TryCatchStatement, []statements.Statement, bool) {
	if tr == nil || len(tr.TryBody) > 128 || len(tail) == 0 || len(catches) > 32 {
		return nil, nil, false
	}
	for _, body := range catches {
		if !finallyMethodExit(body, 0) {
			return nil, nil, false
		}
	}
	// Opaque source closures can introduce unseen binders, declarations or
	// transfers. They cannot authorize a new source scope.
	if !finallyCleanupBindingsScoped(tr.TryBody, nil, nil) {
		return nil, nil, false
	}
	clone := *tr
	clone.TryBody = nil
	var declarations []statements.Statement
	seen := map[*utils.VariableId]bool{}
	for _, statement := range tr.TryBody {
		clone.TryBody = append(clone.TryBody, statement)
		store, initializing := statement.(*statements.AssignStatement)
		if !initializing || store == nil || store.ArrayMember != nil {
			continue
		}
		bare := store.IsDeclare && !store.IsFirst && store.JavaValue == nil && !store.HasOriginPC
		if !bare && (!store.IsFirst || store.IsDeclare || store.JavaValue == nil || !store.HasOriginPC) {
			continue
		}
		ref, local := plainTryValue(store.LeftValue).(*values.JavaRef)
		if !local || ref == nil || ref.Id == nil || ref.IsParam || ref.IsThis || !finallyPureLocal(ref) || ref.Type() == nil {
			continue
		}
		if bare {
			// An already separated declaration has no operation to move. Its
			// decoded STORE must still exist in the actual body, possibly in a
			// nested region. Value history or entry-initializer metadata alone
			// cannot license a new source binding.
			if !finallyScopeRetainsOriginalDefinition(ref, tr.TryBody) {
				continue
			}
		} else if ref.WebDeclType != nil {
			if !reflect.DeepEqual(ref.WebDeclType.RawType(), ref.Type().RawType()) {
				continue
			}
		} else if !finallyScopeOriginalDefinition(ref, store) {
			continue
		}
		binding := map[*utils.VariableId]int{ref.Id: 1}
		if finallyCleanupBindingsScoped(tail, nil, binding) {
			continue
		}
		if seen[ref.Id] || len(declarations) >= 32 || !bare && !finallyCleanupBindingsScoped([]statements.Statement{statements.NewExpressionStatement(store.JavaValue)}, nil, binding) {
			return nil, nil, false
		}
		seen[ref.Id] = true
		if bare {
			clone.TryBody = clone.TryBody[:len(clone.TryBody)-1]
		} else {
			assignment := *store
			assignment.IsFirst, assignment.IsDeclare, assignment.Final = false, false, false
			clone.TryBody[len(clone.TryBody)-1] = &assignment
		}
		declarations = append(declarations, &statements.AssignStatement{LeftValue: ref, IsDeclare: true, Final: store.Final})
	}
	if len(declarations) == 0 || !finallyContinuationKeepsLexicalBindings([]statements.Statement{&clone}, tail) {
		return nil, nil, false
	}
	return &clone, declarations, true
}

// Reopen both independent decoder receipts at the actual source assignment.
// Requiring the unchanged source/producer type avoids inferring a declaration
// from a later narrowed or widened view when no solved web type is available.
func finallyScopeOriginalDefinition(ref *values.JavaRef, store *statements.AssignStatement) bool {
	if ref == nil || store == nil || store.LeftValue != ref || store.IsDeclare || store.JavaValue == nil || store.JavaValue.Type() == nil || ref.Type() == nil ||
		!reflect.DeepEqual(ref.Type().RawType(), store.JavaValue.Type().RawType()) {
		return false
	}
	pc, slot, known := store.OriginalLocalStore()
	declPC, declSlot, declaration := ref.OriginalLocalDeclaration(store.JavaValue)
	return known && declaration && pc == declPC && slot == declSlot
}

func finallyScopeRetainsOriginalDefinition(ref *values.JavaRef, body []statements.Statement) bool {
	queue := slices.Clone(body)
	seen := map[statements.Statement]bool{}
	matches := 0
	for len(queue) > 0 {
		if len(queue) > 512 || len(seen) >= 256 {
			return false
		}
		x := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if x == nil || seen[x] {
			return false
		}
		seen[x] = true
		switch st := x.(type) {
		case *statements.AssignStatement:
			if finallyScopeOriginalDefinition(ref, st) {
				matches++
			}
		case *statements.IfStatement:
			queue = append(queue, st.IfBody...)
			queue = append(queue, st.ElseBody...)
		case *statements.TryCatchStatement:
			queue = append(queue, st.TryBody...)
			for _, handler := range st.CatchBodies {
				queue = append(queue, handler...)
			}
		case *statements.DoWhileStatement:
			queue = append(queue, st.Body...)
		case *statements.SynchronizedStatement:
			queue = append(queue, st.Body...)
		case *statements.SwitchStatement:
			for _, arm := range st.Cases {
				if arm == nil {
					return false
				}
				queue = append(queue, arm.Body...)
			}
		}
	}
	return matches == 1
}
