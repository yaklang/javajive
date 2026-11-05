package rewriter

import (
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// hoistStackMergePhiDeclarations gives every synthetic acyclic stack-join local
// one root declaration before RewriteVar starts naming variables. Without it,
// the first textual assignment can occur inside a nested arm (for example a
// switch expression's guarded case) and the renamer binds that identity to a
// block-local name, while sibling arms and the shared consumer use another
// name. The join would then read a different variable. A root declaration with
// a type default preserves the JVM proof that every incoming edge assigns the
// value before the consumer.
func hoistStackMergePhiDeclarations(sts *[]statements.Statement) {
	if sts == nil || len(*sts) == 0 {
		return
	}
	declared := map[*utils.VariableId]bool{}
	for _, st := range *sts {
		as, ok := st.(*statements.AssignStatement)
		if !ok || as.ArrayMember != nil || !as.IsFirst || as.JavaValue == nil {
			continue
		}
		if ref, ok := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok && ref != nil && ref.Id != nil {
			declared[ref.Id] = true
		}
	}

	refs := map[*utils.VariableId]*values.JavaRef{}
	var walk func([]statements.Statement)
	walk = func(list []statements.Statement) {
		for _, st := range list {
			if as, ok := st.(*statements.AssignStatement); ok && as.ArrayMember == nil {
				if ref, ok := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok && ref != nil && ref.Id != nil &&
					ref.IsStackMergePhi() && !ref.IsParam && !ref.IsThis && ref.Type() != nil {
					refs[ref.Id] = ref
				}
			}
			for _, child := range childStatementLists(st) {
				walk(*child)
			}
		}
	}
	walk(*sts)
	if len(refs) == 0 {
		return
	}

	ordered := make([]*values.JavaRef, 0, len(refs))
	for _, ref := range refs {
		ordered = append(ordered, ref)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].VarUid < ordered[j].VarUid
	})
	prefix := make([]statements.Statement, 0, len(ordered))
	for _, ref := range ordered {
		if declared[ref.Id] {
			continue
		}
		initializer := defaultInitializerForType(ref.Type())
		if initializer == nil {
			continue
		}
		prefix = append(prefix, statements.NewAssignStatement(ref, initializer, true))
		declared[ref.Id] = true
	}
	if len(prefix) == 0 {
		return
	}
	*sts = append(prefix, *sts...)
}
