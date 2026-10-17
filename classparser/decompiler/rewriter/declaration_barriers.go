package rewriter

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"reflect"
)

// DeclarationInsertionBarriers records actual typed declarations in this method.
// It only forbids a later source pass from inventing a method-scope declaration
// with the same spelling. It grants no liveness, dominance, value or alias fact.
// In particular, declaring a second outer local would invalidate a declaration
// already emitted in a nested block. Types and their rendered alphabets are not
// parsed to discover locals; independent VariableIds remain independent.
func DeclarationInsertionBarriers(root []statements.Statement, ctx *class_context.ClassContext, work *workbudget.Budget) (map[string]bool, error) {
	if work != nil {
		if err := work.CheckAlloc(int64(len(root)+1) * 64); err != nil {
			return nil, err
		}
	}
	names := map[string]bool{}
	seen := map[statements.Statement]bool{}
	pending := append([]statements.Statement(nil), root...)
	record := func(ref *values.JavaRef) error {
		if ref == nil || ref.Id == nil || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil {
			return nil
		}
		name := ref.Id.String()
		if ctx != nil && ctx.LocalNames[ref.Id] != "" {
			name = ctx.LocalNames[ref.Id]
		}
		if work != nil {
			if err := work.Charge(workbudget.CounterGraphScans, int64(len(name))+1); err != nil {
				return err
			}
		}
		names[name] = true
		return nil
	}
	for len(pending) > 0 {
		if work != nil {
			if err := work.Charge(workbudget.CounterGraphScans, 1); err != nil {
				return nil, err
			}
			if err := work.CheckAlloc(int64(len(seen)+len(names)+len(pending)+1) * 64); err != nil {
				return nil, err
			}
		}
		st := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if st == nil {
			continue
		}
		node := reflect.ValueOf(st)
		if !node.Type().Comparable() {
			return nil, fmt.Errorf("noncomparable statement in declaration census")
		}
		if node.Kind() == reflect.Ptr && node.IsNil() {
			continue
		}
		if seen[st] {
			continue
		}
		seen[st] = true
		switch s := st.(type) {
		case *statements.AssignStatement:
			if s != nil && (s.IsFirst || s.IsDeclare) && s.ArrayMember == nil {
				if ref, ok := values.UnpackSoltValue(s.LeftValue).(*values.JavaRef); ok {
					if err := record(ref); err != nil {
						return nil, err
					}
				}
			}
		case *statements.ForStatement:
			pending = append(pending, s.InitVar, s.Condition, s.EndExp)
		case *statements.TryCatchStatement:
			for _, ref := range s.Exception {
				if err := record(ref); err != nil {
					return nil, err
				}
			}
		}
		for _, child := range childStatementLists(st) {
			pending = append(pending, (*child)...)
		}
	}
	return names, nil
}
