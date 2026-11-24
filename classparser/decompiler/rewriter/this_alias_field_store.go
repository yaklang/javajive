package rewriter

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/jdecenv"
)

// RewriteThisAliasFieldStores rewrites field stores through a proved `this`
// alias local (`varK.field = expr` -> `this.field = expr`) inside constructors
// so javac definite assignment recognizes blank final field initialization.
// The alias declaration `Self varK = this;` and non-store uses remain intact.
// Kill-switch: JDEC_THIS_ALIAS_FIELD_STORE_OFF=1.
func RewriteThisAliasFieldStores(sts *[]statements.Statement, ctx *class_context.ClassContext) {
	if jdecenv.Get("JDEC_THIS_ALIAS_FIELD_STORE_OFF") == "1" {
		return
	}
	if sts == nil || len(*sts) == 0 || ctx == nil || ctx.FunctionName != "<init>" || ctx.ClassName == "" {
		return
	}
	targetClass := normalizeClassFQN(ctx.ClassName)

	defCount := map[*utils.VariableId]int{}
	singleRHS := map[*utils.VariableId]values.JavaValue{}
	singleRef := map[*utils.VariableId]*values.JavaRef{}

	recordLocalDef := func(ref *values.JavaRef, rhs values.JavaValue) {
		if ref == nil || ref.Id == nil || ref.IsParam || ref.IsThis {
			return
		}
		defCount[ref.Id]++
		if defCount[ref.Id] == 1 && rhs != nil && sameConstructorClassType(ref.Type(), targetClass) {
			singleRHS[ref.Id] = rhs
			singleRef[ref.Id] = ref
			return
		}
		delete(singleRHS, ref.Id)
		delete(singleRef, ref.Id)
	}

	seenValsPass1 := map[values.JavaValue]bool{}
	var scanValuePass1 func(values.JavaValue)
	scanValuePass1 = func(v values.JavaValue) {
		if v == nil || seenValsPass1[v] {
			return
		}
		seenValsPass1[v] = true
		switch x := v.(type) {
		case *values.AssignmentExpression:
			if x != nil {
				if ref, ok := core.UnpackSoltValue(x.Target).(*values.JavaRef); ok {
					recordLocalDef(ref, x.Value)
				}
			}
		case *values.JavaExpression:
			if x != nil && (x.Op == values.INC || x.Op == values.DEC) && len(x.Values) > 0 {
				if ref, ok := core.UnpackSoltValue(x.Values[0]).(*values.JavaRef); ok {
					recordLocalDef(ref, nil)
				}
			}
		}
		children, known := values.Children(v)
		if custom, ok := v.(*values.CustomValue); ok && custom != nil && custom.CapturesKnown {
			children, known = custom.Captures, true
		}
		if known {
			for _, c := range children {
				scanValuePass1(c)
			}
		}
	}

	var scanStmtsPass1 func([]statements.Statement)
	scanStmtsPass1 = func(list []statements.Statement) {
		for _, st := range list {
			if st == nil {
				continue
			}
			if as, _ := unwrapAssignStatement(st); as != nil {
				if as.ArrayMember == nil {
					if ref, ok := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok {
						rhs := as.JavaValue
						if as.IsDeclare && rhs == nil {
							rhs = nil
						}
						recordLocalDef(ref, rhs)
					}
				}
				scanValuePass1(as.LeftValue)
				scanValuePass1(as.ArrayMember)
				scanValuePass1(as.JavaValue)
			} else {
				for _, operand := range statementHeadValues(st) {
					scanValuePass1(operand)
				}
			}
			switch s := st.(type) {
			case *statements.ForStatement:
				scanStmtsPass1([]statements.Statement{s.InitVar, s.EndExp})
			case *statements.TryCatchStatement:
				for _, ex := range s.Exception {
					if ex != nil && ex.Id != nil {
						defCount[ex.Id]++
						delete(singleRHS, ex.Id)
						delete(singleRef, ex.Id)
					}
				}
			}
			for _, child := range childStatementLists(st) {
				scanStmtsPass1(*child)
			}
		}
	}
	scanStmtsPass1(*sts)

	thisAlias := map[*utils.VariableId]bool{}
	var canonicalThis *values.JavaRef
	changed := true
	for changed {
		changed = false
		for id, rhs := range singleRHS {
			if thisAlias[id] || defCount[id] != 1 {
				continue
			}
			srcRef, ok := core.UnpackSoltValue(rhs).(*values.JavaRef)
			if !ok || srcRef == nil {
				continue
			}
			if srcRef.IsThis {
				if canonicalThis == nil {
					canonicalThis = srcRef
				}
				thisAlias[id] = true
				changed = true
			} else if srcRef.Id != nil && thisAlias[srcRef.Id] {
				thisAlias[id] = true
				changed = true
			}
		}
	}

	makeThisRef := func(recv *values.JavaRef) *values.JavaRef {
		var typ types.JavaType
		if recv != nil && recv.Type() != nil {
			typ = recv.Type().Copy()
		} else if canonicalThis != nil && canonicalThis.Type() != nil {
			typ = canonicalThis.Type().Copy()
		} else {
			typ = types.NewJavaClass(targetClass)
		}
		var id *utils.VariableId
		if canonicalThis != nil && canonicalThis.Id != nil {
			id = canonicalThis.Id
		} else {
			id = utils.NewRootVariableId()
			id.SetName("this")
		}
		ref := values.NewJavaRef(id, nil, typ)
		ref.IsThis = true
		return ref
	}

	rewriteFieldTarget := func(lhs values.JavaValue) {
		rm, ok := core.UnpackSoltValue(lhs).(*values.RefMember)
		if !ok || rm == nil || rm.Object == nil {
			return
		}
		recv, ok := core.UnpackSoltValue(rm.Object).(*values.JavaRef)
		if !ok || recv == nil || recv.IsThis {
			return
		}
		if !sameConstructorClassType(recv.Type(), targetClass) {
			return
		}
		qualifiedAlias := false
		if recv.Id != nil && thisAlias[recv.Id] {
			qualifiedAlias = true
		} else if recv.IsThisAlias && (recv.Id == nil || defCount[recv.Id] <= 1) {
			qualifiedAlias = true
		}
		if !qualifiedAlias {
			return
		}
		rm.Object = makeThisRef(recv)
	}

	seenValsPass2 := map[values.JavaValue]bool{}
	var scanValuePass2 func(values.JavaValue)
	scanValuePass2 = func(v values.JavaValue) {
		if v == nil || seenValsPass2[v] {
			return
		}
		seenValsPass2[v] = true
		if ae, ok := v.(*values.AssignmentExpression); ok && ae != nil {
			rewriteFieldTarget(ae.Target)
		}
		children, known := values.Children(v)
		if custom, ok := v.(*values.CustomValue); ok && custom != nil && custom.CapturesKnown {
			children, known = custom.Captures, true
		}
		if known {
			for _, c := range children {
				scanValuePass2(c)
			}
		}
	}

	var rewriteStmts func([]statements.Statement)
	rewriteStmts = func(list []statements.Statement) {
		for _, st := range list {
			if st == nil {
				continue
			}
			if as, _ := unwrapAssignStatement(st); as != nil {
				if as.ArrayMember == nil {
					rewriteFieldTarget(as.LeftValue)
				}
				scanValuePass2(as.LeftValue)
				scanValuePass2(as.ArrayMember)
				scanValuePass2(as.JavaValue)
			} else {
				for _, operand := range statementHeadValues(st) {
					scanValuePass2(operand)
				}
			}
			if loop, ok := st.(*statements.ForStatement); ok && loop != nil {
				rewriteStmts([]statements.Statement{loop.InitVar, loop.EndExp})
			}
			for _, child := range childStatementLists(st) {
				rewriteStmts(*child)
			}
		}
	}
	rewriteStmts(*sts)
}

func statementHeadValues(st statements.Statement) []values.JavaValue {
	switch s := st.(type) {
	case *statements.ConditionStatement:
		if s != nil {
			return []values.JavaValue{s.Condition}
		}
	case *statements.IfStatement:
		if s != nil {
			return []values.JavaValue{s.Condition}
		}
	case *statements.WhileStatement:
		if s != nil {
			return []values.JavaValue{s.ConditionValue}
		}
	case *statements.DoWhileStatement:
		if s != nil {
			return []values.JavaValue{s.ConditionValue}
		}
	case *statements.SwitchStatement:
		if s != nil {
			return []values.JavaValue{s.Value}
		}
	case *statements.SynchronizedStatement:
		if s != nil {
			return []values.JavaValue{s.Argument}
		}
	case *statements.ForStatement:
		if s != nil && s.Condition != nil {
			return []values.JavaValue{s.Condition.Condition}
		}
	case *statements.ReturnStatement:
		if s != nil {
			return []values.JavaValue{s.JavaValue}
		}
	case *statements.ExpressionStatement:
		if s != nil {
			return []values.JavaValue{s.Expression}
		}
	case *statements.StackAssignStatement:
		if s != nil {
			return []values.JavaValue{s.JavaValue}
		}
	case *statements.CustomStatement:
		if s != nil && s.ThrownValue != nil {
			return []values.JavaValue{s.ThrownValue}
		}
	}
	if v, ok := st.(values.JavaValue); ok {
		return []values.JavaValue{v}
	}
	return nil
}

func normalizeClassFQN(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "/", ".")
}

func sameConstructorClassType(typ types.JavaType, normalizedClass string) bool {
	if typ == nil || typ.IsArray() || normalizedClass == "" {
		return false
	}
	raw, ok := types.RawClassFQN(typ)
	if !ok {
		return false
	}
	return normalizeClassFQN(raw) == normalizedClass
}
