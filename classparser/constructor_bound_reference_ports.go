package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Prove a whole ordered constructor argument list made of unchanged parameter
// reads and bound references. Every receiver check must be the discarded check
// immediately paired with its own factory. Every factory must feed its exact
// THIS/SUPER argument once. No effect is moved past a different argument: the
// source list has precisely the original packet order, including repeated or
// distinct receivers. More general argument computations retain the separate
// single-leading-reference proof or ordinary constructor-prefix handling.
func (c *ClassObjectDumper) constructorBoundReferencePortListSource(code *CodeAttribute, body []statements.Statement, params []values.JavaValue, method *MemberInfo) ([]statements.Statement, bool) {
	fail := func() ([]statements.Statement, bool) { return body, false }
	if c == nil || c.obj == nil || c.FuncCtx == nil || method == nil || code == nil || c.isGenuineEnum() || c.options.SourceCompiler != NativeJavac8 && c.options.SourceCompiler != ModernJavac {
		return fail()
	}
	name, nk := sourceBridgeUTF8(c.obj, method.NameIndex)
	descriptor, dk := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	physical, _, err := callbinding.Descriptor(descriptor)
	if !nk || !dk || name != "<init>" || method.AccessFlags&8 != 0 || err != nil || len(params) != len(physical)+1 {
		return fail()
	}
	this, known := params[0].(*values.JavaRef)
	if !known || this == nil {
		return fail()
	}
	if _, known := this.OriginalReceiverSlot(); !known {
		return fail()
	}
	var checks []*values.FunctionCallExpression
	var indices []int
	var delegate *values.FunctionCallExpression
	var statement *statements.ExpressionStatement
	delegateIndex := -1
	for i, st := range body {
		if !nativeProofWork(c.Work, 1) || len(checks) > 128 {
			return fail()
		}
		if middle, ok := st.(*statements.MiddleStatement); ok && middle != nil && middle.Data == nil {
			continue
		}
		expression, ok := st.(*statements.ExpressionStatement)
		if !ok || expression == nil {
			return fail()
		}
		call, ok := values.UnpackSoltValue(expression.Expression).(*values.FunctionCallExpression)
		if !ok || call == nil {
			return fail()
		}
		if call.FunctionName == "<init>" {
			delegate, statement, delegateIndex = call, expression, i
			break
		}
		checks, indices = append(checks, call), append(indices, i)
	}
	if delegate == nil || len(checks) == 0 || delegate.Kind != values.InvokeSpecial || values.UnpackSoltValue(delegate.Object) != this {
		return fail()
	}
	owner := strings.ReplaceAll(delegate.ClassName, ".", "/")
	if owner != c.obj.GetClassName() && owner != c.obj.GetSupperClassName() {
		return fail()
	}
	decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(c.ConstantPool, index) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil || !constructorPreludeOutsideHandlers(code, delegate.OriginPC, decoder, c.Work) || !constructorOriginalInvoke(decoder, delegate, func(index int) values.JavaValue { return GetValueFromCP(c.ConstantPool, index) }) {
		return fail()
	}
	ops := constructorMotionOps(decoder)
	if len(ops) < 3 || core.GetRetrieveIdx(ops[0]) != 0 || !constructorMotionLoad(ops[0], "Ljava/lang/Object;") {
		return fail()
	}
	slots := constructorParameterSlots(physical)
	parameter := func(value values.JavaValue) (*values.JavaRef, int, string, bool) {
		ref, ok := values.UnpackSoltValue(value).(*values.JavaRef)
		if !ok || ref == nil || ref.Id == nil {
			return nil, 0, "", false
		}
		slot, known := ref.OriginalParameterSlot()
		index, found := slots[slot]
		if !known || !found || params[index+1] != ref {
			return nil, 0, "", false
		}
		erased, known := values.SourceTypeErasure(ref.Type(), c.FuncCtx)
		return ref, slot, physical[index], known && erased == physical[index]
	}
	arguments := append([]values.JavaValue(nil), delegate.Arguments...)
	at, checkIndex := 1, 0
	for port, argument := range delegate.Arguments {
		if !nativeProofWork(c.Work, 1) || at >= len(ops) {
			return fail()
		}
		factory, reference := values.UnpackSoltValue(argument).(*values.CustomValue)
		if !reference || factory == nil || !factory.IsMethodRef {
			_, slot, desc, known := parameter(argument)
			if !known || core.GetRetrieveIdx(ops[at]) != slot || !constructorMotionLoad(ops[at], desc) {
				return fail()
			}
			at++
			continue
		}
		pc, captures, known := factory.OriginalLambdaFactoryEvaluation()
		if !known || len(captures) != 1 || checkIndex >= len(checks) || at+4 >= len(ops) {
			return fail()
		}
		receiver, slot, desc, known := parameter(captures[0])
		check := checks[checkIndex]
		if !known || !strings.HasPrefix(desc, "L") || core.GetRetrieveIdx(ops[at]) != slot || !constructorMotionLoad(ops[at], desc) || ops[at+1].Instr.OpCode != core.OP_DUP || !nativeMemberNullCheck(c.obj, ops[at+2]) || ops[at+3].Instr.OpCode != core.OP_POP || ops[at+4].Instr.OpCode != core.OP_INVOKEDYNAMIC || int(ops[at+2].CurrentOffset) != check.OriginPC || int(ops[at+4].CurrentOffset) != pc {
			return fail()
		}
		if !constructorOriginalInvoke(decoder, check, func(index int) values.JavaValue { return GetValueFromCP(c.ConstantPool, index) }) || !c.constructorReferenceFactoryPort(method, code, pc, delegate, slot, port) {
			return fail()
		}
		if ops[at+2].Instr.OpCode == core.OP_INVOKEVIRTUAL {
			if check.Kind != values.InvokeVirtual || len(check.Arguments) != 0 || values.UnpackSoltValue(check.Object) != receiver {
				return fail()
			}
		} else if check.Kind != values.InvokeStatic || len(check.Arguments) != 1 || values.UnpackSoltValue(check.Arguments[0]) != receiver {
			return fail()
		}
		target, member, instantiated, proved := c.constructorBoundReferenceTarget(code, ops[at+4], owner, delegate.Descriptor, desc, port)
		if !proved {
			return fail()
		}
		arguments[port] = c.constructorBoundReferenceValue(receiver, target, member, instantiated, pc, nil)
		at += 5
		checkIndex++
	}
	if checkIndex != len(checks) || at >= len(ops) || int(ops[at].CurrentOffset) != delegate.OriginPC || ops[at].Instr.OpCode != core.OP_INVOKESPECIAL {
		return fail()
	}
	copyCall, copyStatement := *delegate, *statement
	copyCall.Arguments = arguments
	copyStatement.Expression = &copyCall
	removed := map[int]bool{}
	for _, index := range indices {
		removed[index] = true
	}
	result := make([]statements.Statement, 0, len(body)-len(indices))
	for i, st := range body {
		if i == delegateIndex {
			result = append(result, &copyStatement)
		} else if !removed[i] {
			result = append(result, st)
		}
	}
	return result, true
}
