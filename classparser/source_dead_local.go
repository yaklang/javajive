package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"reflect"
)

// Structuring consumes compiler monitor holders, leaving effect-free stores
// whose identities have no source reads. Prove that fact over the COMPLETE
// method before discarding a store. Unknown operands, hidden text, cycles or a
// bounded traversal failure preserve the original tree. Do not follow a plain
// local's initializer as a read: a load reads its identity, not its old producer.
func sourceWithoutDeadLocalStores(body []statements.Statement, params []values.JavaValue, work *workbudget.Budget) []statements.Statement {
	reads := map[*utils.VariableId]bool{}
	stores := map[*statements.AssignStatement]*utils.VariableId{}
	activeValues := map[values.JavaValue]bool{}
	activeStatements := map[statements.Statement]bool{}
	remaining := 16384
	valid := true
	// These maps retain identities or the active recursion path, not every
	// visited node. Sequential siblings reuse traversal storage. Work remains
	// cumulative, but an allocation preflight must describe a live container.
	storage := func(entries int) bool {
		// Include a map's initial bucket as well as its pointer-key entries.
		return work == nil || work.CheckAlloc(int64(max(entries, 2))*128) == nil
	}
	valueDepth := 0
	var value func(values.JavaValue)
	value = func(v values.JavaValue) {
		if !valid || sourceProofNil(v) {
			return
		}
		valueDepth++
		defer func() { valueDepth-- }()
		if valueDepth > 128 {
			valid = false
			return
		}
		remaining--
		if remaining < 0 || !storage(len(activeValues)+1) || !nativeProofWork(work, 1) || activeValues[v] {
			valid = false
			return
		}
		if work != nil {
			if work.Enter(workbudget.CounterASTDepth) != nil {
				valid = false
				return
			}
			defer work.Leave(workbudget.CounterASTDepth)
		}
		activeValues[v] = true
		defer delete(activeValues, v)
		if ref, ok := v.(*values.JavaRef); ok && ref.Id != nil && ref.CustomValue == nil && ref.StackVar == nil {
			if !reads[ref.Id] && !storage(len(reads)+1) {
				valid = false
				return
			}
			reads[ref.Id] = true
			return
		}
		children, known := values.Children(v)
		if !known {
			valid = false
			return
		}
		for _, child := range children {
			value(child)
		}
	}
	for _, v := range params {
		value(v)
	}
	pure := func(target *values.JavaRef, v values.JavaValue) bool {
		if target.Type() == nil {
			return false
		}
		var known bool
		v, known = sourceBoundedLocalOperand(v)
		if !known {
			return false
		}
		if v == values.JavaNull || values.IsNullLiteral(v) {
			_, class := target.Type().RawType().(*types.JavaClass)
			return class || target.Type().IsArray()
		}
		switch x := v.(type) {
		case *values.JavaRef:
			return x != nil && x.Id != nil && x.CustomValue == nil && x.StackVar == nil && x.Type() != nil && reflect.DeepEqual(target.Type().RawType(), x.Type().RawType())
		case *values.JavaLiteral:
			return x != nil && x.Type() != nil && reflect.DeepEqual(target.Type().RawType(), x.Type().RawType())
		}

		return false
	}
	statementDepth := 0
	var walk func([]statements.Statement)
	walk = func(list []statements.Statement) {
		if !valid {
			return
		}
		statementDepth++
		defer func() { statementDepth-- }()
		if statementDepth > 128 {
			valid = false
			return
		}
		if work != nil {
			if work.Enter(workbudget.CounterASTDepth) != nil {
				valid = false
				return
			}
			defer work.Leave(workbudget.CounterASTDepth)
		}
		for _, st := range list {
			remaining--
			if remaining < 0 || !storage(len(activeStatements)+1) || !nativeProofWork(work, 1) || sourceProofNil(st) || activeStatements[st] {
				valid = false
				return
			}
			activeStatements[st] = true
			if custom, ok := st.(*statements.CustomStatement); ok && !custom.HasSourceTransfer() && (custom.ThrownValue == nil || !custom.HasOriginPC) {
				valid = false
				return
			}
			roots, children, known := nativeSourceNameChildren(st)
			if !known {
				valid = false
				return
			}
			if assign, ok := st.(*statements.AssignStatement); ok && assign.ArrayMember == nil {
				left, knownLeft := sourceBoundedLocalOperand(assign.LeftValue)
				ref, local := left.(*values.JavaRef)
				local = local && knownLeft
				if local && ref != nil && ref.Id != nil && !ref.IsThis && !ref.IsParam && ref.CustomValue == nil && ref.StackVar == nil {
					// Even a dead store's initializer is evaluated in the original method.
					// Retain every call/cast/read/class-initialization effect. The narrow pure
					// profile below consists only of inert local/literal/null loads.
					if assign.HasOriginPC && assign.OriginPC >= 0 && pure(ref, assign.JavaValue) {
						if stores[assign] == nil && !storage(len(stores)+1) {
							valid = false
							return
						}
						stores[assign] = ref.Id
					}
					roots = []values.JavaValue{assign.JavaValue}
				}
			}
			if tr, ok := st.(*statements.TryCatchStatement); ok {
				for _, ref := range tr.Exception {
					value(ref)
				}
			}
			for _, v := range roots {
				value(v)
			}
			for _, child := range children {
				walk(child)
			}
			delete(activeStatements, st)
		}
	}
	walk(body)
	if !valid || len(stores) == 0 {
		return body
	}
	dead := map[*statements.AssignStatement]bool{}
	for st, id := range stores {
		if !reads[id] {
			if !storage(len(dead) + 1) {
				return body
			}
			dead[st] = true
		}
	}
	if len(dead) == 0 {
		return body
	}
	// Produce a private source view. CFG, original statement identities, and
	// variable rebinding retain their complete original trees.
	var filter func([]statements.Statement) []statements.Statement
	filter = func(list []statements.Statement) []statements.Statement {
		if !valid {
			return nil
		}
		if work != nil && work.CheckAlloc(int64(len(list))*16) != nil {
			valid = false
			return nil
		}
		out := make([]statements.Statement, 0, len(list))
		for _, st := range list {
			if assign, ok := st.(*statements.AssignStatement); ok && dead[assign] {
				continue
			}
			switch x := st.(type) {
			case *statements.IfStatement:
				if !storage(1) {
					valid = false
					return nil
				}
				clone := *x
				clone.IfBody = filter(x.IfBody)
				clone.ElseBody = filter(x.ElseBody)
				st = &clone
			case *statements.TryCatchStatement:
				if !storage(1) || work != nil && work.CheckAlloc(int64(len(x.CatchBodies))*24) != nil {
					valid = false
					return nil
				}
				clone := *x
				clone.TryBody = filter(x.TryBody)
				clone.CatchBodies = make([][]statements.Statement, len(x.CatchBodies))
				for i, b := range x.CatchBodies {
					clone.CatchBodies[i] = filter(b)
				}
				st = &clone
			case *statements.SynchronizedStatement:
				if !storage(1) {
					valid = false
					return nil
				}
				clone := *x
				clone.Body = filter(x.Body)
				st = &clone
			case *statements.DoWhileStatement:
				if !storage(1) {
					valid = false
					return nil
				}
				clone := *x
				clone.Body = filter(x.Body)
				st = &clone
			case *statements.WhileStatement:
				if !storage(1) {
					valid = false
					return nil
				}
				clone := *x
				clone.Body = filter(x.Body)
				st = &clone
			}
			if !valid {
				return nil
			}
			out = append(out, st)
		}
		return out
	}
	out := filter(body)
	if !valid {
		return body
	}
	return out
}

// Slot wrappers are transparent loads, but malformed cyclic simulator operands
// must not recurse indefinitely before the complete dependency proof sees them.
func sourceBoundedLocalOperand(v values.JavaValue) (values.JavaValue, bool) {
	for depth := 0; depth < 128; depth++ {
		if sourceProofNil(v) {
			return nil, true
		}
		slot, ok := v.(*values.SlotValue)
		if !ok {
			return v, true
		}
		v = slot.GetValue()
	}
	return nil, false
}
