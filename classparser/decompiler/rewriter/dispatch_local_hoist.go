package rewriter

import (
	"sort"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/jdecenv"
)

// hoistDispatchCrossCaseDeclarations gives irreducible-dispatch locals a method
// root declaration with a type-default initializer. placeCrossScopeDeclarations
// widens a local used in sibling switch cases to the do-body, where it renders
// as a bare `int varN;`. javac's definite-assignment analysis rejects a read in
// a sibling case that may run before the declaring case, even though the JVM
// verifier proves a store dominates every original read. Declaring once at the
// method root with a default value is semantically safe (the JVM proof still
// holds) and does not reset a loop-carried value on every dispatch iteration.
//
// Kill-switch: JDEC_DISPATCH_LOCAL_HOIST_OFF=1.
func hoistDispatchCrossCaseDeclarations(sts *[]statements.Statement) {
	if sts == nil || jdecenv.Get("JDEC_DISPATCH_LOCAL_HOIST_OFF") == "1" {
		return
	}
	var loops []*statements.DoWhileStatement
	var walk func([]statements.Statement)
	walk = func(list []statements.Statement) {
		for _, st := range list {
			if loop, ok := st.(*statements.DoWhileStatement); ok && dispatchSwitchOf(loop) != nil {
				loops = append(loops, loop)
			}
			for _, child := range childStatementLists(st) {
				walk(*child)
			}
		}
	}
	walk(*sts)
	if len(loops) == 0 {
		return
	}

	candidates := map[*utils.VariableId]*values.JavaRef{}
	var collect func([]statements.Statement)
	collect = func(list []statements.Statement) {
		for _, st := range list {
			if as, ok := st.(*statements.AssignStatement); ok && as.ArrayMember == nil {
				ref, _ := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef)
				if ref != nil && ref.Id != nil && !ref.IsParam && !ref.IsThis && ref.Type() != nil &&
					defaultInitializerForType(ref.Type()) != nil && !isSyntheticDispatchRef(ref) &&
					((as.IsDeclare && as.JavaValue == nil) || as.IsFirst) {
					if !hasInitializedRootDeclaration(*sts, ref.Id) && dispatchLoopsReference(loops, ref.Id) {
						candidates[ref.Id] = ref
					}
				}
			}
			for _, child := range childStatementLists(st) {
				collect(*child)
			}
		}
	}
	collect(*sts)
	if len(candidates) == 0 {
		return
	}

	refs := make([]*values.JavaRef, 0, len(candidates))
	for _, ref := range candidates {
		refs = append(refs, ref)
	}
	sort.SliceStable(refs, func(i, j int) bool {
		ni, nj := dispatchLocalName(refs[i]), dispatchLocalName(refs[j])
		if ni != nj {
			return ni < nj
		}
		return refs[i].VarUid < refs[j].VarUid
	})

	for _, ref := range refs {
		relocateDeclarations(sts, ref.Id)
		clearFinalForIdentity(*sts, ref.Id)
	}
	declared := make([]statements.Statement, 0, len(refs))
	for _, ref := range refs {
		initializer := defaultInitializerForType(ref.Type())
		if initializer == nil {
			continue
		}
		declared = append(declared, statements.NewAssignStatement(ref, initializer, true))
	}
	if len(declared) == 0 {
		return
	}
	*sts = append(declared, *sts...)
}

func dispatchLocalName(ref *values.JavaRef) string {
	if ref == nil {
		return ""
	}
	if name := ref.String(&class_context.ClassContext{}); name != "" {
		return name
	}
	if ref.Id != nil {
		return ref.Id.Name
	}
	return ""
}

func isSyntheticDispatchRef(ref *values.JavaRef) bool {
	if ref == nil || ref.Id == nil {
		return true
	}
	name := ref.Id.Name
	return strings.HasPrefix(name, "jdecState") ||
		strings.HasPrefix(name, "jdecExit") ||
		strings.HasPrefix(name, "jdecSkip")
}

func dispatchSwitchOf(loop *statements.DoWhileStatement) *statements.SwitchStatement {
	if loop == nil || !loop.IrreducibleDispatch {
		return nil
	}
	for _, st := range loop.Body {
		if sw, ok := st.(*statements.SwitchStatement); ok {
			return sw
		}
	}
	return nil
}

// dispatchLoopsReference reports whether any case body of a dispatcher switch
// references id without a declaration covering that reference. A local
// declared in one case and read in a sibling case therefore qualifies, while a
// local declared and used only inside one case does not.
func dispatchLoopsReference(loops []*statements.DoWhileStatement, id *utils.VariableId) bool {
	for _, loop := range loops {
		sw := dispatchSwitchOf(loop)
		if sw == nil {
			continue
		}
		for _, c := range sw.Cases {
			if c != nil && blockHasUncoveredRef(c.Body, id, false) {
				return true
			}
		}
	}
	return false
}

func hasInitializedRootDeclaration(list []statements.Statement, id *utils.VariableId) bool {
	for _, st := range list {
		as, ok := st.(*statements.AssignStatement)
		if !ok || as.ArrayMember != nil || !as.IsFirst || as.IsDeclare || as.JavaValue == nil {
			continue
		}
		ref, _ := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef)
		if ref != nil && ref.Id == id {
			return true
		}
	}
	return false
}

func clearFinalForIdentity(list []statements.Statement, id *utils.VariableId) {
	for _, st := range list {
		if as, ok := st.(*statements.AssignStatement); ok {
			if ref, _ := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ref != nil && ref.Id == id {
				as.Final = false
			}
		}
		for _, child := range childStatementLists(st) {
			clearFinalForIdentity(*child, id)
		}
	}
}

// defaultInitializerForType builds the Java zero value for a local's final
// type. Reference, array, String, generic, and type-variable defaults use an
// Object-typed null literal so JavaLiteral.String renders `null` rather than
// the quoted string "null"; the declaration type still comes from the ref.
func defaultInitializerForType(t types.JavaType) values.JavaValue {
	if t == nil {
		return nil
	}
	if t.IsArray() {
		return values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
	}
	if primer, ok := t.RawType().(*types.JavaPrimer); ok {
		switch primer.Name {
		case types.JavaBoolean:
			return values.NewJavaLiteral(false, t.Copy())
		case types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger:
			return values.NewJavaLiteral(0, t.Copy())
		case types.JavaLong:
			return values.NewJavaLiteral(0, t.Copy())
		case types.JavaFloat:
			return values.NewJavaLiteral(float32(0), t.Copy())
		case types.JavaDouble:
			return values.NewJavaLiteral(float64(0), t.Copy())
		case types.JavaVoid:
			return nil
		}
	}
	return values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
}
