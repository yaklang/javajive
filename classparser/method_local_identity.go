package javaclassparser

import (
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// nameMethodDeclarationIdentities gives distinct declaration identities distinct
// names throughout a method, including disjoint scopes. JVM slot reuse is not
// declaration identity. Keeping names injective also lets downstream source
// recovery retain the same identity distinction without merging unrelated types
// by their old slot spelling. This changes names, never declaration placement,
// types, values, or evaluation order.
func nameMethodDeclarationIdentities(params []values.JavaValue, body []statements.Statement) {
	var order []*utils.VariableId
	seen := map[*utils.VariableId]bool{}
	add := func(value values.JavaValue) {
		if id := localDeclVarId(value); id != nil && !seen[id] {
			seen[id] = true
			order = append(order, id)
		}
	}
	for _, param := range params {
		add(param)
	}
	var visit func([]statements.Statement)
	visit = func(list []statements.Statement) {
		for _, statement := range list {
			switch s := statement.(type) {
			case *statements.AssignStatement:
				if (s.IsFirst || s.IsDeclare) && s.ArrayMember == nil {
					add(s.LeftValue)
				}
			case *statements.IfStatement:
				visit(s.IfBody)
				visit(s.ElseBody)
			case *statements.DoWhileStatement:
				visit(s.Body)
			case *statements.WhileStatement:
				visit(s.Body)
			case *statements.ForStatement:
				if s.InitVar != nil {
					visit([]statements.Statement{s.InitVar})
				}
				visit(s.SubStatements)
			case *statements.SwitchStatement:
				for _, item := range s.Cases {
					visit(item.Body)
				}
			case *statements.SynchronizedStatement:
				visit(s.Body)
			case *statements.TryCatchStatement:
				visit(s.TryBody)
				for i, handler := range s.CatchBodies {
					if i < len(s.Exception) {
						add(s.Exception[i])
					}
					visit(handler)
				}
			}
		}
	}
	visit(body)
	// Reserve every existing spelling before allocating a suffix. Otherwise an
	// early duplicate could steal a later declaration's already unique name.
	original := map[string]bool{}
	for _, id := range order {
		original[id.String()] = true
	}
	used := map[string]*utils.VariableId{}
	for _, id := range order {
		name := id.String()
		if previous := used[name]; previous != nil && previous != id {
			for suffix := 1; ; suffix++ {
				candidate := fmt.Sprintf("%s_%d", name, suffix)
				if used[candidate] == nil && !original[candidate] {
					id.SetName(candidate)
					name = candidate
					break
				}
			}
		}
		used[name] = id
	}
}
