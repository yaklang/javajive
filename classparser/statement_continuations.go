package javaclassparser

import "github.com/yaklang/javajive/classparser/decompiler/core/statements"

// Normal completion of a nested block resumes its enclosing block. An empty
// catch at the end of an if arm can therefore reach a fallback after the if.
// Looking only for a sibling inside the arm invents a terminating throw and
// changes recoverable parse failures into exceptions.
func normalStatementContinuations(body []statements.Statement) map[statements.Statement]bool {
	out := map[statements.Statement]bool{}
	var visit func([]statements.Statement, bool)
	visit = func(body []statements.Statement, next bool) {
		for i := len(body) - 1; i >= 0; i-- {
			st := body[i]
			switch st.(type) {
			case *statements.MiddleStatement, *statements.StackAssignStatement:
				continue
			}
			previous, seen := out[st]
			out[st] = previous || next
			if !seen || (!previous && next) {
				switch s := st.(type) {
				case *statements.IfStatement:
					visit(s.IfBody, next)
					visit(s.ElseBody, next)
				case *statements.TryCatchStatement:
					visit(s.TryBody, next)
					for _, handler := range s.CatchBodies {
						visit(handler, next)
					}
				case *statements.SynchronizedStatement:
					visit(s.Body, next)
				case *statements.SwitchStatement:
					for j, c := range s.Cases {
						visit(c.Body, next || j+1 < len(s.Cases))
					}
				case *statements.DoWhileStatement:
					visit(s.Body, true) // normal completion reaches the loop test
				case *statements.WhileStatement:
					visit(s.Body, true)
				case *statements.ForStatement:
					visit(s.SubStatements, true) // step, then test
				}
			}
			next = true
		}
	}
	visit(body, false)
	return out
}
