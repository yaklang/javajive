package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Recover a lost declaration flag only for an explicit single local definition
// whose original position lexically covers every use of that exact identity.
// A printed slot name, read-only orphan or multi-definition web is insufficient.
// Restoring the inline declaration preserves its RHS and evaluation position;
// it neither hoists a store nor invents a default initializer for a missing path.
func restoreExplicitLocalDeclarations(root *[]statements.Statement) {
	if root == nil {
		return
	}
	declared := map[*utils.VariableId]bool{}
	definitions := map[*utils.VariableId][]*statements.AssignStatement{}
	var order []*utils.VariableId
	var visit func([]statements.Statement)
	visit = func(list []statements.Statement) {
		for _, st := range list {
			if as, ok := st.(*statements.AssignStatement); ok && as != nil && as.ArrayMember == nil {
				if ref, ok := values.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok && ref != nil && ref.Id != nil {
					if as.IsFirst || as.IsDeclare || ref.IsParam || ref.IsThis {
						declared[ref.Id] = true
					}
					if as.JavaValue != nil && ref.Type() != nil {
						if len(definitions[ref.Id]) == 0 {
							order = append(order, ref.Id)
						}
						definitions[ref.Id] = append(definitions[ref.Id], as)
					}
				}
			}
			switch s := st.(type) {
			case *statements.TryCatchStatement:
				for _, ex := range s.Exception {
					if ex != nil && ex.Id != nil {
						declared[ex.Id] = true
					}
				}
			case *statements.ForStatement:
				visit([]statements.Statement{s.InitVar, s.EndExp})
			}
			for _, child := range childStatementLists(st) {
				visit(*child)
			}
		}
	}
	visit(*root)
	for _, id := range order {
		if declared[id] || len(definitions[id]) != 1 {
			continue
		}
		store := definitions[id][0]
		store.IsFirst = true
		if !topLevelDeclDominatesAllUses(*root, id) {
			store.IsFirst = false
		}
	}
}
