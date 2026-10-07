package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// The seven-op compiler prefix joins with an empty stack. No source edge or
// exception handler may enter or cover it; every later effect stays in place.
func nativeAssertionInitializerPrefixClosed(d *core.Decompiler, code *CodeAttribute, ops []*core.OpCode, end int, work *workbudget.Budget) bool {
	if end <= 0 || !nativeAssertionRegionClosed(d, code, ops, 0, end, work) {
		return false
	}
	for _, h := range code.ExceptionTable {
		if h == nil || int(h.StartPc) < end || int(h.HandlerPc) < end {
			return false
		}
	}
	for _, op := range ops {
		if !nativeProofWork(work, 1) {
			return false
		}
		if int(op.CurrentOffset) < end {
			continue
		}
		switch op.Instr.OpCode {
		case core.OP_GOTO, core.OP_GOTO_W, core.OP_IFEQ, core.OP_IFNE, core.OP_IFLT, core.OP_IFGE, core.OP_IFGT, core.OP_IFLE, core.OP_IF_ICMPEQ, core.OP_IF_ICMPNE, core.OP_IF_ICMPLT, core.OP_IF_ICMPGE, core.OP_IF_ICMPGT, core.OP_IF_ICMPLE, core.OP_IF_ACMPEQ, core.OP_IF_ACMPNE, core.OP_IFNULL, core.OP_IFNONNULL:
			width := 2
			if op.Instr.OpCode == core.OP_GOTO_W {
				width = 4
			}
			target, e := core.BranchTarget(int(op.CurrentOffset), op.Data, width, len(code.Code))
			if e != nil || target < end {
				return false
			}
		case core.OP_TABLESWITCH, core.OP_LOOKUPSWITCH:
			if int(op.SwitchDefaultOffset) < end {
				return false
			}
			valid := true
			op.SwitchJmpCase.ForEach(func(_ int, target int32) bool {
				if int(target) < end || !nativeProofWork(work, 1) {
					valid = false
					return false
				}
				return true
			})
			if !valid {
				return false
			}
		}
	}
	return true
}

// Remove only the original certified flag assignment. Interpret the tiny
// boolean view for both status values and require one exact original call in
// each evaluation; this accepts compiler structuring variants, not source text.
func (c *ClassObjectDumper) projectNativeAssertionInitializer(body []statements.Statement, p *nativeMemberAssertion) ([]statements.Statement, bool) {
	if p == nil || len(body) == 0 || !nativeProofWork(c.Work, 1) {
		return nil, false
	}
	assignment, ok := body[0].(*statements.AssignStatement)
	if !ok || assignment == nil || !assignment.HasOriginPC || assignment.OriginPC != p.initializerStorePC || assignment.IsDeclare || assignment.ArrayMember != nil {
		return nil, false
	}
	field, ok := assignment.LeftValue.(*values.JavaClassMember)
	if !ok || field == nil || field.RefKind != 0 || strings.ReplaceAll(field.Name, ".", "/") != c.obj.GetClassName() || field.Member != nativeAssertionField || field.Description != "Z" {
		return nil, false
	}
	for _, input := range []bool{false, true} {
		calls := 0
		remaining := 64
		active := map[values.JavaValue]bool{}
		var evaluate func(values.JavaValue) (bool, bool)
		evaluate = func(v values.JavaValue) (bool, bool) {
			if sourceProofNil(v) || remaining <= 0 || active[v] || !nativeProofWork(c.Work, 1) {
				return false, false
			}
			remaining--
			active[v] = true
			defer delete(active, v)
			switch x := v.(type) {
			case *values.SlotValue:
				return evaluate(x.GetValue())
			case *values.JavaLiteral:
				switch n := x.Data.(type) {
				case bool:
					return n, true
				case int:
					return n != 0, n == 0 || n == 1
				case int32:
					return n != 0, n == 0 || n == 1
				}
				return false, false
			case *values.FunctionCallExpression:
				if x == nil || !x.HasOriginPC || x.OriginPC != p.statusInvokePC || x.Kind != values.InvokeVirtual || x.IsStatic || x.IsSpecialInvoke || x.ClassName != "java.lang.Class" || x.FunctionName != "desiredAssertionStatus" || x.Descriptor != "()Z" || len(x.Arguments) != 0 {
					return false, false
				}
				literal, ok := values.UnpackSoltValue(x.Object).(*values.JavaClassValue)
				if !ok || literal == nil || !literal.HasOriginPC || literal.OriginPC != 0 {
					return false, false
				}
				name, known := types.RawClassFQN(literal.JavaType)
				if !known || strings.ReplaceAll(name, ".", "/") != p.statusOwner {
					return false, false
				}
				calls++
				return input, true
			case *values.JavaExpression:
				if x.Op == values.Not && len(x.Values) == 1 {
					v, ok := evaluate(x.Values[0])
					return !v, ok
				}
				if (x.Op == values.EQ || x.Op == values.NEQ) && len(x.Values) == 2 {
					a, ak := evaluate(x.Values[0])
					b, bk := evaluate(x.Values[1])
					if x.Op == values.EQ {
						return a == b, ak && bk
					}
					return a != b, ak && bk
				}
				return false, false
			case *values.TernaryExpression:
				condition, ok := evaluate(x.Condition)
				if !ok {
					return false, false
				}
				if condition {
					return evaluate(x.TrueValue)
				}
				return evaluate(x.FalseValue)
			}
			return false, false
		}
		result, known := evaluate(assignment.JavaValue)
		if !known || result == input || calls != 1 {
			return nil, false
		}
	}
	return body[1:], true
}
