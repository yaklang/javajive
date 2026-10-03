package javaclassparser

import (
	"encoding/binary"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Values are logical JVM values, including category-2 width. Only the newly
// allocated receiver can carry receiver=true: publishing it into any other
// object, static field or opaque call is rejected. Therefore a receiver-free
// value loaded from a parameter/field/call cannot acquire an alias to it.
type constructorEffectValue struct {
	kind       byte
	receiver   bool
	allocation int // Original NEW PC + 1, until its exact <init> completes.
}

// Stores of THIS into THIS's own nonvolatile storage do not publish it. Until
// a field-sensitive heap proof exists, reject their combination with any
// reference-valued read from THIS anywhere in the constructor chain. Facts are
// monotone across all branches and recursive delegations: a different branch
// or parent cannot hide a potentially receiver-valued alias behind a plain L.
type constructorSelfStorageProof struct {
	selfStored    bool
	referenceRead bool
}

func (p *constructorSelfStorageProof) closed() bool {
	return p != nil && !(p.selfStored && p.referenceRead)
}

// A source release is not a runtime pin. Where original platform code is
// supplied by the catalog, every catalogued runtime capable of running this
// source must admit the same movement. Actual caller-supplied bytes retain
// precedence. All attempts share the original bounded proof budget.
func (c *ClassObjectDumper) constructorCaptureChainDoesNotObserve(owner, descriptor string, writes map[string]bool) bool {
	target := c.options.TargetSourceVersion
	if target == 0 {
		target = core.ClassMajorToSourceVersion(c.obj.MajorVersion)
	}
	remaining := 512
	prove := func(release int) (bool, bool) {
		d := *c
		d.options.TargetSourceVersion = release
		platformUsed := false
		d.foldSiblingResolver = func(name string) ([]byte, bool) {
			if c.foldSiblingResolver != nil {
				if raw, ok := c.foldSiblingResolver(name); ok {
					return raw, true
				}
			}
			if c.declarationResolver != nil {
				if raw, ok := c.declarationResolver(name); ok {
					return raw, true
				}
			}
			raw, ok := jdkConstructorClassBytes(name, release)
			platformUsed = platformUsed || ok
			return raw, ok
		}
		d.declarationResolver = nil // already consulted before the platform evidence
		d.FuncCtx = &class_context.ClassContext{}
		d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
		d.constructorReceiverFinalizerSilent = d.constructorReceiverCannotObserveFinalization(&remaining)
		safe := d.constructorChainDoesNotObserve(owner, descriptor, writes, map[string]bool{}, &remaining, 0)
		return safe, platformUsed
	}
	if safe, platformUsed := prove(target); !safe {
		return false
	} else if !platformUsed {
		return true
	}
	// Metadata initialization above also initializes the supported release set.
	releases := []int{}
	for release := range jdkInvocationCatalog.profiles {
		if release > target {
			releases = append(releases, release)
		}
	}
	sort.Ints(releases)
	for _, release := range releases {
		if safe, _ := prove(release); !safe {
			return false
		}
	}
	return true
}

func constructorEffectType(desc string) constructorEffectValue {
	if len(desc) == 0 {
		return constructorEffectValue{}
	}
	kind := desc[0]
	if kind == '[' || kind == 'L' {
		kind = 'L'
	} else if kind == 'Z' || kind == 'B' || kind == 'C' || kind == 'S' {
		kind = 'I'
	}
	return constructorEffectValue{kind: kind}
}

func (v constructorEffectValue) width() int {
	if v.kind == 'J' || v.kind == 'D' {
		return 2
	}
	if v.kind == 'I' || v.kind == 'F' || v.kind == 'L' {
		return 1
	}
	return 0
}

func (c *ClassObjectDumper) constructorMotionClass(owner string) (*ClassObject, bool) {
	if c.obj != nil && owner == c.obj.GetClassName() {
		return c.obj, true
	}
	var raw []byte
	var ok bool
	if c.foldSiblingResolver != nil {
		raw, ok = c.foldSiblingResolver(owner)
	}
	if !ok && c.declarationResolver != nil {
		raw, ok = c.declarationResolver(owner)
	}
	if !ok {
		target := c.options.TargetSourceVersion
		if target == 0 {
			target = core.ClassMajorToSourceVersion(c.obj.MajorVersion)
		}
		raw, ok = jdkConstructorClassBytes(owner, target)
	}
	if !ok {
		return nil, false
	}
	obj, err := c.parseResolved(raw)
	return obj, err == nil && obj != nil && obj.GetClassName() == owner
}

// A Fieldref may name a subclass while resolving to an ancestor's field. Bind
// the symbolic owner first, then search its original declaration hierarchy.
// Parent and child fields named this$0 must remain different storage locations.
func (c *ClassObjectDumper) constructorEffectField(obj *ClassObject, member *values.JavaClassMember, put bool, remaining *int) (string, bool) {
	if member == nil {
		return "", false
	}
	caller := obj.GetClassName()
	pkg := func(name string) string {
		if slash := strings.LastIndexByte(name, '/'); slash >= 0 {
			return name[:slash]
		}
		return ""
	}
	symbolic := false
	seen := map[string]bool{}
	for depth := 0; obj != nil && depth <= 16; depth++ {
		*remaining--
		name := obj.GetClassName()
		if *remaining < 0 || seen[name] {
			return "", false
		}
		seen[name] = true
		symbolic = symbolic || name == member.Name
		if symbolic {
			matches := 0
			var flags uint16
			for _, field := range obj.Fields {
				n, _ := obj.getUtf8(field.NameIndex)
				d, _ := obj.getUtf8(field.DescriptorIndex)
				if n == member.Member && d == member.Description {
					matches++
					flags = field.AccessFlags
				}
			}
			if matches != 0 {
				// Resolution must not introduce an IllegalAccessError after
				// initialization. Protected access is through this descendant;
				// private ancestor access needs a separate nestmate proof.
				accessible := name == caller || flags&0x0001 != 0 || flags&0x0004 != 0 || flags&0x0002 == 0 && pkg(name) == pkg(caller)
				writable := !put || flags&0x0010 == 0 || name == caller
				return name + "\x00" + member.Member + "\x00" + member.Description, matches == 1 && flags&(0x0008|0x0040) == 0 && accessible && writable
			}
		}
		var ok bool
		obj, ok = c.constructorMotionClass(obj.GetSupperClassName())
		if !ok {
			return "", false
		}
	}
	return "", false
}

// This effect domain admits receiver-independent scalar computation, local
// aliases, ordinary field access and acyclic conditional control flow. Every
// feasible abstract path must finish initialization and remain receiver-silent.
// Paths are memoized by PC and exact type/receiver state; loops fail closed. A normal
// Java exception after Object initialization can expose the receiver to a
// finalizer, even without an explicit publication. Opaque operations after
// initialization therefore require the separate closed-finalizer proof; all
// receiver publication and moved-storage observation remain forbidden.
func (c *ClassObjectDumper) constructorReceiverEffects(obj *ClassObject, code *CodeAttribute, ops []*core.OpCode, descriptor string, writes, active map[string]bool, remaining *int, depth int) bool {
	aliases := &constructorSelfStorageProof{}
	return c.constructorReceiverEffectsWithStorage(obj, code, ops, descriptor, writes, active, remaining, depth, aliases) && aliases.closed()
}

func (c *ClassObjectDumper) constructorReceiverEffectsWithStorage(obj *ClassObject, code *CodeAttribute, ops []*core.OpCode, descriptor string, writes, active map[string]bool, remaining *int, depth int, aliases *constructorSelfStorageProof) bool {
	params, ret, err := callbinding.Descriptor(descriptor)
	if err != nil || ret != "V" || code.MaxLocals == 0 {
		return false
	}
	locals := make([]constructorEffectValue, int(code.MaxLocals))
	locals[0] = constructorEffectValue{kind: 'L', receiver: true}
	slot := 1
	for _, param := range params {
		value := constructorEffectType(param)
		if value.width() == 0 || slot+value.width() > len(locals) {
			return false
		}
		locals[slot] = value
		slot += value.width()
	}
	offsets := map[int]int{}
	for index, op := range ops {
		if op != nil {
			offsets[int(op.CurrentOffset)] = index
		}
	}
	target := func(op *core.OpCode) (int, bool) {
		delta := 0
		if len(op.Data) == 2 {
			delta = int(int16(binary.BigEndian.Uint16(op.Data)))
		} else if len(op.Data) == 4 && op.Instr.OpCode == core.OP_GOTO_W {
			delta = int(int32(binary.BigEndian.Uint32(op.Data)))
		} else {
			return 0, false
		}
		pc := int(op.CurrentOffset) + delta
		index, ok := offsets[pc]
		return index, ok
	}
	allocations := map[int]string{}
	memo := map[string]bool{}
	var walk func(int, []constructorEffectValue, []constructorEffectValue, bool) bool
	walk = func(start int, locals, stack []constructorEffectValue, initialized bool) (result bool) {
		if start < 0 || start >= len(ops) {
			return false
		}
		key := strconv.Itoa(start) + ":"
		state := []byte(key)
		if initialized {
			state = append(state, 1)
		} else {
			state = append(state, 0)
		}
		for _, part := range [][]constructorEffectValue{locals, stack} {
			for _, v := range part {
				state = append(state, v.kind)
				state = binary.BigEndian.AppendUint32(state, uint32(v.allocation))
				if v.receiver {
					state = append(state, 1)
				} else {
					state = append(state, 0)
				}
			}
			state = append(state, 255)
		}
		key = string(state)
		if prior, known := memo[key]; known {
			return prior
		}
		defer func() { memo[key] = result }()
		pop := func(kind byte) (constructorEffectValue, bool) {
			if len(stack) == 0 {
				return constructorEffectValue{}, false
			}
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			return v, kind == 0 || v.kind == kind
		}
		for index := start; index < len(ops); index++ {
			op := ops[index]
			*remaining--
			if *remaining < 0 {
				return false
			}
			// Check before each instruction, including local-load fast paths and
			// RETURN. A malformed declared stack limit cannot certify movement.
			words := 0
			for _, v := range stack {
				words += v.width()
			}
			if words > int(code.MaxStack) {
				return false
			}
			if op == nil || op.Instr == nil {
				return false
			}
			opcode := op.Instr.OpCode
			access := core.LocalAccessOf(opcode)
			if access.Read || access.Write {
				kind := byte(0)
				if opcode == core.OP_IINC {
					slot := core.GetRetrieveIdx(op)
					if slot < 0 || slot >= len(locals) || locals[slot].kind != 'I' {
						return false
					}
					continue
				}
				for category, k := range []byte{'I', 'J', 'F', 'D', 'L'} {
					if opcode == core.OP_ILOAD+category || opcode == core.OP_ISTORE+category || opcode >= core.OP_ILOAD_0+category*4 && opcode < core.OP_ILOAD_0+category*4+4 || opcode >= core.OP_ISTORE_0+category*4 && opcode < core.OP_ISTORE_0+category*4+4 {
						kind = k
					}
				}
				if kind == 0 {
					return false
				}
				if access.Read {
					slot := core.GetRetrieveIdx(op)
					if slot < 0 || slot+access.Width > len(locals) || locals[slot].kind != kind {
						return false
					}
					stack = append(stack, locals[slot])
				} else {
					v, ok := pop(kind)
					slot := core.GetStoreIdx(op)
					if !ok || slot < 0 || slot+v.width() > len(locals) {
						return false
					}
					// Overwriting either half invalidates the previous wide value.
					if slot > 0 && locals[slot-1].width() == 2 {
						locals[slot-1] = constructorEffectValue{}
					}
					if locals[slot].width() == 2 {
						locals[slot+1] = constructorEffectValue{}
					}
					locals[slot] = v
					if v.width() == 2 {
						locals[slot+1] = constructorEffectValue{}
					}
				}
				continue
			}
			switch {
			case opcode == core.OP_NOP:
				continue
			case opcode == core.OP_GETSTATIC:
				// Class initialization/linkage may fail, but no published THIS
				// can reside in a static field. Preserve the original operation;
				// after initialization, failure requires a closed finalizer.
				if initialized && !c.constructorReceiverFinalizerSilent {
					return false
				}
				member := constructorMotionMember(obj, op, opcode)
				if member == nil {
					return false
				}
				fields, _, err := callbinding.Descriptor("(" + member.Description + ")V")
				if err != nil || len(fields) != 1 {
					return false
				}
				stack = append(stack, constructorEffectType(fields[0]))
			case opcode == core.OP_CHECKCAST || opcode == core.OP_INSTANCEOF:
				if len(op.Data) != 2 || initialized && !c.constructorReceiverFinalizerSilent {
					return false
				}
				if _, known := sourceBridgeClassName(obj, core.Convert2bytesToInt(op.Data)); !known {
					return false
				}
				v, ok := pop('L')
				if !ok || v.receiver || v.allocation != 0 {
					return false
				}
				if opcode == core.OP_INSTANCEOF {
					v = constructorEffectValue{kind: 'I'}
				}
				stack = append(stack, v)
			case opcode == core.OP_NEW:
				// A distinct allocation is not THIS. Preserve its identity through
				// DUP/local aliases until its original constructor initializes all
				// those aliases. Failure still needs the caller's finalizer proof.
				if len(op.Data) != 2 || initialized && !c.constructorReceiverFinalizerSilent {
					return false
				}
				owner, known := sourceBridgeClassName(obj, core.Convert2bytesToInt(op.Data))
				if !known || strings.HasPrefix(owner, "[") || owner == "" {
					return false
				}
				id := int(op.CurrentOffset) + 1
				allocations[id] = owner
				stack = append(stack, constructorEffectValue{kind: 'L', allocation: id})
			case opcode == core.OP_NEWARRAY || opcode == core.OP_ANEWARRAY:
				// Allocation and array-component linkage can fail, including
				// OOME. This is the same failure boundary as a receiver-free call;
				// the allocation never receives THIS or changes effect order.
				if initialized && !c.constructorReceiverFinalizerSilent {
					return false
				}
				if opcode == core.OP_NEWARRAY {
					if len(op.Data) != 1 || op.Data[0] < 4 || op.Data[0] > 11 {
						return false
					}
				} else {
					if len(op.Data) != 2 {
						return false
					}
					if _, known := sourceBridgeClassName(obj, core.Convert2bytesToInt(op.Data)); !known {
						return false
					}
				}
				if _, ok := pop('I'); !ok {
					return false
				}
				stack = append(stack, constructorEffectValue{kind: 'L'})
			case opcode == core.OP_ACONST_NULL:
				stack = append(stack, constructorEffectValue{kind: 'L'})
			case opcode >= core.OP_ICONST_M1 && opcode <= core.OP_ICONST_5 || opcode == core.OP_BIPUSH || opcode == core.OP_SIPUSH:
				stack = append(stack, constructorEffectValue{kind: 'I'})
			case opcode >= core.OP_LCONST_0 && opcode <= core.OP_DCONST_1:
				kind := byte('F')
				if opcode <= core.OP_LCONST_1 {
					kind = 'J'
				} else if opcode >= core.OP_DCONST_0 {
					kind = 'D'
				}
				stack = append(stack, constructorEffectValue{kind: kind})
			case opcode == core.OP_LDC || opcode == core.OP_LDC_W || opcode == core.OP_LDC2_W:
				cp := 0
				if len(op.Data) == 1 {
					cp = int(op.Data[0])
				} else if len(op.Data) == 2 {
					cp = int(core.Convert2bytesToInt(op.Data))
				}
				if cp <= 0 || cp > len(obj.ConstantPool) {
					return false
				}
				kind := byte(0)
				switch obj.ConstantPool[cp-1].(type) {
				case *ConstantIntegerInfo:
					kind = 'I'
				case *ConstantFloatInfo:
					kind = 'F'
				case *ConstantLongInfo:
					kind = 'J'
				case *ConstantDoubleInfo:
					kind = 'D'
				case *ConstantStringInfo, *ConstantClassInfo:
					kind = 'L'
				}
				if _, class := obj.ConstantPool[cp-1].(*ConstantClassInfo); class && initialized {
					// A missing class can fail linkage after Object initialization.
					// Do not move captures past that potentially finalizable failure.
					return false
				}
				if kind == 0 || (opcode == core.OP_LDC2_W) != (kind == 'J' || kind == 'D') {
					return false
				}
				stack = append(stack, constructorEffectValue{kind: kind})
			case opcode >= core.OP_DUP && opcode <= core.OP_SWAP:
				if len(op.Data) != 0 {
					return false
				}
				var known bool
				stack, known = constructorEffectStackPermutation(stack, opcode)
				if !known {
					return false
				}
			case opcode == core.OP_POP || opcode == core.OP_POP2:
				v, ok := pop(0)
				if !ok {
					return false
				}
				if opcode == core.OP_POP && v.width() != 1 {
					return false
				}
				if opcode == core.OP_POP2 && v.width() == 1 {
					w, ok := pop(0)
					if !ok || w.width() != 1 {
						return false
					}
				}
			case opcode == core.OP_GETFIELD || opcode == core.OP_PUTFIELD:
				member := constructorMotionMember(obj, op, opcode)
				if member == nil {
					return false
				}
				typeOf := constructorEffectType(member.Description)
				params, _, err := callbinding.Descriptor("(" + member.Description + ")V")
				if err != nil || len(params) != 1 || typeOf.width() == 0 {
					return false
				}
				var stored constructorEffectValue
				if opcode == core.OP_PUTFIELD {
					value, ok := pop(typeOf.kind)
					if !ok {
						return false
					}
					if value.allocation != 0 || value.receiver && !initialized {
						// Uninitialized THIS is not a legal stored reference value.
						return false
					}
					stored = value
				}
				receiver, ok := pop('L')
				if !ok || receiver.allocation != 0 {
					return false
				}
				if !receiver.receiver {
					// No earlier operation has published THIS. An independent
					// input or initialized allocation cannot hide an alias to it.
					// Keep the original field operation and failure timing; after
					// Object initialization, failure needs the closed finalizer.
					if stored.receiver || initialized && !c.constructorReceiverFinalizerSilent {
						return false
					}
					if opcode == core.OP_GETFIELD {
						stack = append(stack, typeOf)
					}
					continue
				}
				aliases.selfStored = aliases.selfStored || stored.receiver
				field, known := c.constructorEffectField(obj, member, opcode == core.OP_PUTFIELD, remaining)
				if !known || writes[field] {
					return false
				}
				if opcode == core.OP_GETFIELD {
					if !initialized {
						return false
					}
					aliases.referenceRead = aliases.referenceRead || typeOf.kind == 'L'
					stack = append(stack, typeOf)
				}
			case opcode == core.OP_INVOKESPECIAL || opcode == core.OP_INVOKESTATIC || opcode == core.OP_INVOKEVIRTUAL || opcode == core.OP_INVOKEINTERFACE:
				member := constructorMotionMember(obj, op, opcode)
				if member == nil || member.Member == "<clinit>" {
					return false
				}
				args, result, err := callbinding.Descriptor(member.Description)
				if err != nil {
					return false
				}
				words := 1
				for _, arg := range args {
					words += constructorEffectType(arg).width()
				}
				if opcode == core.OP_INVOKEINTERFACE && (words > 255 || int(op.Data[2]) != words) {
					return false
				}
				for i := len(args) - 1; i >= 0; i-- {
					v, ok := pop(constructorEffectType(args[i]).kind)
					if !ok || v.receiver || v.allocation != 0 {
						return false
					}
				}
				var receiver constructorEffectValue
				if opcode != core.OP_INVOKESTATIC {
					var ok bool
					receiver, ok = pop('L')
					if !ok {
						return false
					}
				}
				if receiver.receiver && member.Member != "<init>" {
					if !initialized || !c.constructorReceiverFinalizerSilent || len(args) != 0 {
						return false
					}
					value, known := c.constructorReceiverReadOnlyMethod(obj, member, opcode, writes, remaining)
					if !known {
						return false
					}
					aliases.referenceRead = aliases.referenceRead || value.kind == 'L'
					if result != "V" {
						stack = append(stack, value)
					}
					continue
				}
				if initialized && !c.constructorReceiverFinalizerSilent {
					return false
				}
				if member.Member == "<init>" {
					if receiver.allocation != 0 {
						if opcode != core.OP_INVOKESPECIAL || result != "V" || member.Name != allocations[receiver.allocation] {
							return false
						}
						// JVM initialization changes every alias of THIS allocation,
						// without changing the enclosing receiver's initialized state.
						for i := range locals {
							if locals[i].allocation == receiver.allocation {
								locals[i].allocation = 0
							}
						}
						for i := range stack {
							if stack[i].allocation == receiver.allocation {
								stack[i].allocation = 0
							}
						}
					} else {
						if initialized || opcode != core.OP_INVOKESPECIAL || result != "V" || !receiver.receiver || (member.Name != obj.GetClassName() && member.Name != obj.GetSupperClassName()) || !c.constructorChainEffects(member.Name, member.Description, writes, active, remaining, depth+1, aliases) {
							return false
						}
						initialized = true
					}
				} else {
					// Receiver-free calls keep external effects in the same order.
					// Before Object initialization, a throw cannot expose this via
					// finalization. Afterward, require the separate closed-receiver
					// proof that its original finalizer is unobservable.
					// THIS arguments/receiver are rejected; results cannot alias
					// THIS because no earlier operation has published it.
					if receiver.receiver || receiver.allocation != 0 {
						return false
					}
					if result != "V" {
						stack = append(stack, constructorEffectType(result))
					}
				}
			case opcode >= core.OP_IADD && opcode <= core.OP_DREM:
				kind := []byte{'I', 'J', 'F', 'D'}[(opcode-core.OP_IADD)%4]
				if kind == 'I' || kind == 'J' {
					if opcode >= core.OP_IDIV {
						return false
					}
				}
				_, ok := pop(kind)
				_, ok2 := pop(kind)
				if !ok || !ok2 {
					return false
				}
				stack = append(stack, constructorEffectValue{kind: kind})
			case opcode >= core.OP_INEG && opcode <= core.OP_DNEG:
				kind := []byte{'I', 'J', 'F', 'D'}[opcode-core.OP_INEG]
				_, ok := pop(kind)
				if !ok {
					return false
				}
				stack = append(stack, constructorEffectValue{kind: kind})
			case opcode >= core.OP_ISHL && opcode <= core.OP_LXOR:
				kind := byte('I')
				if (opcode-core.OP_ISHL)%2 == 1 {
					kind = 'J'
				}
				right := kind
				if opcode <= core.OP_LUSHR {
					right = 'I'
				}
				_, ok := pop(right)
				_, ok2 := pop(kind)
				if !ok || !ok2 {
					return false
				}
				stack = append(stack, constructorEffectValue{kind: kind})
			case opcode >= core.OP_I2L && opcode <= core.OP_I2S:
				conversions := [][2]byte{{'I', 'J'}, {'I', 'F'}, {'I', 'D'}, {'J', 'I'}, {'J', 'F'}, {'J', 'D'}, {'F', 'I'}, {'F', 'J'}, {'F', 'D'}, {'D', 'I'}, {'D', 'J'}, {'D', 'F'}, {'I', 'I'}, {'I', 'I'}, {'I', 'I'}}
				pair := conversions[opcode-core.OP_I2L]
				_, ok := pop(pair[0])
				if !ok {
					return false
				}
				stack = append(stack, constructorEffectValue{kind: pair[1]})
			case opcode >= core.OP_IFEQ && opcode <= core.OP_IF_ACMPNE || opcode == core.OP_IFNULL || opcode == core.OP_IFNONNULL:
				kind, count := byte('I'), 1
				if opcode >= core.OP_IF_ICMPEQ && opcode <= core.OP_IF_ACMPNE {
					count = 2
				}
				if opcode >= core.OP_IF_ACMPEQ && opcode <= core.OP_IF_ACMPNE || opcode == core.OP_IFNULL || opcode == core.OP_IFNONNULL {
					kind = 'L'
				}
				for i := 0; i < count; i++ {
					if v, ok := pop(kind); !ok || v.allocation != 0 {
						return false
					}
				}
				branch, ok := target(op)
				if !ok || branch <= index {
					return false
				}
				// Each arm owns its aliases and operand stack. A publication or exceptional
				// operation in even one arm rejects the entire movement proof.
				return walk(branch, append([]constructorEffectValue(nil), locals...), append([]constructorEffectValue(nil), stack...), initialized) && walk(index+1, locals, stack, initialized)
			case opcode == core.OP_GOTO || opcode == core.OP_GOTO_W:
				branch, ok := target(op)
				if !ok || branch <= index {
					return false
				}
				return walk(branch, locals, stack, initialized)
			case opcode >= core.OP_LCMP && opcode <= core.OP_DCMPG:
				kind := byte('D')
				if opcode == core.OP_LCMP {
					kind = 'J'
				} else if opcode <= core.OP_FCMPG {
					kind = 'F'
				}
				_, ok := pop(kind)
				_, ok2 := pop(kind)
				if !ok || !ok2 {
					return false
				}
				stack = append(stack, constructorEffectValue{kind: 'I'})
			case opcode == core.OP_RETURN:
				return initialized && len(stack) == 0
			default:
				return false
			}
		}
		return false
	}
	return walk(0, locals, nil, false)
}

// JVM dup forms copy an exact one/two-word top packet below an exact
// zero/one/two-word packet. Packet boundaries may never split a category-2
// value. Copy whole abstract values so receiver and NEW identities survive.
func constructorEffectStackPermutation(stack []constructorEffectValue, opcode int) ([]constructorEffectValue, bool) {
	packet := func(end, words int) (int, bool) {
		for words > 0 {
			if end <= 0 {
				return 0, false
			}
			end--
			width := stack[end].width()
			if width == 0 {
				return 0, false
			}
			words -= width
		}
		return end, words == 0
	}
	if opcode == core.OP_SWAP {
		top, ok := packet(len(stack), 1)
		if !ok {
			return nil, false
		}
		under, ok := packet(top, 1)
		if !ok {
			return nil, false
		}
		result := append([]constructorEffectValue(nil), stack[:under]...)
		result = append(result, stack[top:]...)
		result = append(result, stack[under:top]...)
		return result, true
	}
	if opcode < core.OP_DUP || opcode > core.OP_DUP2_X2 {
		return nil, false
	}
	copyWords, insertWords := 1, opcode-core.OP_DUP
	if opcode >= core.OP_DUP2 {
		copyWords = 2
		insertWords = opcode - core.OP_DUP2
	}
	top, ok := packet(len(stack), copyWords)
	if !ok {
		return nil, false
	}
	under, ok := packet(top, insertWords)
	if !ok {
		return nil, false
	}
	result := make([]constructorEffectValue, 0, len(stack)+len(stack)-top)
	result = append(result, stack[:under]...)
	result = append(result, stack[top:]...)
	result = append(result, stack[under:]...)
	return result, true
}
