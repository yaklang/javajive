package javaclassparser

import (
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// Source regenerates a fresh array of constants in declaration order. A clone
// factory proves only copying, not the order or identity of the array copied.
// Certify its unique original producer and every use of its erased field. The
// helper and inline compiler forms have the same array-packet contract; user
// initialization outside that packet is never moved or silently suppressed.
func (c *ClassObjectDumper) nativeEnumBackingArrayProtocol(initializations map[string]nativeEnumConstantAllocation) error {
	fail := func(reason string) error {
		if c.Work != nil && c.Work.Err() != nil {
			return c.Work.Err()
		}
		return fmt.Errorf("enum regeneration: original backing array is unproved: %s", reason)
	}
	obj := c.obj
	owner := obj.GetClassName()
	descriptor := "[L" + owner + ";"
	var factory, initializer *MemberInfo
	var backing *MemberInfo
	for _, m := range obj.Methods {
		if m == nil || !nativeProofWork(c.Work, 1) {
			return fail("method table")
		}
		name, nk := sourceBridgeUTF8(obj, m.NameIndex)
		desc, dk := sourceBridgeUTF8(obj, m.DescriptorIndex)
		if !nk || !dk {
			return fail("method identity")
		}
		if name == "values" && desc == "()"+descriptor {
			if factory != nil {
				return fail("duplicate factory")
			}
			factory = m
			backing = nativeEnumValuesFactoryBackingWithParameterNames(obj, m, c.Work, true)
		}
		if name == "<clinit>" {
			if initializer != nil {
				return fail("duplicate initializer")
			}
			initializer = m
		}
	}
	if factory == nil || backing == nil || initializer == nil {
		return fail("factory or initializer")
	}
	backingName, nk := sourceBridgeUTF8(obj, backing.NameIndex)
	if !nk || backingName != "$VALUES" {
		return fail("compiler backing declaration")
	}
	constants := []string{}
	for _, f := range obj.Fields {
		if f == nil || !nativeProofWork(c.Work, 1) {
			return fail("constant table")
		}
		if f.AccessFlags&0x4000 == 0 {
			continue
		}
		name, nk := sourceBridgeUTF8(obj, f.NameIndex)
		desc, dk := sourceBridgeUTF8(obj, f.DescriptorIndex)
		if !nk || !dk || f.AccessFlags != 0x4019 || desc != "L"+owner+";" {
			return fail("constant declaration")
		}
		constants = append(constants, name)
	}
	var initCode *CodeAttribute
	var initOps []*core.OpCode
	storeIndex := -1
	readCount := 0
	for _, method := range obj.Methods {
		for _, a := range method.Attributes {
			code, ok := a.(*CodeAttribute)
			if !ok {
				continue
			}
			if code == nil || !nativeProofWork(c.Work, int64(len(code.Code))) {
				return fail("original code budget")
			}
			d := core.NewDecompiler(code.Code, nil)
			d.Work = c.Work
			if d.ParseOpcode() != nil {
				return fail("original code")
			}
			ops := constructorMotionOps(d)
			if method == initializer {
				if initOps != nil {
					return fail("duplicate initializer code")
				}
				initOps, initCode = ops, code
			}
			for i, op := range ops {
				if !nativeEnumOpcode(op, core.OP_GETSTATIC) && !nativeEnumOpcode(op, core.OP_PUTSTATIC) {
					continue
				}
				operand := constructorMotionMember(obj, op, op.Instr.OpCode)
				if operand == nil || operand.Name != owner || operand.Member != backingName {
					continue
				}
				if operand.Description != descriptor || !nativeEnumMemberOperand(obj, op, op.Instr.OpCode, owner, backingName, descriptor) {
					return fail("backing field identity")
				}
				if nativeEnumOpcode(op, core.OP_GETSTATIC) {
					if method != factory || i != 0 || readCount != 0 {
						return fail("source-erased backing field escapes its generated factory")
					}
					readCount++
				} else {
					if method != initializer || storeIndex != -1 {
						return fail("multiple or non-initializer backing writes")
					}
					storeIndex = i
				}
			}
		}
	}
	if initOps == nil || storeIndex < 1 || readCount != 1 {
		return fail("missing unique producer")
	}
	start := storeIndex - 1
	previous := initOps[start]
	if nativeEnumOpcode(previous, core.OP_INVOKESTATIC) {
		helperName, known := nativeEnumArrayHelperName(obj, c.Work)
		if !known || !nativeEnumMemberOperand(obj, previous, core.OP_INVOKESTATIC, owner, helperName, "()"+descriptor) {
			return fail("array helper identity")
		}
		var helper *MemberInfo
		for _, method := range obj.Methods {
			name, nk := sourceBridgeUTF8(obj, method.NameIndex)
			desc, dk := sourceBridgeUTF8(obj, method.DescriptorIndex)
			if nk && dk && name == helperName && desc == "()"+descriptor {
				if helper != nil {
					return fail("duplicate array helper")
				}
				helper = method
			}
		}
		if helper == nil {
			return fail("missing array helper")
		}
		regenerated, err := nativeEnumRegeneratedMethod(obj, helper, helperName, "()"+descriptor, c.Work)
		if err != nil {
			return err
		}
		if !regenerated {
			return fail("non-generated array helper")
		}
	} else {
		start = storeIndex - (2 + 4*len(constants))
		if start < 0 || !nativeEnumValuesArrayPacket(obj, initOps[start:storeIndex], constants, c.Work) {
			return fail("inline array order and constant identities")
		}
	}
	// Immutable CFG edges include switch targets and exception edges. Mutable
	// decoder Target lists do not exist after ParseOpcode and are not evidence.
	ir, _, known := c.nativeOriginalMethodSnapshot(initializer, initCode)
	if !known {
		return fail("initializer control flow")
	}
	first, last := int(initOps[start].CurrentOffset), int(initOps[storeIndex].CurrentOffset)
	for _, edge := range ir.Edges {
		if !nativeProofWork(c.Work, 1) {
			return fail("control-flow budget")
		}
		from, to := int(edge.From), int(edge.To)
		if edge.Kind == core.EdgeException && from >= first && from <= last {
			return fail("handler protects source-generated initialization")
		}
		if to > first && to <= last && (from < first || from > last) || to == first && from >= first && from <= last {
			return fail("control flow enters or repeats erased array packet")
		}
	}
	var preceding []uint16
	if len(constants) != len(initializations) {
		return fail("constant initialization coverage")
	}
	if len(constants) > 0 {
		constant := constants[len(constants)-1]
		plan, known := initializations[constant]
		if !known || plan.constant != constant || plan.ordinal != len(constants)-1 {
			return fail("preceding constant assignment")
		}
		preceding = []uint16{uint16(plan.storePC)}
	}
	if !nativeEnumRegionExecutedOnce(ir, uint16(first), uint16(last), c.Work, preceding...) {
		return fail("backing producer is bypassed, unreachable or repeated")
	}

	return nil
}
