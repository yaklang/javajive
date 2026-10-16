package javaclassparser

import "github.com/yaklang/javajive/classparser/decompiler/core/statements"

// Copy structured child lists while preserving headers, monitors, handler
// ranges, labels and selectors. The caller owns the complete cycle, work and
// original-site proof, and commits value changes only after projection closes.
func nativeProjectStructuredBlocks(st statements.Statement, project func([]statements.Statement) ([]statements.Statement, bool)) (statements.Statement, bool) {
	if sourceProofNil(st) || project == nil {
		return nil, false
	}
	switch x := st.(type) {
	case *statements.IfStatement:
		copy := *x
		yes, known := project(x.IfBody)
		if !known {
			return nil, false
		}
		no, known := project(x.ElseBody)
		copy.IfBody, copy.ElseBody = yes, no
		return &copy, known
	case *statements.WhileStatement:
		copy := *x
		body, ok := project(x.Body)
		copy.Body = body
		return &copy, ok
	case *statements.DoWhileStatement:
		copy := *x
		body, ok := project(x.Body)
		copy.Body = body
		return &copy, ok
	case *statements.ForStatement:
		copy := *x
		body, ok := project(x.SubStatements)
		copy.SubStatements = body
		return &copy, ok
	case *statements.SynchronizedStatement:
		copy := *x
		body, ok := project(x.Body)
		copy.Body = body
		return &copy, ok
	case *statements.TryCatchStatement:
		copy := *x
		body, ok := project(x.TryBody)
		if !ok {
			return nil, false
		}
		copy.TryBody = body
		copy.CatchBodies = make([][]statements.Statement, len(x.CatchBodies))
		for i, handler := range x.CatchBodies {
			body, ok := project(handler)
			if !ok {
				return nil, false
			}
			copy.CatchBodies[i] = body
		}
		return &copy, true
	case *statements.SwitchStatement:
		copy := *x
		copy.Cases = make([]*statements.CaseItem, len(x.Cases))
		for i, arm := range x.Cases {
			if arm == nil {
				return nil, false
			}
			body, ok := project(arm.Body)
			if !ok {
				return nil, false
			}
			item := *arm
			item.Body = body
			copy.Cases[i] = &item
		}
		return &copy, true
	default:
		// New structured types must be explicitly copied before their children can
		// be rewritten; silently treating them as leaves would lose original sites.
		_, children, known := nativeSourceNameChildren(st)
		return st, known && len(children) == 0
	}
}
