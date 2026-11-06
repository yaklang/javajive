package rewriter

import (
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// widenInstanceofStructured is the default widening path. It discovers
// instanceof operands and all uses of their variables from the statement tree,
// then widens only when every use is legal under Object or Number. The legacy
// rendered-text scan remains available through JDEC_LEGACY_TEXT_WIDEN=1 for
// A/B measurement and rollback while this pass is validated.
func widenInstanceofStructured(sts *[]statements.Statement, funcCtx *class_context.ClassContext) {
	candidates := collectInstanceOfRefs(*sts)
	if len(candidates) == 0 {
		return
	}
	uids := make([]string, 0, len(candidates))
	for uid := range candidates {
		uids = append(uids, uid)
	}
	sort.Strings(uids)
	objectType := types.NewJavaClass("java.lang.Object")
	numberType := types.NewJavaClass("java.lang.Number")
	for _, uid := range uids {
		ref := candidates[uid]
		if ref == nil || ref.Type() == nil || ref.IsThis || ref.IsParam || ref.WebDeclType != nil {
			continue
		}
		if _, primitive := ref.Type().RawType().(*types.JavaPrimer); primitive {
			continue
		}
		fqn, known := types.ClassFQNOf(ref.Type())
		if !known || fqn == "java.lang.Object" {
			continue
		}
		safety := analyzeWidenSafety(*sts, ref, funcCtx)
		switch {
		case safety.object:
			widenRefByVarUid(*sts, uid, objectType)
		case safety.number:
			widenRefByVarUid(*sts, uid, numberType)
		}
	}
}

// widenNumericMixedStructured is the default path for a concrete boxed numeric
// declaration that is reassigned an incompatible boxed numeric subtype.
func widenNumericMixedStructured(sts *[]statements.Statement, funcCtx *class_context.ClassContext) {
	decls := concreteNumericDeclarations(*sts)
	if len(decls) == 0 {
		return
	}
	mixed := mixedNumericDeclarations(*sts, decls)
	if len(mixed) == 0 {
		return
	}
	uids := make([]string, 0, len(mixed))
	for uid := range mixed {
		uids = append(uids, uid)
	}
	sort.Strings(uids)
	numberType := types.NewJavaClass("java.lang.Number")
	for _, uid := range uids {
		ref := decls[uid]
		if ref == nil {
			continue
		}
		if safety := analyzeWidenSafety(*sts, ref, funcCtx); safety.number {
			widenRefByVarUid(*sts, uid, numberType)
		}
	}
}

func concreteNumericDeclarations(sts []statements.Statement) map[string]*values.JavaRef {
	out := map[string]*values.JavaRef{}
	walkAssignments(sts, func(as *statements.AssignStatement) {
		if as == nil || (!as.IsFirst && !as.IsDeclare) || as.ArrayMember != nil {
			return
		}
		ref := unwrapJavaRef(as.LeftValue)
		if ref == nil || ref.VarUid == "" || ref.Type() == nil || !isConcreteNumericType(ref.Type()) {
			return
		}
		if _, exists := out[ref.VarUid]; !exists {
			out[ref.VarUid] = ref
		}
	})
	return out
}

func mixedNumericDeclarations(sts []statements.Statement, decls map[string]*values.JavaRef) map[string]struct{} {
	out := map[string]struct{}{}
	walkAssignments(sts, func(as *statements.AssignStatement) {
		if as == nil || as.IsFirst || as.IsDeclare || as.ArrayMember != nil || as.JavaValue == nil {
			return
		}
		ref := unwrapJavaRef(as.LeftValue)
		if ref == nil {
			return
		}
		decl := decls[ref.VarUid]
		if decl == nil || decl.Type() == nil || !isConcreteNumericType(decl.Type()) {
			return
		}
		rhs := as.JavaValue.Type()
		if rhs == nil || !isConcreteNumericType(rhs) || typeFQNEquals(decl.Type(), rhs) {
			return
		}
		out[ref.VarUid] = struct{}{}
	})
	return out
}

func walkAssignments(sts []statements.Statement, fn func(*statements.AssignStatement)) {
	var walkList func([]statements.Statement)
	var walkStatement func(statements.Statement)
	walkStatement = func(st statements.Statement) {
		if st == nil {
			return
		}
		if as, ok := st.(*statements.AssignStatement); ok {
			fn(as)
		}
		if loop, ok := st.(*statements.ForStatement); ok {
			if loop.InitVar != nil {
				walkStatement(loop.InitVar)
			}
			if loop.EndExp != nil {
				walkStatement(loop.EndExp)
			}
		}
		for _, child := range childStatementLists(st) {
			walkList(*child)
		}
	}
	walkList = func(list []statements.Statement) {
		for _, st := range list {
			walkStatement(st)
		}
	}
	walkList(sts)
}

func unwrapJavaRef(v values.JavaValue) *values.JavaRef {
	for {
		switch t := v.(type) {
		case *values.SlotValue:
			if t == nil {
				return nil
			}
			v = t.GetValue()
		case *values.JavaRef:
			return t
		default:
			return nil
		}
	}
}

type instanceOfCollector struct {
	refs  map[string]*values.JavaRef
	steps int
}

func collectInstanceOfRefs(sts []statements.Statement) map[string]*values.JavaRef {
	c := &instanceOfCollector{refs: map[string]*values.JavaRef{}}
	c.walkStatements(sts)
	return c.refs
}

func (c *instanceOfCollector) walkStatements(sts []statements.Statement) {
	for _, st := range sts {
		c.walkStatement(st)
	}
}

func (c *instanceOfCollector) walkStatement(st statements.Statement) {
	if st == nil || c.budget() {
		return
	}
	walkStatementValues(st, c.walkValue)
}

func (c *instanceOfCollector) budget() bool {
	c.steps++
	return c.steps > 16384
}

func (c *instanceOfCollector) walkValue(v values.JavaValue) {
	if v == nil || c.budget() {
		return
	}
	for {
		slot, ok := v.(*values.SlotValue)
		if !ok || slot == nil {
			break
		}
		v = slot.GetValue()
		if v == nil {
			return
		}
	}
	if ref, ok := v.(*values.JavaRef); ok {
		if ref == nil {
			return
		}
		if ref.CustomValue != nil {
			c.walkValue(ref.CustomValue)
		}
		return
	}
	if custom, ok := v.(*values.CustomValue); ok && custom != nil {
		if operand, _, _, known := custom.OriginalInstanceOfSource(); known {
			if ref := unwrapJavaRef(operand); ref != nil && ref.VarUid != "" {
				c.refs[ref.VarUid] = ref
			}
			c.walkValue(operand)
			return
		}
	}
	children, known := values.Children(v)
	if !known {
		return
	}
	for _, child := range children {
		c.walkValue(child)
	}
}

func walkStatementValues(st statements.Statement, fn func(values.JavaValue)) {
	switch s := st.(type) {
	case *statements.AssignStatement:
		if s.ArrayMember != nil {
			fn(s.ArrayMember.Object)
			fn(s.ArrayMember.Index)
		}
		fn(s.LeftValue)
		fn(s.JavaValue)
	case *statements.ConditionStatement:
		fn(s.Condition)
	case *statements.IfStatement:
		fn(s.Condition)
		walkStatementListValues(s.IfBody, fn)
		walkStatementListValues(s.ElseBody, fn)
	case *statements.ForStatement:
		walkStatementValues(s.InitVar, fn)
		if s.Condition != nil {
			fn(s.Condition.Condition)
		}
		walkStatementValues(s.EndExp, fn)
		walkStatementListValues(s.SubStatements, fn)
	case *statements.WhileStatement:
		fn(s.ConditionValue)
		walkStatementListValues(s.Body, fn)
	case *statements.DoWhileStatement:
		fn(s.ConditionValue)
		walkStatementListValues(s.Body, fn)
	case *statements.SwitchStatement:
		fn(s.Value)
		for _, item := range s.Cases {
			if item != nil {
				walkStatementListValues(item.Body, fn)
			}
		}
	case *statements.TryCatchStatement:
		walkStatementListValues(s.TryBody, fn)
		for _, body := range s.CatchBodies {
			walkStatementListValues(body, fn)
		}
	case *statements.SynchronizedStatement:
		fn(s.Argument)
		walkStatementListValues(s.Body, fn)
	case *statements.ReturnStatement:
		fn(s.JavaValue)
	case *statements.ExpressionStatement:
		fn(s.Expression)
	case *statements.StackAssignStatement:
		fn(s.JavaValue)
	case *statements.CustomStatement:
		if as, ok := s.SourceAssignmentForward(); ok && as != nil {
			walkStatementValues(as, fn)
			return
		}
		fn(s.ThrownValue)
	}
}

func walkStatementListValues(sts []statements.Statement, fn func(values.JavaValue)) {
	for _, st := range sts {
		walkStatementValues(st, fn)
	}
}
