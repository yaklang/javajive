package javaclassparser

import (
	"crypto/sha256"
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

type nativeEnumBackingSource struct {
	object      *ClassObject
	initializer *MemberInfo
	code        *CodeAttribute
	codeHash    [32]byte
	packet      core.RegeneratedStaticArray
}

// nativeEnumReadOnlyArrayIteration certifies that a non-factory read of the
// compiler's backing array cannot observe the difference between the shared
// array and the clone factory's copy: the reference must be stored into one
// local immediately, and every later load of that local must be consumed
// directly by arraylength or aaload. A store, a call, a field write or a return
// of the reference needs a different operand position and is refused.
func nativeEnumRefLocal(op *core.OpCode, write bool) (int, bool) {
	if op == nil || op.Instr == nil {
		return 0, false
	}
	code := op.Instr.OpCode
	switch code {
	case core.OP_ASTORE, core.OP_ASTORE_0, core.OP_ASTORE_1, core.OP_ASTORE_2, core.OP_ASTORE_3:
		if !write {
			return 0, false
		}
	case core.OP_ALOAD, core.OP_ALOAD_0, core.OP_ALOAD_1, core.OP_ALOAD_2, core.OP_ALOAD_3:
		if write {
			return 0, false
		}
	default:
		return 0, false
	}
	slot := core.GetRetrieveIdx(op)
	if write {
		slot = core.GetStoreIdx(op)
	}
	return slot, slot >= 0
}

func nativeEnumReadOnlyArrayIteration(ops []*core.OpCode, readIndex int) bool {
	if readIndex < 0 || readIndex+1 >= len(ops) {
		return false
	}
	// javac binds the array with either astore or astore_<n>. A wide store is
	// the same slot contract; anything else leaves the reference on the stack.
	local, stored := nativeEnumRefLocal(ops[readIndex+1], true)
	if !stored || readIndex+3 >= len(ops) {
		return false
	}
	// Check null before any observable work. Calling values() on a not-yet-
	// initialized backing array throws at clone(), whereas a bare getstatic
	// only yields null. Requiring the original immediate arraylength preserves
	// exception priority in reentrant initialization as well as ordinary calls.
	firstLoad, loaded := nativeEnumRefLocal(ops[readIndex+2], false)
	if !loaded || firstLoad != local || !nativeEnumOpcode(ops[readIndex+3], core.OP_ARRAYLENGTH) {
		return false
	}
	loads := 0
	for i, op := range ops {
		slot, loaded := nativeEnumRefLocal(op, false)
		if !loaded || slot != local {
			continue
		}
		// arraylength consumes the reference directly. aaload consumes it under
		// one int index. javac's for-each is `aload; iload; aaload`, so the index
		// may sit between the load and aaload. Anything that could pop, copy or
		// pass the reference (a call, a store, a return, another reference) is
		// not an int producer and is refused.
		if !nativeEnumArrayLoadIsLocal(ops, i) {
			return false
		}
		loads++
	}
	// A dead store is not an iteration. Require a proved consumer so values()
	// replaces a real read, not an unused reference.
	return loads > 0
}

// nativeEnumArrayLoadIsLocal reports whether ops[loadIndex], an aload of the
// backing array, is consumed by arraylength or by aaload under a straight-line
// int index. The array reference stays at stack depth 1 until that consumer.
func nativeEnumArrayLoadIsLocal(ops []*core.OpCode, loadIndex int) bool {
	depth := 1
	for j := loadIndex + 1; j < len(ops) && j <= loadIndex+8; j++ {
		op := ops[j]
		if nativeEnumOpcode(op, core.OP_ARRAYLENGTH) && depth == 1 {
			return true
		}
		if nativeEnumOpcode(op, core.OP_AALOAD) && depth == 2 {
			return true
		}
		delta, inputs, ok := nativeEnumIntProducer(op)
		if !ok || depth-1 < inputs {
			return false
		}
		depth += delta
		if depth < 1 || depth > 4 {
			return false
		}
	}
	return false
}

func nativeEnumIntProducer(op *core.OpCode) (delta, inputs int, valid bool) {
	if op == nil || op.Instr == nil {
		return 0, 0, false
	}
	switch op.Instr.OpCode {
	case core.OP_ICONST_M1, core.OP_ICONST_0, core.OP_ICONST_1, core.OP_ICONST_2, core.OP_ICONST_3, core.OP_ICONST_4, core.OP_ICONST_5,
		core.OP_BIPUSH, core.OP_SIPUSH,
		core.OP_ILOAD, core.OP_ILOAD_0, core.OP_ILOAD_1, core.OP_ILOAD_2, core.OP_ILOAD_3:
		return 1, 0, true
	case core.OP_INEG:
		return 0, 1, true
	case core.OP_IADD, core.OP_ISUB, core.OP_IMUL, core.OP_IDIV, core.OP_IREM,
		core.OP_IAND, core.OP_IOR, core.OP_IXOR, core.OP_ISHL, core.OP_ISHR, core.OP_IUSHR:
		return -1, 2, true
	default:
		return 0, 0, false
	}
}

// Source regenerates a fresh array of constants in declaration order. A clone
// factory proves only copying, not the order or identity of the array copied.
// Certify its unique original producer and every use of its erased field. The
// helper and inline compiler forms have the same array-packet contract; user
// initialization outside that packet is never moved or silently suppressed.
func (c *ClassObjectDumper) nativeEnumBackingArrayProtocol(initializations map[string]nativeEnumConstantAllocation) error {
	c.nativeEnumBacking = nil
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
	// values() already selected the unique private static final synthetic
	// self-array (flags 0x101a, no attributes). Javac spells that field
	// $VALUES, but an obfuscator can rename it. The later packet proof, not
	// the spelling, certifies the constant order. Source regenerates $VALUES.
	if !nk || backingName == "" {
		return fail("compiler backing declaration")
	}
	c.enumSyntheticBackingField = backingName
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
					if method == factory {
						if i != 0 || readCount != 0 {
							return fail("source-erased backing field escapes its generated factory")
						}
						readCount++
						continue
					}
					// A hand-written method may iterate the shared array instead of
					// calling the clone factory. That is expressible through the
					// factory only when the original cannot observe the difference:
					// the reference is bound to one local and every use of that local
					// is arraylength or aaload, so no store, call, return or field
					// write can see the shared array.
					if !nativeEnumReadOnlyArrayIteration(ops, i) {
						return fail("source-erased backing field escapes its generated factory")
					}
					continue
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
	arrayHelper := ""
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
		arrayHelper = helperName
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
	// Java emits its backing immediately after the constants (and any proved
	// compiler assertion prefix). Dominance alone would also accept a later
	// packet, after user code which can call values() before the array exists.
	before := -1
	if len(constants) != 0 {
		for i, op := range initOps {
			if int(op.CurrentOffset) == initializations[constants[len(constants)-1]].storePC {
				before = i
				break
			}
		}
	} else if assertion := c.nativeAssertionProtocol(); assertion != nil && assertion.initializer == initializer {
		for i, op := range initOps {
			if int(op.CurrentOffset) < assertion.initializerEndPC {
				before = i
			}
		}
	}
	if start != before+1 {
		return fail("backing producer follows non-regenerated initializer effects")
	}
	allocation := first
	if arrayHelper == "" {
		allocation = int(initOps[start+1].CurrentOffset)
	}
	c.nativeEnumBacking = &nativeEnumBackingSource{object: obj, initializer: initializer, code: initCode, codeHash: sha256.Sum256(initCode.Code), packet: core.RegeneratedStaticArray{Owner: owner, Field: backingName, Descriptor: descriptor, Helper: arrayHelper, AllocationPC: allocation, StorePC: last, Constants: constants}}

	return nil
}

func (c *ClassObjectDumper) projectNativeEnumBacking(parser *core.Decompiler, code *CodeAttribute, method *MemberInfo, root *[]statements.Statement) error {
	plan := c.nativeEnumBacking
	if plan == nil || method != plan.initializer {
		return nil
	}
	if plan.object != c.obj || plan.code != code || sha256.Sum256(code.Code) != plan.codeHash || !c.isGenuineEnum() || !parser.ProjectRegeneratedStaticArray(root, plan.packet) {
		if c.Work != nil && c.Work.Err() != nil {
			return c.Work.Err()
		}
		return fmt.Errorf("enum regeneration: original backing source packet is not closed")
	}
	return nil
}
