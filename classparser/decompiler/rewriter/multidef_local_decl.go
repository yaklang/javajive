package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/jdecenv"
)

// declareUncoveredMultiDefGeneratedLocals recovers missing declarations for
// generated locals (varN / varN_K) that RewriteVar left undeclared after
// coverUndeclaredGeneratedLocals (for example, a generated local defined in
// both arms of an `if/else` or inside a loop and read after, or assigned
// twice sequentially in the same block with IsFirst=false).
//
// Unlike restoreExplicitLocalDeclarations, this runs only at the end of
// RewriteVar and never changes coverUndeclaredGeneratedLocals's direct contract.
// Kill-switch: JDEC_MULTIDEF_LOCAL_DECL_OFF=1.
func declareUncoveredMultiDefGeneratedLocals(sts *[]statements.Statement) {
	if sts == nil || len(*sts) == 0 || jdecenv.Get("JDEC_MULTIDEF_LOCAL_DECL_OFF") == "1" {
		return
	}
	syncOff := jdecenv.Get("JDEC_SYNC_REWRITE_OFF") == "1"
	declared := map[*utils.VariableId]bool{}
	defs := map[*utils.VariableId][]*statements.AssignStatement{}
	var order []*utils.VariableId

	var visit func([]statements.Statement)
	visit = func(list []statements.Statement) {
		for _, st := range list {
			if st == nil {
				continue
			}
			if as, _ := unwrapAssignStatement(st); as != nil && as.ArrayMember == nil {
				if ref, ok := values.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok && ref != nil && ref.Id != nil {
					if as.IsFirst || as.IsDeclare || ref.IsParam || ref.IsThis {
						declared[ref.Id] = true
					}
					if !ref.IsParam && !ref.IsThis && ref.CustomValue == nil && ref.StackVar == nil && as.JavaValue != nil && ref.Type() != nil {
						if len(defs[ref.Id]) == 0 {
							order = append(order, ref.Id)
						}
						defs[ref.Id] = append(defs[ref.Id], as)
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
			for _, child := range visibleChildStatementLists(st, syncOff) {
				visit(*child)
			}
		}
	}
	visit(*sts)

	if len(order) == 0 {
		return
	}
	ctx := &class_context.ClassContext{}
	for _, id := range order {
		stores := defs[id]
		if declared[id] || len(stores) == 0 {
			continue
		}
		ref0, _ := values.UnpackSoltValue(stores[0].LeftValue).(*values.JavaRef)
		if ref0 == nil || ref0.Type() == nil {
			continue
		}
		name := ref0.String(ctx)
		if !generatedLocalNameRe.MatchString(name) {
			continue
		}
		typStr := ref0.Type().String(ctx)
		if typStr == "" || defaultInitializerForType(ref0.Type()) == nil {
			continue
		}
		consistent := true
		for _, as := range stores[1:] {
			r, _ := values.UnpackSoltValue(as.LeftValue).(*values.JavaRef)
			if r == nil || r.Type() == nil || r.Type().String(ctx) != typStr {
				consistent = false
				break
			}
		}
		if !consistent {
			continue
		}
		// Inline same-block fast path for multi-definition sequences where the
		// first store dominates every use of this identity.
		if len(stores) >= 2 && !stores[0].IsFirst {
			stores[0].IsFirst = true
			if topLevelDeclDominatesAllUses(*sts, id) {
				declared[id] = true
				continue
			}
			stores[0].IsFirst = false
		}
		if insertDefaultDeclAtLCA(sts, id, ref0, syncOff) {
			declared[id] = true
		}
	}
}

func visibleChildStatementLists(st statements.Statement, syncOff bool) []*[]statements.Statement {
	if syncOff {
		if _, ok := st.(*statements.SynchronizedStatement); ok {
			return nil
		}
	}
	return childStatementLists(st)
}

func stmtContainsIdentity(st statements.Statement, id *utils.VariableId, syncOff bool) bool {
	if st == nil || id == nil {
		return false
	}
	if statementHeadReferencesIdentity(st, id) {
		return true
	}
	for _, cl := range visibleChildStatementLists(st, syncOff) {
		for _, child := range *cl {
			if stmtContainsIdentity(child, id, syncOff) {
				return true
			}
		}
	}
	return false
}

func blockContainsIdentity(list []statements.Statement, id *utils.VariableId, syncOff bool) bool {
	for _, st := range list {
		if stmtContainsIdentity(st, id, syncOff) {
			return true
		}
	}
	return false
}

func insertDefaultDeclAtLCA(block *[]statements.Statement, id *utils.VariableId, sample *values.JavaRef, syncOff bool) bool {
	if block == nil || len(*block) == 0 || id == nil || sample == nil || sample.Type() == nil {
		return false
	}
	list := *block
	firstIdx := -1
	refCount := 0
	for i, st := range list {
		if stmtContainsIdentity(st, id, syncOff) {
			if firstIdx < 0 {
				firstIdx = i
			}
			refCount++
		}
	}
	if firstIdx < 0 {
		return false
	}
	if refCount == 1 && !statementHeadReferencesIdentity(list[firstIdx], id) {
		var matchedChild *[]statements.Statement
		childHits := 0
		for _, cl := range visibleChildStatementLists(list[firstIdx], syncOff) {
			if blockContainsIdentity(*cl, id, syncOff) {
				matchedChild = cl
				childHits++
			}
		}
		if childHits == 1 && matchedChild != nil {
			if insertDefaultDeclAtLCA(matchedChild, id, sample, syncOff) {
				return true
			}
		}
	}
	init := defaultInitializerForType(sample.Type())
	if init == nil {
		return false
	}
	decl := statements.NewAssignStatement(values.NewJavaRef(id, nil, sample.Type().Copy()), init, true)
	updated := make([]statements.Statement, 0, len(list)+1)
	updated = append(updated, list[:firstIdx]...)
	updated = append(updated, decl)
	updated = append(updated, list[firstIdx:]...)
	*block = updated
	return true
}
