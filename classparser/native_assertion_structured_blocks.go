package javaclassparser

import "github.com/yaklang/javajive/classparser/decompiler/core/statements"

// Assertion projection changes the proved flag/throw packet in place. Every
// structured child block participates, without moving loop headers, monitors,
// handler ranges, labels, switch discriminants, or any original effect. Copies
// retain control/exception evidence and leave the original source graph intact.
// The callback owns the shared cycle, work, and original-site uniqueness proof.
func nativeAssertionProjectBlocks(st statements.Statement, project func([]statements.Statement) ([]statements.Statement, bool)) (statements.Statement, bool) {
	if sourceProofNil(st) || project == nil {
		return nil, false
	}
	switch x := st.(type) {
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
		// If statements are projected by the caller's original assertion matcher.
		// New structured types must be explicitly copied before their children can
		// be rewritten; silently treating them as leaves would lose original sites.
		_, children, known := nativeSourceNameChildren(st)
		return st, known && len(children) == 0
	}
}
