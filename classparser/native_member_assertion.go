package javaclassparser

import (
	"encoding/binary"
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

const nativeAssertionField = "$assertionsDisabled"

type nativeMemberAssertion struct {
	initializer                                          *MemberInfo
	pureInitializer                                      bool
	initializerEndPC, initializerStorePC, statusInvokePC int
	statusOwner                                          string
	reads                                                map[string]map[int]*nativeAssertionPacket
}

// Source assert regenerates a field and initializer. Establish their physical
// compiler profile first; arbitrary synthetic fields are not source assertions.
func nativeMemberAssertionProof(obj *ClassObject, outermost string, work *workbudget.Budget) (*nativeMemberAssertion, bool) {
	return nativeMemberAssertionProofMode(obj, outermost, work, false)
}

func nativeMemberAssertionProofMode(obj *ClassObject, outermost string, work *workbudget.Budget, allowSuffix bool) (*nativeMemberAssertion, bool) {
	if obj == nil || outermost == "" || !nativeProofWork(work, 1) {
		return nil, false
	}
	var flag *MemberInfo
	for _, f := range obj.Fields {
		if f == nil || !nativeProofWork(work, 1) {
			return nil, false
		}
		name, ok := sourceBridgeUTF8(obj, f.NameIndex)
		if !ok {
			return nil, false
		}
		if name == nativeAssertionField {
			if flag != nil {
				return nil, false
			}
			flag = f
		}
	}
	if flag == nil {
		return nil, true
	}
	desc, ok := sourceBridgeUTF8(obj, flag.DescriptorIndex)
	flags, onlySynthetic, known := nativeMemberEffectiveFieldFlags(flag, work)
	if !ok || desc != "Z" || !known || !onlySynthetic || flags != 0x1018 || obj.MinorVersion != 0 || (obj.MajorVersion != 51 && obj.MajorVersion != 52) {
		return nil, false
	}
	// Hidden field handles cannot borrow the ordinary assertion-read proof.
	for _, constant := range obj.ConstantPool {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		handle, ok := constant.(*ConstantMethodHandleInfo)
		if !ok {
			continue
		}
		if handle == nil || handle.ReferenceIndex == 0 || int(handle.ReferenceIndex) > len(obj.ConstantPool) {
			return nil, false
		}
		ref, ok := obj.ConstantPool[handle.ReferenceIndex-1].(*ConstantFieldrefInfo)
		if !ok {
			continue
		}
		if ref == nil {
			return nil, false
		}
		owner, ok := sourceBridgeClassName(obj, ref.ClassIndex)
		if !ok {
			return nil, false
		}
		if owner != obj.GetClassName() {
			continue
		}
		if ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(obj.ConstantPool) {
			return nil, false
		}
		nt, ok := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
		if !ok || nt == nil {
			return nil, false
		}
		name, ok := sourceBridgeUTF8(obj, nt.NameIndex)
		if !ok || name == nativeAssertionField {
			return nil, false
		}
	}
	plan := &nativeMemberAssertion{reads: map[string]map[int]*nativeAssertionPacket{}}
	reads := 0
	for _, m := range obj.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return nil, false
		}
		name, nk := sourceBridgeUTF8(obj, m.NameIndex)
		ds, dk := sourceBridgeUTF8(obj, m.DescriptorIndex)
		if !nk || !dk {
			return nil, false
		}
		var code *CodeAttribute
		for _, a := range m.Attributes {
			if c, ok := a.(*CodeAttribute); ok {
				if code != nil || c == nil {
					return nil, false
				}
				code = c
			}
		}
		if code == nil {
			continue
		}
		if len(code.Code) > 65535 || !nativeProofWork(work, int64(len(code.Code))) {
			return nil, false
		}
		decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
		decoder.Work = work
		if decoder.ParseOpcode() != nil {
			return nil, false
		}
		ops := constructorMotionOps(decoder)
		if name == "<clinit>" {
			if plan.initializer != nil || m.AccessFlags != 8 || ds != "()V" || len(m.Attributes) != 1 || code.MaxStack < 1 || len(ops) < 8 || (!allowSuffix && (code.MaxLocals != 0 || code.MaxStack != 1 || len(code.ExceptionTable) != 0 || len(ops) != 8)) {
				return nil, false
			}
			cp := ops[0]
			index := 0
			if cp.Instr.OpCode == core.OP_LDC && len(cp.Data) == 1 {
				index = int(cp.Data[0])
			} else if cp.Instr.OpCode == core.OP_LDC_W && len(cp.Data) == 2 {
				index = int(binary.BigEndian.Uint16(cp.Data))
			} else {
				return nil, false
			}
			owner, ok := sourceBridgeClassName(obj, uint16(index))
			invoke := constructorMotionMember(obj, ops[1], core.OP_INVOKEVIRTUAL)
			store := constructorMotionMember(obj, ops[6], core.OP_PUTSTATIC)
			branch := func(op *core.OpCode, target int) bool {
				return len(op.Data) == 2 && int(op.CurrentOffset)+int(int16(binary.BigEndian.Uint16(op.Data))) == target
			}
			if !ok || owner != outermost || invoke == nil || invoke.Name != "java/lang/Class" || invoke.Member != "desiredAssertionStatus" || invoke.Description != "()Z" || ops[2].Instr.OpCode != core.OP_IFNE || !branch(ops[2], int(ops[5].CurrentOffset)) || ops[3].Instr.OpCode != core.OP_ICONST_1 || ops[4].Instr.OpCode != core.OP_GOTO || !branch(ops[4], int(ops[6].CurrentOffset)) || ops[5].Instr.OpCode != core.OP_ICONST_0 {
				return nil, false
			}
			if store == nil || store.Name != obj.GetClassName() || store.Member != nativeAssertionField || store.Description != "Z" || !allowSuffix && ops[7].Instr.OpCode != core.OP_RETURN {
				return nil, false
			}
			end := int(ops[7].CurrentOffset)
			if allowSuffix && !nativeAssertionInitializerPrefixClosed(decoder, code, ops, end, work) {
				return nil, false
			}
			plan.initializer = m
			plan.pureInitializer = len(ops) == 8 && ops[7].Instr.OpCode == core.OP_RETURN
			plan.initializerEndPC, plan.initializerStorePC, plan.statusInvokePC, plan.statusOwner = end, int(ops[6].CurrentOffset), int(ops[1].CurrentOffset), outermost
			if plan.pureInitializer {
				continue
			}
		}
		sites := map[int]*nativeAssertionPacket{}
		for i, op := range ops {
			if name == "<clinit>" && i < 7 {
				continue
			}
			for _, kind := range []int{core.OP_GETSTATIC, core.OP_PUTSTATIC, core.OP_GETFIELD, core.OP_PUTFIELD} {
				field := constructorMotionMember(obj, op, kind)
				if field == nil || field.Name != obj.GetClassName() || field.Member != nativeAssertionField {
					continue
				}
				if kind != core.OP_GETSTATIC || field.Description != "Z" || i+1 >= len(ops) || ops[i+1].Instr.OpCode != core.OP_IFNE || len(ops[i+1].Data) != 2 {
					return nil, false
				}
				join := int(ops[i+1].CurrentOffset) + int(int16(binary.BigEndian.Uint16(ops[i+1].Data)))
				if join < 0 || join >= len(code.Code) {
					return nil, false
				}
				packet := nativeAssertionBytecodePacket(obj, ops, i, work)
				if packet == nil || len(sites) >= 256 || !nativeAssertionRegionClosed(decoder, code, ops, int(op.CurrentOffset), packet.endPC, work) || !nativeAssertionPacketExitsClosed(ops, i, packet.endPC, join, len(code.Code), work) {
					return nil, false
				}
				sites[int(op.CurrentOffset)] = packet
				reads++
			}
		}
		plan.reads[name+ds] = sites
	}
	return plan, plan.initializer != nil && reads > 0
}

type nativeAssertStatement struct {
	condition, message values.JavaValue
	messageCall        *values.FunctionCallExpression
}

func (s *nativeAssertStatement) ReplaceVar(old, new *coreutils.VariableId) {
	s.condition.ReplaceVar(old, new)
	if s.message != nil {
		s.message.ReplaceVar(old, new)
	}
}
func (s *nativeAssertStatement) String(ctx *class_context.ClassContext) string {
	text := "assert " + values.SimplifyConditionValue(s.condition).String(ctx)
	if s.message != nil {
		text += " : " + s.messageCall.ArgumentStrings(ctx)[0]
	}
	return text
}

// Match the original flag read and exception allocation back to typed source
// operands. A whole family refuses if any flag read cannot be reconstructed.
func (c *ClassObjectDumper) prepareNativeAssertions(name, desc string, body []statements.Statement) ([]statements.Statement, error) {
	plan := c.nativeAssertionProtocol()
	if plan == nil {
		return body, nil
	}
	if plan.initializer != nil && !plan.pureInitializer && name == "<clinit>" && desc == "()V" {
		var valid bool
		body, valid = c.projectNativeAssertionInitializer(body, plan)
		if !valid {
			return nil, fmt.Errorf("assertion initialization source occurrence unproved")
		}
	}
	sites := plan.reads[name+desc]
	if len(sites) == 0 {
		return body, nil
	}
	if !nativeAssertionSourceReads(body, c.obj.GetClassName(), sites, c.Work) {
		return nil, fmt.Errorf("assertion source read closure unproved")
	}
	seen := map[int]bool{}
	remaining := 8192
	active := map[statements.Statement]bool{}
	var project func([]statements.Statement) ([]statements.Statement, bool)
	project = func(body []statements.Statement) ([]statements.Statement, bool) {
		out := []statements.Statement{}
		for _, st := range body {
			remaining--
			if remaining < 0 || !nativeProofWork(c.Work, 1) || sourceProofNil(st) || active[st] {
				return nil, false
			}
			active[st] = true
			if branch, ok := st.(*statements.IfStatement); ok {
				cond, known := nativeMemberEnclosingUnpack(branch.Condition, c.Work)
				if !known {
					return nil, false
				}
				field, failure, guardKnown := nativeAssertionFailureGuard(cond, types.SlashToDot(c.obj.GetClassName()), c.Work, 64)
				if guardKnown && len(branch.IfBody) == 1 {
					packet, exists := sites[field.OriginPC]
					thrown, ok := branch.IfBody[0].(*statements.CustomStatement)
					if !exists || seen[field.OriginPC] || !ok || thrown == nil || !thrown.HasOriginPC || thrown.OriginPC != packet.throwPC {
						return nil, false
					}
					v, valid := nativeMemberEnclosingUnpack(thrown.ThrownValue, c.Work)
					allocation, ok := v.(*values.NewExpression)
					if !valid || !ok || allocation == nil || allocation.Type() == nil || !nativeAssertionErrorType(allocation.Type()) || !allocation.HasOriginPC || allocation.OriginPC != packet.newPC {
						return nil, false
					}
					call := allocation.ConstructorCall

					if call == nil || !call.HasOriginPC || call.OriginPC != packet.invokePC || call.Descriptor != packet.descriptor || call.Object != allocation || call.ClassName != "java.lang.AssertionError" || call.FunctionName != "<init>" || call.Kind != values.InvokeSpecial {
						return nil, false
					}
					args, ret, err := nativeAssertionConstructorDescriptor(call.Descriptor)
					if err || ret != "V" || len(call.Arguments) != len(args) || len(args) > 1 {
						return nil, false
					}
					assertion := &nativeAssertStatement{condition: values.NewUnaryExpression(failure, values.Not, types.NewJavaPrimer(types.JavaBoolean)), messageCall: call}
					if len(args) == 1 {
						assertion.message = call.Arguments[0]
					}
					seen[field.OriginPC] = true
					out = append(out, assertion)
					continuation, ok := project(branch.ElseBody)
					if !ok {
						return nil, false
					}
					out = append(out, continuation...)
					delete(active, st)
					continue
				}
				yes, ok := project(branch.IfBody)
				if !ok {
					return nil, false
				}
				no, ok := project(branch.ElseBody)
				if !ok {
					return nil, false
				}
				copy := *branch
				copy.IfBody = yes
				copy.ElseBody = no
				out = append(out, &copy)
			} else {
				mapped, known := nativeAssertionProjectBlocks(st, project)
				if !known {
					return nil, false
				}
				out = append(out, mapped)
			}
			delete(active, st)
		}
		return out, true
	}
	projected, ok := project(body)
	if !ok || len(seen) != len(sites) {
		return nil, fmt.Errorf("source assertion protocol unproved")
	}
	return projected, nil
}
func nativeAssertionConstructorDescriptor(desc string) ([]string, string, bool) {
	switch desc {
	case "()V":
		return nil, "V", false
	case "(Ljava/lang/Object;)V":
		return []string{"Ljava/lang/Object;"}, "V", false
	case "(Z)V", "(C)V", "(I)V", "(J)V", "(F)V", "(D)V":
		return []string{desc[1:2]}, "V", false
	}
	return nil, "", true
}

func nativeAssertionErrorType(t types.JavaType) bool {
	if t == nil {
		return false
	}
	c, ok := t.RawType().(*types.JavaClass)
	return ok && c != nil && c.Name == "java.lang.AssertionError"
}

// The original disabled edge bypasses NEW, message evaluation, invokespecial
// and ATHROW. Match those PCs and the constructor descriptor, not printed text.
type nativeAssertionPacket struct {
	newPC, invokePC, throwPC, endPC int
	descriptor                      string
}

func nativeAssertionBytecodePacket(obj *ClassObject, ops []*core.OpCode, start int, work *workbudget.Budget) *nativeAssertionPacket {
	// The disabled/success continuation need not physically follow ATHROW:
	// loop assertions branch backward, and switch arms may jump over another
	// assertion. Bound the physical failure packet by its first terminal throw,
	// then independently certify every original exit and incoming edge.
	end := -1
	for i := start + 2; i < len(ops); i++ {
		if !nativeProofWork(work, 1) || ops[i] == nil || ops[i].Instr == nil {
			return nil
		}
		if ops[i].Instr.OpCode == core.OP_ATHROW {
			end = i + 1
			break
		}
	}
	if end < start+6 || ops[end-1].Instr.OpCode != core.OP_ATHROW {
		return nil
	}
	invoke := constructorMotionMember(obj, ops[end-2], core.OP_INVOKESPECIAL)
	if invoke == nil || invoke.Name != "java/lang/AssertionError" || invoke.Member != "<init>" {
		return nil
	}
	_, _, bad := nativeAssertionConstructorDescriptor(invoke.Description)
	if bad {
		return nil
	}
	found := -1
	for i := start + 2; i < end-2; i++ {
		op := ops[i]
		if op.Instr.OpCode != core.OP_NEW {
			continue
		}
		if len(op.Data) != 2 {
			return nil
		}
		name, ok := sourceBridgeClassName(obj, binary.BigEndian.Uint16(op.Data))
		if !ok {
			return nil
		}
		if name == "java/lang/AssertionError" {
			if found >= 0 || i+1 >= end-2 || ops[i+1].Instr.OpCode != core.OP_DUP {
				return nil
			}
			found = i
		}
	}
	if found < 0 {
		return nil
	}
	return &nativeAssertionPacket{newPC: int(ops[found].CurrentOffset), invokePC: int(ops[end-2].CurrentOffset), throwPC: int(ops[end-1].CurrentOffset), descriptor: invoke.Description, endPC: int(ops[end-1].CurrentOffset) + 1}
}

// Enumerate every source edge, rather than memoizing shared values: duplicated
// reads and omitted reads both refuse. Opaque nodes and cycles cannot prove
// that replacing the complete compiler flag protocol is safe.
func nativeAssertionSourceReads(body []statements.Statement, owner string, sites map[int]*nativeAssertionPacket, work *workbudget.Budget) bool {
	remaining := 8192
	seen := map[int]bool{}
	activeV := map[values.JavaValue]bool{}
	activeS := map[statements.Statement]bool{}
	step := func() bool { remaining--; return remaining >= 0 && nativeProofWork(work, 1) }
	var value func(values.JavaValue) bool
	value = func(v values.JavaValue) bool {
		if sourceProofNil(v) {
			return true
		}
		if !step() || activeV[v] {
			return false
		}
		activeV[v] = true
		defer delete(activeV, v)
		if field, ok := v.(*values.JavaClassMember); ok && field.Name == types.SlashToDot(owner) && field.Member == nativeAssertionField {
			if !field.HasOriginPC || field.RefKind != 0 || field.Description != "Z" || sites[field.OriginPC] == nil || seen[field.OriginPC] {
				return false
			}
			seen[field.OriginPC] = true
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, child := range children {
			if !value(child) {
				return false
			}
		}
		return true
	}
	var walk func([]statements.Statement) bool
	walk = func(body []statements.Statement) bool {
		for _, st := range body {
			if !step() || sourceProofNil(st) || activeS[st] {
				return false
			}
			activeS[st] = true
			// This proves source read identity/absence, not effect motion.
			// Sealed operand-free transfers cannot hide a flag read.
			roots, children, known := nativeSourceNameChildren(st)
			if !known {
				return false
			}
			for _, v := range roots {
				if !value(v) {
					return false
				}
			}
			for _, child := range children {
				if !walk(child) {
					return false
				}
			}
			delete(activeS, st)
		}
		return true
	}
	return walk(body) && len(seen) == len(sites)
}

// A partial handler or an edge entering the guarded packet would give some
// effects a different exception/flag domain after source reconstruction.
func nativeAssertionRegionClosed(d *core.Decompiler, code *CodeAttribute, ops []*core.OpCode, start, end int, work *workbudget.Budget) bool {
	if len(ops) > 8192 {
		return false
	}
	if _, valid := nativeMemberLexicalControlEntries(d, code, work); !valid {
		return false
	}
	for _, h := range code.ExceptionTable {
		if h == nil {
			return false
		}
		a, b, handler := int(h.StartPc), int(h.EndPc), int(h.HandlerPc)
		if handler > start && handler < end || a < end && b > start && !(a <= start && b >= end) {
			return false
		}
	}
	for _, op := range ops {
		if !nativeProofWork(work, 1) {
			return false
		}
		pc := int(op.CurrentOffset)
		if op.Instr.OpCode == core.OP_JSR || op.Instr.OpCode == core.OP_JSR_W || op.Instr.OpCode == core.OP_RET {
			return false
		}
		if pc >= start && pc < end {
			continue
		}
		enters := func(target int) bool { return target > start && target < end }
		switch op.Instr.OpCode {
		case core.OP_GOTO, core.OP_GOTO_W, core.OP_IFEQ, core.OP_IFNE, core.OP_IFLT, core.OP_IFGE, core.OP_IFGT, core.OP_IFLE, core.OP_IF_ICMPEQ, core.OP_IF_ICMPNE, core.OP_IF_ICMPLT, core.OP_IF_ICMPGE, core.OP_IF_ICMPGT, core.OP_IF_ICMPLE, core.OP_IF_ACMPEQ, core.OP_IF_ACMPNE, core.OP_IFNULL, core.OP_IFNONNULL:
			width := 2
			if op.Instr.OpCode == core.OP_GOTO_W {
				width = 4
			}
			target, err := core.BranchTarget(pc, op.Data, width, len(code.Code))
			if err != nil || enters(target) {
				return false
			}
		case core.OP_TABLESWITCH, core.OP_LOOKUPSWITCH:
			if enters(int(op.SwitchDefaultOffset)) {
				return false
			}
			ok := true
			op.SwitchJmpCase.ForEach(func(_ int, target int32) bool {
				if !nativeProofWork(work, 1) || enters(int(target)) {
					ok = false
					return false
				}
				return true
			})
			if !ok {
				return false
			}
		}
	}
	return true
}

// Factor only the first evaluated operand of ordered short-circuit AND.
// Removing a flag from OR, an eager connective, or a later operand changes
// condition evaluation when assertions are disabled and is never permitted.
func nativeAssertionFailureGuard(v values.JavaValue, owner string, work *workbudget.Budget, depth int) (*values.JavaClassMember, values.JavaValue, bool) {
	if depth <= 0 || !nativeProofWork(work, 1) {
		return nil, nil, false
	}
	raw, ok := nativeMemberEnclosingUnpack(v, work)
	if !ok {
		return nil, nil, false
	}
	exp, ok := raw.(*values.JavaExpression)
	if !ok {
		return nil, nil, false
	}
	if exp.Op == values.Not && len(exp.Values) == 1 {
		raw, ok := nativeMemberEnclosingUnpack(exp.Values[0], work)
		f, known := raw.(*values.JavaClassMember)
		if ok && known && f != nil && f.Name == owner && f.Member == nativeAssertionField && f.Description == "Z" && f.HasOriginPC && f.RefKind == 0 {
			return f, values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), true
		}
	}
	if exp.Op != values.LOGICAL_AND || len(exp.Values) != 2 {
		return nil, nil, false
	}
	field, rest, ok := nativeAssertionFailureGuard(exp.Values[0], owner, work, depth-1)
	if !ok {
		return nil, nil, false
	}
	return field, values.NewBinaryExpression(rest, exp.Values[1], values.LOGICAL_AND, types.NewJavaPrimer(types.JavaBoolean)), true
}

func (c *ClassObjectDumper) nativeAssertionProtocol() *nativeMemberAssertion {
	if c.nativeEnumConstantCurrent != nil {
		return c.nativeEnumConstantCurrent.assertions
	}
	if c.nativeMemberCurrent != nil {
		return c.nativeMemberCurrent.assertions
	}
	return c.nativeSourceAssertions
}
