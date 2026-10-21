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
	clone.TryBody = slices.Clone(tr.TryBody)
	var declarations []statements.Statement
	seen := map[*utils.VariableId]bool{}
	for i, statement := range tr.TryBody {
		store, initializing := statement.(*statements.AssignStatement)
		if !initializing || store == nil || !store.IsFirst || store.IsDeclare || store.ArrayMember != nil || store.JavaValue == nil || !store.HasOriginPC {
			continue
		}
		ref, local := plainTryValue(store.LeftValue).(*values.JavaRef)
		if !local || ref == nil || ref.Id == nil || ref.IsParam || ref.IsThis || ref.WebDeclType == nil || ref.Type() == nil || !reflect.DeepEqual(ref.WebDeclType.RawType(), ref.Type().RawType()) {
			continue
		}
		binding := map[*utils.VariableId]int{ref.Id: 1}
		if finallyCleanupBindingsScoped(tail, nil, binding) {
			continue
		}
		if seen[ref.Id] || len(declarations) >= 32 || !finallyCleanupBindingsScoped([]statements.Statement{statements.NewExpressionStatement(store.JavaValue)}, nil, binding) {
			return nil, nil, false
		}
		seen[ref.Id] = true
		assignment := *store
		assignment.IsFirst, assignment.IsDeclare, assignment.Final = false, false, false
		clone.TryBody[i] = &assignment
		declarations = append(declarations, &statements.AssignStatement{LeftValue: ref, IsDeclare: true, Final: store.Final})
	}
	if len(declarations) == 0 || !finallyContinuationKeepsLexicalBindings([]statements.Statement{&clone}, tail) {
		return nil, nil, false
	}
	return &clone, declarations, true
}
