package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// The first producer profile uses actual top-level declarations. The declaration
// stays after every captured definition and before every original allocation.
// A nested declaration/phi requires a lexical block/dominance certificate of its
// own; equal source text or an equal type cannot move its computation outward.
func (c *ClassObjectDumper) nativeMethodLocalProducerBindings(local *nativeMethodLocalClass, body []statements.Statement, params map[int]*values.JavaRef) (map[string]*values.JavaRef, statements.Statement, bool) {
	if local == nil || local.constructor == nil || len(local.allocations) == 0 {
		return nil, nil, false
	}
	var site nativeMethodLocalAllocation
	for _, s := range local.allocations {
		site = s
		break
	}
	result := map[string]*values.JavaRef{}
	last := -1
	for field, index := range local.constructor.captures {
		if index >= len(site.origins) || index >= len(site.slots) {
			return nil, nil, false
		}
		if site.origins[index].Kind != ssabuild.OriginInstr {
			continue
		}
		var found *values.JavaRef
		position := -1
		for i, st := range body {
			if !nativeProofWork(c.Work, 1) {
				return nil, nil, false
			}
			assign, ok := st.(*statements.AssignStatement)
			if !ok || assign == nil || assign.ArrayMember != nil || !(assign.IsFirst || assign.IsDeclare) || !assign.HasOriginPC {
				continue
			}
			left, bounded := nativeMemberEnclosingUnpack(assign.LeftValue, c.Work)
			if !bounded {
				return nil, nil, false
			}
			ref, ok := left.(*values.JavaRef)
			if !ok || ref == nil || ref.Id == nil {
				continue
			}
			pc, slot, known := ref.OriginalLocalDeclaration(assign.JavaValue)
			store, stored := site.stores[pc]
			if !known || pc != assign.OriginPC || !stored || store.slot != slot || store.origin != site.origins[index] {
				continue
			}
			if !c.nativeMethodLocalOriginalProducerInvocation(local, site, site.origins[index], assign.JavaValue, params) {
				return nil, nil, false
			}
			declaration, stable := nativeCaptureDeclaration(body, ref, false, c.Work)
			if !stable || declaration != assign || found != nil {
				return nil, nil, false
			}
			found = ref
			position = i
		}
		if found == nil {
			return nil, nil, false
		}
		result[field] = found
		if position > last {
			last = position
		}
	}
	if last < 0 {
		return result, nil, true
	}
	if last+1 >= len(body) {
		return nil, nil, false
	}
	// Match every actual source NEW to the independently recorded bytecode site.
	// A source body referencing a capture before its declaration is never repaired
	// by hoisting/repeating the producer or by materializing a different value.
	seen := map[int]bool{}
	active := map[values.JavaValue]bool{}
	remaining := 16384
	var value func(values.JavaValue, int) bool
	value = func(v values.JavaValue, position int) bool {
		if c.Work != nil {
			if c.Work.Enter(workbudget.CounterASTDepth) != nil {
				return false
			}
			defer c.Work.Leave(workbudget.CounterASTDepth)
		}
		remaining--
		if remaining < 0 || sourceProofNil(v) || active[v] || !nativeProofWork(c.Work, 1) {
			return false
		}
		active[v] = true
		defer delete(active, v)
		if n, ok := v.(*values.NewExpression); ok && n != nil && n.ConstructorCall != nil && strings.ReplaceAll(n.ConstructorCall.ClassName, ".", "/") == local.object.GetClassName() {
			if !n.HasOriginPC || position <= last {
				return false
			}
			if _, known := local.allocations[n.OriginPC]; !known {
				return false
			}
			seen[n.OriginPC] = true
		}
		if call, ok := v.(*values.FunctionCallExpression); ok {
			if !call.IsStatic && !value(call.Object, position) {
				return false
			}
			for _, a := range call.Arguments {
				if !value(a, position) {
					return false
				}
			}
			return true
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, v := range children {
			if !value(v, position) {
				return false
			}
		}
		return true
	}
	activeStatement := map[statements.Statement]bool{}
	var walk func([]statements.Statement, int) bool
	walk = func(ss []statements.Statement, position int) bool {
		if c.Work != nil {
			if c.Work.Enter(workbudget.CounterASTDepth) != nil {
				return false
			}
			defer c.Work.Leave(workbudget.CounterASTDepth)
		}
		for _, st := range ss {
			remaining--
			if remaining < 0 || sourceProofNil(st) || activeStatement[st] || !nativeProofWork(c.Work, 1) {
				return false
			}
			activeStatement[st] = true
			roots, children, known := nativeSourceNameChildren(st)
			if !known {
				return false
			}
			for _, v := range roots {
				if !value(v, position) {
					return false
				}
			}
			for _, ss := range children {
				if !walk(ss, position) {
					return false
				}
			}
			delete(activeStatement, st)
		}
		return true
	}
	for i, st := range body {
		if !walk([]statements.Statement{st}, i) {
			return nil, nil, false
		}
	}
	if len(seen) != len(local.allocations) {
		return nil, nil, false
	}
	for pc := range local.allocations {
		if !seen[pc] {
			return nil, nil, false
		}
	}
	return result, body[last+1], true
}

// STORE identity must retain the actual invocation and computational operands,
// not only its pointer or return type. This profile admits parameter/THIS
// operands; instruction/phi receiver or arguments need their own source proof.
func (c *ClassObjectDumper) nativeMethodLocalOriginalProducerInvocation(local *nativeMethodLocalClass, site nativeMethodLocalAllocation, origin ssabuild.Origin, value values.JavaValue, params map[int]*values.JavaRef) bool {
	original, known := site.producers[int(origin.PC)]
	if !known || origin.Kind != ssabuild.OriginInstr {
		return false
	}
	for depth := 0; depth < 32; depth++ {
		if sourceProofNil(value) || !nativeProofWork(c.Work, 1) {
			return false
		}
		if wrapped, ok := value.(*values.SlotValue); ok {
			value = wrapped.GetValue()
			continue
		}
		break
	}
	call, known := value.(*values.FunctionCallExpression)
	ins, record := original.instruction, original.record
	if !known || call == nil || !call.HasOriginPC || call.OriginPC != int(ins.PC) || strings.ReplaceAll(call.ClassName, ".", "/") != ins.Class || call.FunctionName != ins.Member || call.Descriptor != ins.Desc || ins.Member == "<init>" {
		return false
	}
	static := ins.Opcode == core.OP_INVOKESTATIC
	kind := values.InvokeVirtual
	switch ins.Opcode {
	case core.OP_INVOKESTATIC:
		kind = values.InvokeStatic
	case core.OP_INVOKEINTERFACE:
		kind = values.InvokeInterface
	case core.OP_INVOKESPECIAL:
		kind = values.InvokeSpecial
	case core.OP_INVOKEVIRTUAL:
	default:
		return false
	}
	descriptors, _, err := callbinding.Descriptor(ins.Desc)
	if err != nil || call.Kind != kind || call.IsStatic != static || call.IsSpecialInvoke != (kind == values.InvokeSpecial) || len(call.Arguments) != len(descriptors) {
		return false
	}
	methodParams, _, err := callbinding.Descriptor(local.owner.descriptor)
	if err != nil {
		return false
	}
	typesBySlot := map[int]string{}
	slot := 0
	if local.owner.declaration.AccessFlags&8 == 0 {
		typesBySlot[0] = "L" + local.owner.owner + ";"
		slot = 1
	}
	for _, d := range methodParams {
		typesBySlot[slot] = d
		slot++
		if d == "J" || d == "D" {
			slot++
		}
	}
	operands := call.Arguments
	expect := append([]string(nil), descriptors...)
	if !static {
		operands = append([]values.JavaValue{call.Object}, operands...)
		expect = append([]string{"L" + ins.Class + ";"}, expect...)
	}
	if len(record.Uses) != len(operands) {
		return false
	}
	for i, operand := range operands {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		o := record.Uses[i]
		if o.Kind != ssabuild.OriginParam {
			return false
		}
		for depth := 0; depth < 32; depth++ {
			if cast, ok := operand.(*values.CastExpression); ok && cast != nil && cast.Binding {
				erased, known := values.SourceTypeErasure(cast.TargetType, c.FuncCtx)
				if !known || erased != expect[i] {
					return false
				}
				operand = cast.Value
				continue
			}
			if wrapped, ok := operand.(*values.SlotValue); ok && wrapped != nil {
				operand = wrapped.GetValue()
				continue
			}
			break
		}
		ref, known := operand.(*values.JavaRef)
		if !known || ref == nil || params[o.Slot] == nil || params[o.Slot].Id != ref.Id || typesBySlot[o.Slot] == "" || !nativeMethodLocalParameterOperand(ref, o.Slot, typesBySlot[o.Slot], c.FuncCtx, c.Work) {
			return false
		}
	}
	return true
}
