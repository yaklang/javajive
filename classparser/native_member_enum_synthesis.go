package javaclassparser

import (
	"encoding/binary"
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Java regenerates these enum members instead of rendering their bodies.
// Their original code must implement that exact protocol before a member enum
// can join a lexical ownership plan. An enum flag or a synthetic-looking name
// alone grants no right to replace a method, backing array or constructor.
type nativeMemberEnumSynthesis struct {
	assertions  *nativeMemberAssertion
	valuesField *MemberInfo
	constants   map[string]nativeEnumConstantAllocation
	bodies      map[string]*nativeEnumConstantBody
}

// javac reserves a generated helper name by adding '$' until no source method
// already owns it (overloads count too). Compare that naming protocol with the
// original flags and descriptor; a prefix match alone would delete user code.
func nativeEnumArrayHelperName(obj *ClassObject, work *workbudget.Budget) (string, bool) {
	occupied := make(map[string]bool)
	for _, method := range obj.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return "", false
		}
		if method.AccessFlags&0x1000 != 0 {
			continue
		}
		name, known := sourceBridgeUTF8(obj, method.NameIndex)
		if !known {
			return "", false
		}
		occupied[name] = true
	}
	name := "$values"
	for i := 0; i <= len(occupied); i++ {
		if !nativeProofWork(work, 1) {
			return "", false
		}
		if !occupied[name] {
			return name, true
		}
		name += "$"
	}
	return "", false
}

// Java reserves values() and valueOf(String) for generated methods. Unlike an
// ordinary method, their original executable bodies cannot be retained in enum
// source. Prove each replacement independently of constructor parameters and
// constant-specific bodies, so raw fallback has the same semantic boundary as
// lexical ownership. A refused body must produce an explicit failure, never a
// compilable enum whose implicit factories behave differently.
func nativeEnumValuesFactoryBacking(obj *ClassObject, method *MemberInfo, work *workbudget.Budget) *MemberInfo {
	return nativeEnumValuesFactoryBackingWithParameterNames(obj, method, work, false)
}
func nativeEnumValuesFactoryBackingWithParameterNames(obj *ClassObject, method *MemberInfo, work *workbudget.Budget, allowNames bool) *MemberInfo {
	if obj == nil || method == nil || method.AccessFlags != 9 || !nativeEnumOriginalFrameCapacity(method, 1, 0) {
		return nil
	}
	name := obj.GetClassName()
	array := "[L" + name + ";"
	ops, known := nativeEnumMethodOpsWithParameterNames(obj, method, work, allowNames)
	if !known || len(ops) != 4 || !nativeEnumOpcode(ops[0], core.OP_GETSTATIC) || !nativeEnumMemberOperand(obj, ops[1], core.OP_INVOKEVIRTUAL, array, "clone", "()Ljava/lang/Object;") || !nativeEnumOpcode(ops[2], core.OP_CHECKCAST) || !nativeEnumClassOperand(obj, ops[2], array) || !nativeEnumOpcode(ops[3], core.OP_ARETURN) {
		return nil
	}
	member := constructorMotionMember(obj, ops[0], core.OP_GETSTATIC)
	if member == nil || member.Name != name || member.Description != array || !nativeEnumMemberOperand(obj, ops[0], core.OP_GETSTATIC, name, member.Member, array) {
		return nil
	}
	var backing *MemberInfo
	for _, field := range obj.Fields {
		if field == nil || !nativeProofWork(work, 1) {
			return nil
		}
		fieldName, nk := sourceBridgeUTF8(obj, field.NameIndex)
		descriptor, dk := sourceBridgeUTF8(obj, field.DescriptorIndex)
		if !nk || !dk {
			return nil
		}
		if fieldName == member.Member && descriptor == array {
			if backing != nil || field.AccessFlags != 0x101a || len(field.Attributes) != 0 {
				return nil
			}
			backing = field
		}
	}
	return backing
}

func nativeEnumValueOfFactoryProof(obj *ClassObject, method *MemberInfo, work *workbudget.Budget) bool {
	return nativeEnumValueOfFactoryProofWithParameterNames(obj, method, work, false)
}
func nativeEnumValueOfFactoryProofWithParameterNames(obj *ClassObject, method *MemberInfo, work *workbudget.Budget, allowNames bool) bool {
	if obj == nil || method == nil || method.AccessFlags != 9 || !nativeEnumOriginalFrameCapacity(method, 2, 1) {
		return false
	}
	name := obj.GetClassName()
	ops, known := nativeEnumMethodOpsWithParameterNames(obj, method, work, allowNames)
	return known && len(ops) == 5 && (nativeEnumOpcode(ops[0], core.OP_LDC) || nativeEnumOpcode(ops[0], core.OP_LDC_W)) && nativeEnumClassOperand(obj, ops[0], name) && constructorMotionLoad(ops[1], "Ljava/lang/String;") && core.GetRetrieveIdx(ops[1]) == 0 && nativeEnumMemberOperand(obj, ops[2], core.OP_INVOKESTATIC, "java/lang/Enum", "valueOf", "(Ljava/lang/Class;Ljava/lang/String;)Ljava/lang/Enum;") && nativeEnumOpcode(ops[3], core.OP_CHECKCAST) && nativeEnumClassOperand(obj, ops[3], name) && nativeEnumOpcode(ops[4], core.OP_ARETURN)
}

// Only the actual synthetic array helper is suppressed. A user-declared
// overload named $values has ordinary source semantics and must be rendered.
func nativeEnumRegeneratedMethod(obj *ClassObject, method *MemberInfo, name, descriptor string, work *workbudget.Budget) (bool, error) {
	self := "L" + obj.GetClassName() + ";"
	generated, closed := false, false
	switch {
	case name == "values" && descriptor == "()["+self:
		generated, closed = true, nativeEnumValuesFactoryBackingWithParameterNames(obj, method, work, true) != nil
	case name == "valueOf" && descriptor == "(Ljava/lang/String;)"+self:
		generated, closed = true, nativeEnumValueOfFactoryProofWithParameterNames(obj, method, work, true)
	case descriptor == "()["+self && method.AccessFlags == 0x100a:
		generated = true
		helperName, named := nativeEnumArrayHelperName(obj, work)
		if !named || helperName != name {
			return true, fmt.Errorf("enum regeneration: original array helper has no compiler naming certificate")
		}
		var constants []string
		for _, field := range obj.Fields {
			if field == nil || !nativeProofWork(work, 1) {
				return true, fmt.Errorf("enum regeneration: original constant table is unproved")
			}
			if field.AccessFlags&0x4000 != 0 {
				n, nk := sourceBridgeUTF8(obj, field.NameIndex)
				d, dk := sourceBridgeUTF8(obj, field.DescriptorIndex)
				if !nk || !dk || field.AccessFlags != 0x4019 || d != self {
					return true, fmt.Errorf("enum regeneration: invalid original constant for %s", obj.GetClassName())
				}
				constants = append(constants, n)
			}
		}
		ops, known := nativeEnumMethodOps(obj, method, work)
		stack := uint16(1)
		if len(constants) > 0 {
			stack = 4
		}
		closed = known && len(ops) > 0 && nativeEnumOriginalFrameCapacity(method, stack, 0) && nativeEnumOpcode(ops[len(ops)-1], core.OP_ARETURN) && nativeEnumValuesArrayPacket(obj, ops[:len(ops)-1], constants, work)
	}
	if generated && !closed {
		if work != nil && work.Err() != nil {
			return true, work.Err()
		}
		return true, fmt.Errorf("enum regeneration: original %s.%s%s does not implement the source compiler protocol", obj.GetClassName(), name, descriptor)
	}
	return generated, nil
}

func nativeEnumMethodOps(obj *ClassObject, method *MemberInfo, work *workbudget.Budget) ([]*core.OpCode, bool) {
	return nativeEnumMethodOpsWithParameterNames(obj, method, work, false)
}
func nativeEnumMethodOpsWithParameterNames(obj *ClassObject, method *MemberInfo, work *workbudget.Budget, allowNames bool) ([]*core.OpCode, bool) {
	if obj == nil || method == nil {
		return nil, false
	}
	var code *CodeAttribute
	constructorSignature := false
	parametersSeen := false
	for _, attribute := range method.Attributes {
		if body, ok := attribute.(*CodeAttribute); ok {
			if body == nil || code != nil {
				return nil, false
			}
			code = body
		} else if signature, ok := attribute.(*SignatureAttribute); ok {
			if signature == nil {
				return nil, false
			}
			name, known := sourceBridgeUTF8(obj, method.NameIndex)
			value, closed := sourceBridgeUTF8(obj, signature.SignatureIndex)
			descriptor, dk := sourceBridgeUTF8(obj, method.DescriptorIndex)
			reader := NewClassObjectDumper(obj)
			reader.Work = work
			if constructorSignature || !known || name != "<init>" || !closed || !dk || !reader.nativeEnumConstructorSignatureMatches(method, value, descriptor) {
				return nil, false
			}
			constructorSignature = true
		} else if parameters, ok := attribute.(*UnparsedAttribute); ok && parameters != nil && parameters.Name == "MethodParameters" {
			name, known := sourceBridgeUTF8(obj, method.NameIndex)
			count, flags := 1, uint16(0x8000)
			if name == "<init>" {
				descriptor, dk := sourceBridgeUTF8(obj, method.DescriptorIndex)
				physical, result, err := callbinding.Descriptor(descriptor)
				if !dk || err != nil || result != "V" || len(physical) < 2 || len(physical) > 255 || physical[0] != "Ljava/lang/String;" || physical[1] != "I" {
					return nil, false
				}
				count, flags = len(physical), 0x1000
			} else if name != "valueOf" {
				return nil, false
			}
			if parametersSeen || !known || parameters.Length != uint32(1+4*count) || len(parameters.Info) != 1+4*count || int(parameters.Info[0]) != count {
				return nil, false
			}
			for i := 0; i < count; i++ {
				data := parameters.Info[1+4*i : 5+4*i]
				index := uint16(data[0])<<8 | uint16(data[1])
				if index != 0 {
					value, known := sourceBridgeUTF8(obj, index)
					if !allowNames || !known || value == "" {
						return nil, false
					}
				}
				expectedFlags := flags
				if name == "<init>" && i >= 2 {
					// Only the compiler's name/ordinal parameters are synthetic.
					// Unnamed ordinary source parameters retain their physical
					// entries; names or extra flags need separate source evidence.
					expectedFlags = 0
				}
				if uint16(data[2])<<8|uint16(data[3]) != expectedFlags {
					return nil, false
				}
			}
			parametersSeen = true
		} else {
			return nil, false
		}
	}
	if code == nil || len(code.ExceptionTable) != 0 || !nativeProofWork(work, int64(len(code.Code))) {
		return nil, false
	}
	name, known := sourceBridgeUTF8(obj, method.NameIndex)
	if !known {
		return nil, false
	}
	descriptor, _ := sourceBridgeUTF8(obj, method.DescriptorIndex)
	if name == "values" || name == "valueOf" || method.AccessFlags == 0x100a && descriptor == "()[L"+obj.GetClassName()+";" {
		// These bodies disappear from source. Debug locations are harmless;
		// executable type annotations and opaque attributes have no source
		// declaration here and cannot be silently claimed as regenerated.
		for _, attribute := range code.Attributes {
			if !nativeProofWork(work, 1) {
				return nil, false
			}
			switch a := attribute.(type) {
			case *LineNumberTableAttribute:
				if a == nil {
					return nil, false
				}
			case *UnparsedAttribute:
				if a == nil || a.Name != "LocalVariableTable" || len(a.Info) < 2 || a.Length != uint32(len(a.Info)) || len(a.Info) != 2+10*int(binary.BigEndian.Uint16(a.Info)) || !nativeProofWork(work, int64(len(a.Info))) {
					return nil, false
				}
			default:
				return nil, false
			}
		}
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return nil, false
	}
	return constructorMotionOps(decoder), true
}

func nativeEnumOriginalFrameCapacity(method *MemberInfo, stack, locals uint16) bool {
	for _, attribute := range method.Attributes {
		if code, ok := attribute.(*CodeAttribute); ok && code != nil {
			return code.MaxStack >= stack && code.MaxLocals >= locals
		}
	}
	return false
}
func nativeEnumCPIndex(op *core.OpCode) (uint16, bool) {
	if op == nil || op.Instr == nil {
		return 0, false
	}
	if op.Instr.OpCode == core.OP_LDC {
		if len(op.Data) != 1 {
			return 0, false
		}
		return uint16(op.Data[0]), true
	}
	if len(op.Data) != 2 {
		return 0, false
	}
	return uint16(core.Convert2bytesToInt(op.Data)), true
}
func nativeEnumOriginalInt(obj *ClassObject, op *core.OpCode) (int, bool) {
	if op == nil || op.Instr == nil {
		return 0, false
	}
	switch code := op.Instr.OpCode; {
	case code >= core.OP_ICONST_M1 && code <= core.OP_ICONST_5:
		return int(code) - int(core.OP_ICONST_0), len(op.Data) == 0
	case code == core.OP_BIPUSH:
		if len(op.Data) == 1 {
			return int(int8(op.Data[0])), true
		}
	case code == core.OP_SIPUSH:
		if len(op.Data) == 2 {
			return int(int16(uint16(core.Convert2bytesToInt(op.Data)))), true
		}
	case code == core.OP_LDC || code == core.OP_LDC_W:
		index, known := nativeEnumCPIndex(op)
		if known && index > 0 && int(index) <= len(obj.ConstantPool) {
			if value, ok := obj.ConstantPool[index-1].(*ConstantIntegerInfo); ok && value != nil {
				return int(value.Value), true
			}
		}
	}
	return 0, false
}
func nativeEnumOpcode(op *core.OpCode, code int) bool {
	return op != nil && op.Instr != nil && op.Instr.OpCode == code
}
func nativeEnumClassOperand(obj *ClassObject, op *core.OpCode, name string) bool {
	index, known := nativeEnumCPIndex(op)
	actual, closed := sourceBridgeClassName(obj, index)
	return known && closed && actual == name
}
func nativeEnumMemberOperand(obj *ClassObject, op *core.OpCode, code int, owner, name, descriptor string) bool {
	index, known := nativeEnumCPIndex(op)
	if !known || index == 0 || int(index) > len(obj.ConstantPool) {
		return false
	}
	if code == core.OP_GETSTATIC || code == core.OP_PUTSTATIC {
		if field, ok := obj.ConstantPool[index-1].(*ConstantFieldrefInfo); !ok || field == nil {
			return false
		}
	} else {
		if method, ok := obj.ConstantPool[index-1].(*ConstantMethodrefInfo); !ok || method == nil {
			return false
		}
	}
	member := constructorMotionMember(obj, op, code)
	return member != nil && member.Name == owner && member.Member == name && member.Description == descriptor
}

// Array construction has the same stack contract in a helper and in <clinit>:
// leave one fresh, declaration-ordered array. Its consumer is separately proved
// as ARETURN or PUTSTATIC; neither consumer is part of array construction.
func nativeEnumValuesArrayPacket(obj *ClassObject, ops []*core.OpCode, constants []string, work *workbudget.Budget) bool {
	name := obj.GetClassName()
	descriptor := "L" + name + ";"
	if len(ops) != 2+4*len(constants) || !nativeProofWork(work, int64(len(ops))) {
		return false
	}
	size, known := nativeEnumOriginalInt(obj, ops[0])
	if !known || size != len(constants) || !nativeEnumOpcode(ops[1], core.OP_ANEWARRAY) || !nativeEnumClassOperand(obj, ops[1], name) {
		return false
	}
	for i, constant := range constants {
		start := 2 + 4*i
		index, known := nativeEnumOriginalInt(obj, ops[start+1])
		if !nativeEnumOpcode(ops[start], core.OP_DUP) || !known || index != i || !nativeEnumMemberOperand(obj, ops[start+2], core.OP_GETSTATIC, name, constant, descriptor) || !nativeEnumOpcode(ops[start+3], core.OP_AASTORE) {
			return false
		}
	}
	return true
}

// The original canonical profile remains available without sibling declarations.
// Constant-specific bodies additionally require their original allocation,
// enclosing identity and pure access-bridge constructor proof. Both profiles
// independently certify every generated factory and backing-array instruction;
// an enum flag or matching source spelling cannot replace executable code.
func nativeMemberEnumSynthesisProof(obj *ClassObject, flags uint16, work *workbudget.Budget) *nativeMemberEnumSynthesis {
	return nativeMemberEnumSynthesisWithDeclarations(obj, flags, nil, work)
}

func nativeMemberEnumSynthesisWithDeclarations(obj *ClassObject, flags uint16, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) *nativeMemberEnumSynthesis {
	canonical := obj != nil && flags&0x4018 == 0x4018 && flags & ^uint16(0x401f) == 0 && obj.AccessFlags&0x4030 == 0x4030 && obj.AccessFlags & ^uint16(0x4031) == 0
	constantBodies := obj != nil && resolve != nil && flags&0x4008 == 0x4008 && flags & ^uint16(0x441f) == 0 && obj.AccessFlags&0x4020 == 0x4020 && obj.AccessFlags & ^uint16(0x4431) == 0 && flags&0x410 == obj.AccessFlags&0x410 && flags&0x10 == 0
	if obj == nil || (!canonical && !constantBodies) || obj.GetSupperClassName() != "java/lang/Enum" || !nativeProofWork(work, 1) {
		return nil
	}
	assertions, valid := nativeEnumAssertionInitialization(obj, resolve, work)
	if !valid {
		return nil
	}
	name := obj.GetClassName()
	descriptor := "L" + name + ";"
	array := "[" + descriptor
	var constants []string
	var backing *MemberInfo
	for _, field := range obj.Fields {
		if field == nil || !nativeProofWork(work, 1) {
			return nil
		}
		n, nk := sourceBridgeUTF8(obj, field.NameIndex)
		d, dk := sourceBridgeUTF8(obj, field.DescriptorIndex)
		if !nk || !dk {
			return nil
		}
		if field.AccessFlags&0x4000 != 0 {
			if field.AccessFlags != 0x4019 || d != descriptor || len(constants) >= 64 {
				return nil
			}
			constants = append(constants, n)
		}
		if assertions != nil && n == nativeAssertionField {
			continue // Independently certified assertion flag, separate from the backing array.
		}
		if field.AccessFlags&0x1000 != 0 {
			if backing != nil || field.AccessFlags != 0x101a || d != array || len(field.Attributes) != 0 {
				return nil
			}
			backing = field
		}
	}
	if backing == nil {
		return nil
	}
	backingName, _ := sourceBridgeUTF8(obj, backing.NameIndex)
	// The source compiler recreates this canonical backing name. Collision-
	// renamed generated arrays need a separate compiler naming certificate.
	if backingName != "$VALUES" {
		return nil
	}
	var valuesMethod, valueOf, helper, ctor, initializer *MemberInfo
	bridges := map[string]*nativeConstructorAccessBridge{}
	if constantBodies {
		reader := NewClassObjectDumper(obj)
		reader.Work = work
		bridges = reader.nativeConstructorAccessBridges()
		if bridges == nil {
			return nil
		}
	}
	constructors := map[string]*MemberInfo{}
	var constructorOrder []*MemberInfo
	sourceArguments := false
	for _, method := range obj.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return nil
		}
		n, nk := sourceBridgeUTF8(obj, method.NameIndex)
		d, dk := sourceBridgeUTF8(obj, method.DescriptorIndex)
		if !nk || !dk {
			return nil
		}
		switch {
		case n == "values" && d == "()"+array:
			if valuesMethod != nil || method.AccessFlags != 9 {
				return nil
			}
			valuesMethod = method
		case n == "valueOf" && d == "(Ljava/lang/String;)"+descriptor:
			if valueOf != nil || method.AccessFlags != 9 {
				return nil
			}
			valueOf = method
		case method.AccessFlags == 0x100a && d == "()"+array:
			if helper != nil || method.AccessFlags != 0x100a || d != "()"+array {
				return nil
			}
			helper = method
		case n == "<init>" && method.AccessFlags == 0x1000 && constantBodies:
			if bridge := bridges[d]; bridge == nil || bridge.method != method {
				return nil
			}
		case n == "<init>":
			params, result, err := callbinding.Descriptor(d)
			if constructors[d] != nil || len(constructors) >= 64 || (method.AccessFlags != 2 && method.AccessFlags != 0x82) || err != nil || result != "V" || len(params) < 2 || params[0] != "Ljava/lang/String;" || params[1] != "I" || method.AccessFlags&0x80 != 0 && (len(params) == 2 || params[len(params)-1][0] != '[') {
				return nil
			}
			constructors[d] = method
			constructorOrder = append(constructorOrder, method)
			sourceArguments = sourceArguments || len(params) > 2
			ctor = method
		case n == "<clinit>":
			if initializer != nil || method.AccessFlags != 8 || d != "()V" {
				return nil
			}
			initializer = method
		}
	}
	if valuesMethod == nil || valueOf == nil || ctor == nil || initializer == nil {
		return nil
	}
	initializerStack, arrayStack := uint16(1), uint16(1)
	if len(constants) > 0 {
		initializerStack, arrayStack = 4, 4
	}
	if !nativeEnumOriginalFrameCapacity(valuesMethod, 1, 0) || !nativeEnumOriginalFrameCapacity(valueOf, 2, 1) || !nativeEnumOriginalFrameCapacity(initializer, initializerStack, 0) || helper != nil && !nativeEnumOriginalFrameCapacity(helper, arrayStack, 0) {
		return nil
	}
	if nativeEnumValuesFactoryBacking(obj, valuesMethod, work) != backing || !nativeEnumValueOfFactoryProof(obj, valueOf, work) {
		return nil
	}
	var ops []*core.OpCode
	var known bool
	if helper != nil {
		expectedName, named := nativeEnumArrayHelperName(obj, work)
		actualName, _ := sourceBridgeUTF8(obj, helper.NameIndex)
		if !named || expectedName != actualName {
			return nil
		}
		ops, known = nativeEnumMethodOps(obj, helper, work)
		if !known || len(ops) == 0 || !nativeEnumOpcode(ops[len(ops)-1], core.OP_ARETURN) || !nativeEnumValuesArrayPacket(obj, ops[:len(ops)-1], constants, work) {
			return nil
		}
	}
	// The hidden name/ordinal belong only to the initial Enum super call. Any
	// later use or overwrite would have no corresponding source-level parameter.
	for _, original := range constructorOrder {
		physical, _ := sourceBridgeUTF8(obj, original.DescriptorIndex)
		params, _, _ := callbinding.Descriptor(physical)
		if !nativeEnumOriginalFrameCapacity(original, 3, uint16(nativeMemberParameterWidth(params)+1)) {
			return nil
		}
		ops, known = nativeEnumMethodOps(obj, original, work)
		if !known || len(ops) < 5 || !constructorMotionLoad(ops[0], descriptor) || core.GetRetrieveIdx(ops[0]) != 0 || !constructorMotionLoad(ops[1], "Ljava/lang/String;") || core.GetRetrieveIdx(ops[1]) != 1 || !constructorMotionLoad(ops[2], "I") || core.GetRetrieveIdx(ops[2]) != 2 || !nativeEnumMemberOperand(obj, ops[3], core.OP_INVOKESPECIAL, "java/lang/Enum", "<init>", "(Ljava/lang/String;I)V") {
			return nil
		}
		for _, op := range ops[4:] {
			load, store := core.GetRetrieveIdx(op), core.GetStoreIdx(op)
			if load == 1 || load == 2 || store == 1 || store == 2 {
				return nil
			}
		}
	}
	// Enum constants precede the backing array and all ordinary source static
	// initialization. Their hidden operands are original literal name and ordinal.
	ops, known = nativeEnumMethodOps(obj, initializer, work)
	if !known || len(ops) < 6*len(constants)+2 {
		return nil
	}
	cursor := 0
	if assertions != nil {
		if len(ops) <= 7 || int(ops[7].CurrentOffset) != assertions.initializerEndPC {
			return nil
		}
		cursor = 7
	}
	var allocations map[string]nativeEnumConstantAllocation
	bodies := map[string]*nativeEnumConstantBody{}
	if sourceArguments || constantBodies {
		// Source arguments have variable instruction spans and category widths.
		// Reuse the immutable typed NEW/invoke/store origin proof; fixed six-op
		// matching cannot distinguish nested allocations from the enum receiver.
		// The same proof checks literal hidden operands, exact original targets,
		// declaration order and all control-flow edges, without evaluating values.
		reader := NewClassObjectDumper(obj)
		reader.Work = work
		var err error
		allocations, err = reader.nativeEnumConstantInitializationsWithDeclarations(resolve)
		if err != nil || len(allocations) != len(constants) {
			return nil
		}
		for ordinal, constant := range constants {
			plan, exists := allocations[constant]
			if !exists || plan.ordinal != ordinal || assertions != nil && ordinal == 0 && (cursor >= len(ops) || plan.newPC != int(ops[cursor].CurrentOffset)) {
				return nil
			}
			if plan.allocatedClass == name {
				if constructors[plan.descriptor] == nil {
					return nil
				}
			} else {
				if !constantBodies || bodies[plan.allocatedClass] != nil {
					return nil
				}
				body := nativeEnumConstantBodyProof(obj, plan, len(bodies)+1, bridges, resolve, work)
				if body == nil {
					return nil
				}
				bodies[plan.allocatedClass] = body
			}
			if ordinal == len(constants)-1 {
				found := false
				for i, op := range ops {
					if int(op.CurrentOffset) == plan.storePC {
						cursor, found = i+1, true
						break
					}
				}
				if !found {
					return nil
				}
			}
		}
	} else {
		for ordinal, constant := range constants {
			if cursor+6 > len(ops) {
				return nil
			}
			packet := ops[cursor : cursor+6]
			index, ok := nativeEnumCPIndex(packet[2])
			literal := ""
			if ok && index > 0 && int(index) <= len(obj.ConstantPool) {
				if str, yes := obj.ConstantPool[index-1].(*ConstantStringInfo); yes && str != nil {
					literal, _ = sourceBridgeUTF8(obj, str.StringIndex)
				}
			}
			number, nk := nativeEnumOriginalInt(obj, packet[3])
			if !nativeEnumOpcode(packet[0], core.OP_NEW) || !nativeEnumClassOperand(obj, packet[0], name) || !nativeEnumOpcode(packet[1], core.OP_DUP) || !(nativeEnumOpcode(packet[2], core.OP_LDC) || nativeEnumOpcode(packet[2], core.OP_LDC_W)) || literal != constant || !nk || number != ordinal || !nativeEnumMemberOperand(obj, packet[4], core.OP_INVOKESPECIAL, name, "<init>", "(Ljava/lang/String;I)V") || !nativeEnumMemberOperand(obj, packet[5], core.OP_PUTSTATIC, name, constant, descriptor) {
				return nil
			}
			cursor += 6
		}
	}
	if helper != nil {
		helperName, _ := sourceBridgeUTF8(obj, helper.NameIndex)
		if cursor+1 >= len(ops) || !nativeEnumMemberOperand(obj, ops[cursor], core.OP_INVOKESTATIC, name, helperName, "()"+array) || !nativeEnumMemberOperand(obj, ops[cursor+1], core.OP_PUTSTATIC, name, backingName, array) {
			return nil
		}
		cursor += 2
	} else {
		end := cursor + 2 + 4*len(constants)
		if end >= len(ops) || !nativeEnumMemberOperand(obj, ops[end], core.OP_PUTSTATIC, name, backingName, array) {
			return nil
		}
		if !nativeEnumValuesArrayPacket(obj, ops[cursor:end], constants, work) {
			return nil
		}
		cursor = end + 1
	}
	for _, op := range ops[cursor:] {
		if store := constructorMotionMember(obj, op, core.OP_PUTSTATIC); store != nil && store.Name == name {
			if store.Member == backingName {
				return nil
			}
			for _, constant := range constants {
				if store.Member == constant {
					return nil
				}
			}
		}
	}
	if constantBodies && len(bodies) == 0 {
		return nil
	}
	return &nativeMemberEnumSynthesis{assertions: assertions, valuesField: backing, constants: allocations, bodies: bodies}
}

// Parameter names are reflection-visible metadata, unlike Code debug tables.
// Raw source can retain a proved factory's execution but cannot directly write
// its generated parameter declaration. Keep that limitation explicit; lexical
// ownership admission continues to require the stronger unnamed profile.
func (c *ClassObjectDumper) noteEnumRegeneratedParameterMetadata(method *MemberInfo, name, descriptor string) {
	for _, attribute := range method.Attributes {
		parameters, ok := attribute.(*UnparsedAttribute)
		if !ok || parameters == nil || parameters.Name != "MethodParameters" {
			continue
		}
		for i := 1; i+3 < len(parameters.Info); i += 4 {
			if parameters.Info[i] != 0 || parameters.Info[i+1] != 0 {
				c.enumParameterMetadataUnsupported = true
				c.appendDiagnostic(DecompileDiagnostic{Code: "enum_factory_parameter_metadata", Method: name + descriptor, Message: "Original enum factory execution is proved; named generated parameters require compiler metadata configuration and are not certified by source alone."})
				return
			}
		}
	}
}
