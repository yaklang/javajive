package rewriter

import (
	"reflect"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Moving a declaration has no evaluation step. For a single normal monitor
// frontier, primitive live-outs may keep their original stores inside the
// monitor and acquire an uninitialized declaration before it. A bounded
// definite-assignment proof must establish every released read; catch arms
// start with the try's entry state, never its successful body's final state.
// Reference lifetimes, opaque captures and expression-valued aliases require
// other proofs and are deliberately not covered by this storage projection.
func monitorPrimitiveLiveOut(held, tail []statements.Statement, rows [][2]int, owner int) (prefix, rewritten []statements.Statement, known bool) {
	needed := map[*utils.VariableId]*values.JavaRef{}
	var order []*values.JavaRef
	released := func(pc int) bool { return !finallyContains(rows, pc) }
	if !monitorContinuationScope(held, [][]statements.Statement{tail}, released, owner, func(ref *values.JavaRef) bool {
		if !monitorPrimitiveLocal(ref) {
			return false
		}
		if old := needed[ref.Id]; old != nil {
			return reflect.DeepEqual(old.Type().RawType(), ref.Type().RawType())
		}
		needed[ref.Id] = ref
		order = append(order, ref)
		return true
	}) {
		return nil, nil, false
	}
	if len(needed) == 0 {
		return nil, held, true
	}
	if len(needed) > 32 {
		return nil, nil, false
	}
	p := monitorLiveOutProof{needed: needed, rows: rows, remaining: 4096}
	body, state, normal, valid := p.block(held, map[*utils.VariableId]bool{}, 0)
	if !valid || !normal {
		return nil, nil, false
	}
	for _, ref := range order {
		if !state[ref.Id] {
			return nil, nil, false
		}
		prefix = append(prefix, statements.NewDeclareStatement(ref))
	}
	if !monitorContinuationLocalsClosed(body, [][]statements.Statement{tail}, released, owner) {
		return nil, nil, false
	}
	return prefix, body, true
}

func monitorPrimitiveLocal(ref *values.JavaRef) bool {
	if ref == nil || ref.Id == nil || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || ref.Type() == nil {
		return false
	}
	typ, known := ref.Type().RawType().(*types.JavaPrimer)
	if !known || typ == nil {
		return false
	}
	switch typ.Name {
	case types.JavaBoolean, types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble:
	default:
		return false
	}
	return ref.WebDeclType == nil || reflect.DeepEqual(ref.WebDeclType.RawType(), ref.Type().RawType())
}

type monitorLiveOutProof struct {
	needed    map[*utils.VariableId]*values.JavaRef
	rows      [][2]int
	remaining int
}

func monitorLiveOutState(source map[*utils.VariableId]bool) map[*utils.VariableId]bool {
	copy := make(map[*utils.VariableId]bool, len(source))
	for id, assigned := range source {
		copy[id] = assigned
	}
	return copy
}

func (p *monitorLiveOutProof) read(root values.JavaValue, assigned map[*utils.VariableId]bool) bool {
	active := map[values.JavaValue]bool{}
	var visit func(values.JavaValue, int) bool
	visit = func(v values.JavaValue, depth int) bool {
		p.remaining--
		if p.remaining < 0 || depth > 32 || v == nil || reflect.ValueOf(v).Kind() == reflect.Ptr && reflect.ValueOf(v).IsNil() || active[v] {
			return false
		}
		active[v] = true
		defer delete(active, v)
		if ref, ok := v.(*values.JavaRef); ok {
			if original := p.needed[ref.Id]; original != nil {
				return monitorPrimitiveLocal(ref) && reflect.DeepEqual(original.Type().RawType(), ref.Type().RawType()) && assigned[ref.Id]
			}
		}
		children, known := values.Children(v)
		if call, ok := v.(*values.FunctionCallExpression); ok {
			children = append([]values.JavaValue(nil), call.Arguments...)
			if !call.IsStatic {
				children = append(children, call.Object)
			}
			known = true // Original effect domains were proved by the partition.
		}
		if allocation, ok := v.(*values.NewExpression); ok && !allocation.IsArray() && allocation.ConstructorCall != nil {
			children, known = allocation.ConstructorCall.Arguments, true
		}
		if !known {
			return false
		}
		for _, child := range children {
			if !visit(child, depth+1) {
				return false
			}
		}
		return true
	}
	return visit(root, 0)
}

func (p *monitorLiveOutProof) block(input []statements.Statement, entry map[*utils.VariableId]bool, depth int) (out []statements.Statement, state map[*utils.VariableId]bool, normal, known bool) {
	if depth > 32 {
		return nil, nil, false, false
	}
	state = monitorLiveOutState(entry)
	read := func(v values.JavaValue) bool { return v == nil || p.read(v, state) }
	for i, st := range input {
		p.remaining--
		if p.remaining < 0 || st == nil {
			return nil, nil, false, false
		}
		switch x := st.(type) {
		case *statements.AssignStatement:
			if x == nil || !read(x.JavaValue) || x.ArrayMember != nil && !read(x.ArrayMember) {
				return nil, nil, false, false
			}
			ref, local := x.LeftValue.(*values.JavaRef)
			if !local || ref == nil || p.needed[ref.Id] == nil {
				if !local && !read(x.LeftValue) {
					return nil, nil, false, false
				}
				break
			}
			if !monitorPrimitiveLocal(ref) || !reflect.DeepEqual(ref.Type().RawType(), p.needed[ref.Id].Type().RawType()) || x.ArrayMember != nil {
				return nil, nil, false, false
			}
			if x.IsDeclare && x.JavaValue == nil {
				continue // Only the corresponding bare declaration is hoisted.
			}
			if x.JavaValue == nil || !x.HasOriginPC || !finallyContains(p.rows, x.OriginPC) {
				return nil, nil, false, false
			}
			clone := *x
			clone.IsFirst, clone.IsDeclare = false, false
			st = &clone
			state[ref.Id] = true
		case *statements.IfStatement:
			if x == nil || !read(x.Condition) {
				return nil, nil, false, false
			}
			clone := *x
			a, as, an, valid := p.block(x.IfBody, state, depth+1)
			if !valid {
				return nil, nil, false, false
			}
			b, bs, bn, valid := p.block(x.ElseBody, state, depth+1)
			if !valid {
				return nil, nil, false, false
			}
			clone.IfBody, clone.ElseBody = a, b
			st = &clone
			if !an && !bn {
				return append(out, st), state, false, i == len(input)-1
			}
			for id := range p.needed {
				state[id] = (!an || as[id]) && (!bn || bs[id])
			}
		case *statements.TryCatchStatement:
			if x == nil || len(x.Exception) != len(x.CatchBodies) || len(x.Handlers) != len(x.CatchBodies) {
				return nil, nil, false, false
			}
			clone := *x
			var success bool
			var valid bool
			var joined map[*utils.VariableId]bool
			clone.TryBody, joined, success, valid = p.block(x.TryBody, state, depth+1)
			if !valid {
				return nil, nil, false, false
			}
			clone.CatchBodies = make([][]statements.Statement, len(x.CatchBodies))
			for j, body := range x.CatchBodies {
				var cs map[*utils.VariableId]bool
				var cn bool
				clone.CatchBodies[j], cs, cn, valid = p.block(body, state, depth+1)
				if !valid {
					return nil, nil, false, false
				}
				if cn {
					if !success {
						joined = cs
					} else {
						for id := range p.needed {
							joined[id] = joined[id] && cs[id]
						}
					}
					success = true
				}
			}
			st = &clone
			if !success {
				return append(out, st), state, false, i == len(input)-1
			}
			state = joined
		case *statements.SynchronizedStatement:
			if x == nil || !read(x.Argument) {
				return nil, nil, false, false
			}
			clone := *x
			var completes, valid bool
			clone.Body, state, completes, valid = p.block(x.Body, state, depth+1)
			if !valid {
				return nil, nil, false, false
			}
			st = &clone
			if !completes {
				return append(out, st), state, false, i == len(input)-1
			}
		case *statements.DoWhileStatement:
			if x == nil || !read(x.ConditionValue) {
				return nil, nil, false, false
			}
			clone := *x
			var valid bool
			clone.Body, _, _, valid = p.block(x.Body, state, depth+1)
			if !valid {
				return nil, nil, false, false
			}
			st = &clone // Conservatively grant no loop-only definition.
		case *statements.ExpressionStatement:
			if x == nil || !read(x.Expression) {
				return nil, nil, false, false
			}
		case *values.JavaExpression:
			if !read(x) {
				return nil, nil, false, false
			}
		case *statements.CustomStatement:
			if x == nil || x.ThrownValue == nil && !x.SourceTransferOnly() || x.ThrownValue != nil && !read(x.ThrownValue) {
				return nil, nil, false, false
			}
			if x.ThrownValue != nil {
				return append(out, st), state, false, i == len(input)-1
			}
		case *statements.ReturnStatement:
			if x == nil || !read(x.JavaValue) {
				return nil, nil, false, false
			}
			return append(out, st), state, false, i == len(input)-1
		case *statements.MiddleStatement:
			if x == nil || x.Data != nil || x.Flag != "start" && x.Flag != "end" {
				return nil, nil, false, false
			}
		default:
			return nil, nil, false, false
		}
		out = append(out, st)
	}
	return out, state, true, true
}
