package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A sealed factory describes its own eager operands, not the operand of a
// later invocation. Reopen the original verifier/SSA snapshot to bind both
// ports: this exact initialized parameter feeds the factory and its one result
// feeds the selected argument of the original THIS/SUPER call. POP, replacement values,
// phis, additional consumers and detached declarations cannot borrow that
// factory's PC or a source value that happens to retain its old annotation.
func (c *ClassObjectDumper) constructorReferenceFactoryPort(method *MemberInfo, code *CodeAttribute, factoryPC int, delegate *values.FunctionCallExpression, receiverSlot int, argumentPort ...int) bool {
	port := 0
	if len(argumentPort) > 1 || len(argumentPort) == 1 && argumentPort[0] < 0 {
		return false
	}
	if len(argumentPort) == 1 {
		port = argumentPort[0]
	}
	if c == nil || c.obj == nil || method == nil || code == nil || delegate == nil || factoryPC < 0 || factoryPC > 65535 || delegate.OriginPC < 0 || delegate.OriginPC > 65535 || len(c.obj.Methods) > 4096 {
		return false
	}
	declarations, codes := 0, 0
	for _, original := range c.obj.Methods {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if original == method {
			declarations++
		}
	}
	for _, a := range method.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if candidate, ok := a.(*CodeAttribute); ok {
			if candidate != code {
				return false
			}
			codes++
		}
	}
	if declarations != 1 || codes != 1 {
		return false
	}
	ir, frames, known := c.nativeOriginalMethodSnapshot(method, code)
	if !known {
		return false
	}
	origin := ssabuild.Origin{Kind: ssabuild.OriginInstr, PC: uint16(factoryPC)}
	factory, invocation, uses := false, false, 0
	for _, record := range frames.Instructions {
		if !nativeProofWork(c.Work, int64(len(record.Uses)+1)) {
			return false
		}
		for _, operand := range record.Uses {
			if operand == origin {
				uses++
			}
		}
		if int(record.PC) == factoryPC {
			ins, found := ir.InstrByID(methodir.InstrID(record.PC))
			if factory || !found || ins.Opcode != core.OP_INVOKEDYNAMIC || len(record.Uses) != 1 || record.Uses[0] != (ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: receiverSlot}) || len(record.Results) != 1 || record.Results[0] != origin {
				return false
			}
			factory = true
		}
		if int(record.PC) == delegate.OriginPC {
			ins, found := ir.InstrByID(methodir.InstrID(record.PC))
			parameters, result, err := callbinding.Descriptor(delegate.Descriptor)
			if invocation || !found || ins.Opcode != core.OP_INVOKESPECIAL || ins.Class != strings.ReplaceAll(delegate.ClassName, ".", "/") || ins.Member != "<init>" || ins.Desc != delegate.Descriptor || err != nil || result != "V" || port >= len(parameters) || len(record.Uses) != len(parameters)+1 || record.Uses[0] != (ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: 0}) || record.Uses[port+1] != origin || !record.Before.ThisUninitialized || len(record.Before.Stack) != nativeMemberParameterWidth(parameters)+1 || record.Before.Stack[0].Kind != frametransfer.UninitThis {
				return false
			}
			invocation = true
		}
	}
	return factory && invocation && uses == 1
}
