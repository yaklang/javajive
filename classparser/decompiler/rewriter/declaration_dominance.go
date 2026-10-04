package rewriter

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/jdecenv"
)

// topLevelDeclDominatesAllUses checks lexical coverage by declaration identity.
// A later or sibling definition cannot cover an earlier use; a child's own
// declaration can cover that child without requiring an enclosing declaration.
// Typed dependencies must never be inferred from a rendering of the whole block:
// that loses hidden captures and repeatedly expands the same expression tree.
func topLevelDeclDominatesAllUses(list []statements.Statement, id *utils.VariableId) bool {
	return id == nil || !blockHasUncoveredRef(list, id, false)
}

func declaresIdentity(st statements.Statement, id *utils.VariableId) bool {
	as, ok := st.(*statements.AssignStatement)
	if !ok || as == nil || as.ArrayMember != nil || !(as.IsFirst || as.IsDeclare) {
		return false
	}
	ref, ok := values.UnpackSoltValue(as.LeftValue).(*values.JavaRef)
	return ok && ref != nil && ref.Id == id
}

func blockHasUncoveredRef(list []statements.Statement, id *utils.VariableId, declared bool) bool {
	for _, st := range list {
		if st == nil {
			continue
		}
		if declaresIdentity(st, id) {
			// The definition is not a use, but its initializer is evaluated before
			// the new local is available (int x = x is not a dominating definition).
			if !declared && valueReferencesIdentity(st.(*statements.AssignStatement).JavaValue, id) {
				return true
			}
			declared = true
			continue
		}
		if loop, ok := st.(*statements.ForStatement); ok {
			if blockHasUncoveredRef([]statements.Statement{loop.InitVar}, id, declared) {
				return true
			}
			bound := declared || declaresIdentity(loop.InitVar, id)
			if !bound && (statementHeadReferencesIdentity(loop.Condition, id) || statementHeadReferencesIdentity(loop.EndExp, id)) {
				return true
			}
			if blockHasUncoveredRef(loop.SubStatements, id, bound) {
				return true
			}
			// An initializer's declaration belongs only to this loop.
			continue
		}
		if !declared && statementHeadReferencesIdentity(st, id) {
			return true
		}
		for i, child := range childStatementLists(st) {
			bound := declared
			if handler, ok := st.(*statements.TryCatchStatement); ok && i > 0 && i-1 < len(handler.Exception) {
				if ex := handler.Exception[i-1]; ex != nil && ex.Id == id {
					bound = true
				}
			}
			if blockHasUncoveredRef(*child, id, bound) {
				return true
			}
		}
	}
	return false
}

// Only the expressions evaluated by this statement are examined here. Child
// blocks are visited with their own lexical bindings, not rendered as head uses.
func statementHeadReferencesIdentity(st statements.Statement, id *utils.VariableId) bool {
	var operands []values.JavaValue
	switch s := st.(type) {
	case nil:
		return false
	case *statements.AssignStatement:
		operands = []values.JavaValue{s.LeftValue, s.ArrayMember, s.JavaValue}
	case *statements.ConditionStatement:
		if s != nil {
			operands = []values.JavaValue{s.Condition}
		}
	case *statements.IfStatement:
		operands = []values.JavaValue{s.Condition}
	case *statements.WhileStatement:
		operands = []values.JavaValue{s.ConditionValue}
	case *statements.DoWhileStatement:
		operands = []values.JavaValue{s.ConditionValue}
	case *statements.SwitchStatement:
		operands = []values.JavaValue{s.Value}
	case *statements.SynchronizedStatement:
		operands = []values.JavaValue{s.Argument}
	case *statements.ForStatement:
		// The loop's initializer has its own lexical binding, checked by
		// blockHasUncoveredRef. Enumerate heads here without rendering a
		// whole loop or mistaking dependencies of its body for head uses.
		return statementHeadReferencesIdentity(s.InitVar, id) ||
			statementHeadReferencesIdentity(s.Condition, id) ||
			statementHeadReferencesIdentity(s.EndExp, id)
	case *statements.ReturnStatement:
		operands = []values.JavaValue{s.JavaValue}
	case *statements.ExpressionStatement:
		operands = []values.JavaValue{s.Expression}
	case *statements.StackAssignStatement:
		operands = []values.JavaValue{s.JavaValue}
	case *statements.TryCatchStatement, *statements.GOTOStatement, *statements.NewStatement, *statements.MiddleStatement:
		return false
	case *statements.CustomStatement:
		if s.ThrownValue == nil {
			return opaqueReferencesIdentity(s.String, id)
		}
		operands = []values.JavaValue{s.ThrownValue}
	default:
		if v, ok := st.(values.JavaValue); ok {
			operands = []values.JavaValue{v}
		} else {
			return opaqueReferencesIdentity(st.String, id)
		}
	}
	for _, operand := range operands {
		if valueReferencesIdentity(operand, id) {
			return true
		}
	}
	return false
}

func valueReferencesIdentity(value values.JavaValue, id *utils.VariableId) bool {
	seen := map[values.JavaValue]bool{}
	var visit func(values.JavaValue) bool
	visit = func(v values.JavaValue) bool {
		if v == nil || seen[v] {
			return false
		}
		seen[v] = true
		if ref, ok := v.(*values.JavaRef); ok && ref != nil && ref.Id == id {
			return true
		}
		children, known := values.Children(v)
		// Explicit dependencies are usable for identity analysis even when the
		// expression's effects remain opaque. This does not grant motion/purity.
		if custom, ok := v.(*values.CustomValue); ok && custom != nil && custom.CapturesKnown {
			children, known = custom.Captures, true
		}
		for _, child := range children {
			if visit(child) {
				return true
			}
		}
		return !known && opaqueReferencesIdentity(v.String, id)
	}
	return visit(value)
}

// Legacy closures still hide some dependencies. Probe only that leaf, with a
// name absent from its original rendering; restore the identity even on panic.
// Literal/comment text is not a Java variable use. Typed nodes never take this
// path, and a panic keeps the old conservative uncovered-use behavior.
func opaqueReferencesIdentity(render func(*class_context.ClassContext) string, id *utils.VariableId) (uses bool) {
	saved := id.Name
	defer func() {
		id.SetName(saved)
		if recover() != nil {
			uses = true
		}
	}()
	ctx := &class_context.ClassContext{Env: jdecenv.Lookup()}
	original := render(ctx)
	probe := "__jdec_dom_probe__"
	for strings.Contains(original, probe) {
		probe += "_"
	}
	id.SetName(probe)
	return codeContainsIdentifier(render(ctx), probe)
}

func codeContainsIdentifier(code, name string) bool {
	word := func(b byte) bool { return isWordByteASCII(b) || b == '$' || b >= 0x80 }
	for i := 0; i < len(code); {
		switch {
		case strings.HasPrefix(code[i:], "//"):
			if end := strings.IndexByte(code[i:], '\n'); end >= 0 {
				i += end + 1
			} else {
				return false
			}
		case strings.HasPrefix(code[i:], "/*"):
			if end := strings.Index(code[i+2:], "*/"); end >= 0 {
				i += end + 4
			} else {
				return false
			}
		case code[i] == '"' || code[i] == '\'':
			quote, delimiter := code[i], 1
			if strings.HasPrefix(code[i:], `"""`) {
				delimiter = 3
			}
			i += delimiter
			for i < len(code) {
				if code[i] == '\\' {
					i += 2
				} else if delimiter == 3 && strings.HasPrefix(code[i:], `"""`) {
					i += 3
					break
				} else if delimiter == 1 && code[i] == quote {
					i++
					break
				} else {
					i++
				}
			}
		default:
			if strings.HasPrefix(code[i:], name) && (i == 0 || !word(code[i-1])) && (i+len(name) == len(code) || !word(code[i+len(name)])) {
				return true
			}
			i++
		}
	}
	return false
}
