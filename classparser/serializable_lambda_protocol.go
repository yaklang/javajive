package javaclassparser

import (
	"encoding/binary"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

const serializedLambdaDescriptor = "(Ljava/lang/invoke/SerializedLambda;)Ljava/lang/Object;"

// A serializable source reference regenerates an executable method in its
// capturing class. Its name/flags alone cannot authorize that replacement.
// This first protocol domain has no captures, adapters, marker interfaces or
// explicit bridges. The external static handle has the instantiated SAM's
// exact descriptor. Other domains remain unavailable, never approximated.
type serializableReferenceTuple struct {
	index                        uint16
	name, descriptor, functional string
	sam, instantiated            string
	implementation               nativeLambdaHandleTarget
}

type serializableReferenceSite struct {
	method *MemberInfo
	code   *CodeAttribute
	pc     int
	tuple  serializableReferenceTuple
}

type serializableReferenceProtocol struct {
	object *ClassObject
	owner  string
	helper *MemberInfo
	sites  []serializableReferenceSite
}

func serializableConstant(object *ClassObject, index uint16) ConstantInfo {
	if object == nil || index == 0 || int(index) > len(object.ConstantPool) {
		return nil
	}
	return object.ConstantPool[index-1]
}

func serializableReferenceTupleAt(object *ClassObject, bootstrap *BootstrapMethodsAttribute, index uint16) (serializableReferenceTuple, bool) {
	tuple := serializableReferenceTuple{index: index}
	dynamic, ok := serializableConstant(object, index).(*ConstantInvokeDynamicInfo)
	if !ok || dynamic == nil || bootstrap == nil || int(dynamic.BootstrapMethodAttrIndex) >= len(bootstrap.BootstrapMethods) {
		return tuple, false
	}
	method := bootstrap.BootstrapMethods[dynamic.BootstrapMethodAttrIndex]
	if method == nil || int(method.NumBootstrapArguments) != len(method.BootstrapArguments) {
		return tuple, false
	}
	handle, ok := serializableConstant(object, method.BootstrapMethodRef).(*ConstantMethodHandleInfo)
	target, known := nativeLambdaOriginalHandleTarget(object, handle)
	if !ok || handle == nil || handle.ReferenceKind != 6 || !known || target.interfaceRef || target.owner != "java/lang/invoke/LambdaMetafactory" || target.name != "altMetafactory" || target.descriptor != "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;[Ljava/lang/Object;)Ljava/lang/invoke/CallSite;" {
		return tuple, false
	}
	args := method.BootstrapArguments
	if len(args) != 4 && len(args) != 5 {
		return tuple, false
	}
	flags, ok := serializableConstant(object, args[3]).(*ConstantIntegerInfo)
	if !ok || flags == nil || len(args) == 4 && flags.Value != 1 || len(args) == 5 && flags.Value != 5 {
		return tuple, false
	}
	if len(args) == 5 {
		bridges, ok := serializableConstant(object, args[4]).(*ConstantIntegerInfo)
		if !ok || bridges == nil || bridges.Value != 0 {
			return tuple, false
		}
	}
	impl, ok := serializableConstant(object, args[1]).(*ConstantMethodHandleInfo)
	tuple.implementation, known = nativeLambdaOriginalHandleTarget(object, impl)
	if !ok || impl == nil || impl.ReferenceKind != 6 || !known {
		return tuple, false
	}
	methodType := func(index uint16) (string, bool) {
		entry, ok := serializableConstant(object, index).(*ConstantMethodTypeInfo)
		if !ok || entry == nil {
			return "", false
		}
		return sourceBridgeUTF8(object, entry.DescriptorIndex)
	}
	tuple.sam, known = methodType(args[0])
	if !known {
		return tuple, false
	}
	tuple.instantiated, known = methodType(args[2])
	if !known || tuple.instantiated != tuple.implementation.descriptor {
		return tuple, false
	}
	nat, ok := serializableConstant(object, dynamic.NameAndTypeIndex).(*ConstantNameAndTypeInfo)
	if !ok || nat == nil {
		return tuple, false
	}
	tuple.name, known = sourceBridgeUTF8(object, nat.NameIndex)
	if !known {
		return tuple, false
	}
	tuple.descriptor, known = sourceBridgeUTF8(object, nat.DescriptorIndex)
	inputs, result, err := callbinding.Descriptor(tuple.descriptor)
	if !known || err != nil || len(inputs) != 0 || len(result) < 3 || result[0] != 'L' || result[len(result)-1] != ';' {
		return tuple, false
	}
	tuple.functional = result[1 : len(result)-1]
	samInputs, _, samErr := callbinding.Descriptor(tuple.sam)
	actualInputs, _, actualErr := callbinding.Descriptor(tuple.instantiated)
	return tuple, samErr == nil && actualErr == nil && len(samInputs) == len(actualInputs)
}

func serializableOriginalCode(object *ClassObject, method *MemberInfo, work *workbudget.Budget) (*CodeAttribute, []*core.OpCode, bool) {
	var code *CodeAttribute
	for _, attribute := range method.Attributes {
		if c, ok := attribute.(*CodeAttribute); ok {
			if c == nil || code != nil {
				return nil, nil, false
			}
			code = c
		}
	}
	if code == nil {
		return nil, nil, true
	}
	if len(code.Code) > 65535 || !nativeProofWork(work, int64(len(code.Code))+1) || work != nil && work.CheckAlloc(int64(len(code.Code))*128+256) != nil {
		return nil, nil, false
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return nil, nil, false
	}
	ops := decoder.Opcodes()
	if len(ops) > 0 && nativeEnumOpcode(ops[0], core.OP_START) {
		ops = ops[1:]
	}
	return code, ops, true
}

func buildSerializableReferenceProtocol(object *ClassObject, work *workbudget.Budget) *serializableReferenceProtocol {
	if object == nil || object.MajorVersion != 52 || object.MinorVersion != 0 || len(object.Methods) > 4096 || len(object.ConstantPool) > 65535 || !nativeProofWork(work, int64(len(object.ConstantPool)+len(object.Methods))+1) || work != nil && work.CheckAlloc(int64(len(object.Methods))*128+int64(len(object.ConstantPool))*32) != nil {
		return nil
	}
	var bootstrap *BootstrapMethodsAttribute
	for _, attribute := range object.Attributes {
		if b, ok := attribute.(*BootstrapMethodsAttribute); ok {
			if b == nil || bootstrap != nil || int(b.NumBootstrapMethods) != len(b.BootstrapMethods) {
				return nil
			}
			bootstrap = b
		}
	}
	if bootstrap == nil {
		return nil
	}
	owner, known := sourceBridgeClassName(object, object.ThisClass)
	if !known {
		return nil
	}
	plan := &serializableReferenceProtocol{object: object, owner: owner}
	var helperOps []*core.OpCode
	var helperCode *CodeAttribute
	names := map[string]serializableReferenceTuple{}
	occurrences := map[string]int{}
	physicalNames := map[string]bool{}
	bytes := 0
	for _, method := range object.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return nil
		}
		name, named := sourceBridgeUTF8(object, method.NameIndex)
		descriptor, typed := sourceBridgeUTF8(object, method.DescriptorIndex)
		if !named || !typed || physicalNames[name+descriptor] {
			return nil
		}
		physicalNames[name+descriptor] = true
		code, ops, known := serializableOriginalCode(object, method, work)
		if !known {
			return nil
		}
		if code == nil {
			continue
		}
		bytes += len(code.Code)
		if bytes > 1<<20 {
			return nil
		}
		if name == "$deserializeLambda$" {
			if plan.helper != nil || descriptor != serializedLambdaDescriptor || method.AccessFlags != 0x100a || len(method.Attributes) != 1 || len(code.ExceptionTable) != 0 || code.MaxLocals != 3 || code.MaxStack != 3 || len(ops) > 4096 {
				return nil
			}
			for _, attribute := range code.Attributes {
				switch a := attribute.(type) {
				case *LineNumberTableAttribute:
					if a == nil {
						return nil
					}
				case *UnparsedAttribute:
					if a == nil || a.Length != uint32(len(a.Info)) || (a.Name != "StackMapTable" && a.Name != "LocalVariableTable") {
						return nil
					}
				default:
					return nil
				}
			}
			plan.helper, helperOps, helperCode = method, ops, code
			continue
		}
		for _, op := range ops {
			if !nativeProofWork(work, 1) {
				return nil
			}
			if !nativeEnumOpcode(op, core.OP_INVOKEDYNAMIC) {
				continue
			}
			if len(op.Data) != 4 || op.Data[2] != 0 || op.Data[3] != 0 {
				return nil
			}
			index := binary.BigEndian.Uint16(op.Data)
			tuple, known := serializableReferenceTupleAt(object, bootstrap, index)
			if !known {
				continue
			}
			if len(plan.sites) >= 256 {
				return nil
			}
			impl := tuple.implementation.name
			if prior, exists := names[impl]; exists && prior != tuple {
				return nil
			}
			names[impl] = tuple
			occurrences[impl]++
			plan.sites = append(plan.sites, serializableReferenceSite{method: method, code: code, pc: int(op.CurrentOffset), tuple: tuple})
		}
	}
	if plan.helper == nil || len(names) == 0 || len(names) > 32 || !serializableDeserializerProtocol(object, helperCode, helperOps, names, occurrences, bootstrap, work) {
		return nil
	}
	return plan
}

// The verifier reads the original decision automaton, not a class name or a
// source comment. Hash buckets may collide and case ordinals are private
// temporaries. Every successful branch must check the same ordered tuple and
// return its exact original factory; every other branch must reach the same
// failure constructor. No unvisited executable instruction is permitted.
func serializableDeserializerProtocol(object *ClassObject, code *CodeAttribute, ops []*core.OpCode, names map[string]serializableReferenceTuple, occurrences map[string]int, bootstrap *BootstrapMethodsAttribute, work *workbudget.Budget) bool {
	proof := &serializableProtocolCursor{object: object, code: code, ops: ops, work: work, used: map[int]bool{}, offsets: map[int]int{}}
	for i, op := range ops {
		if op == nil || op.Instr == nil {
			return false
		}
		proof.offsets[int(op.CurrentOffset)] = i
	}
	p := 0
	if !proof.take(&p, core.OP_ALOAD_0) || !proof.member(&p, core.OP_INVOKEVIRTUAL, "java/lang/invoke/SerializedLambda", "getImplMethodName", "()Ljava/lang/String;") || !proof.take(&p, core.OP_ASTORE_1) || !proof.integer(&p, -1) || !proof.take(&p, core.OP_ISTORE_2) || !proof.take(&p, core.OP_ALOAD_1) || !proof.member(&p, core.OP_INVOKEVIRTUAL, "java/lang/String", "hashCode", "()I") {
		return false
	}
	hashes, stage, valid := proof.switchAt(&p)
	if !valid {
		return false
	}
	q := stage
	if !proof.take(&q, core.OP_ILOAD_2) {
		return false
	}
	cases, failure, valid := proof.switchAt(&q)
	if !valid || len(cases) != len(names) {
		return false
	}
	buckets := map[int]map[string]bool{}
	for name := range names {
		if !utf8.ValidString(name) || !nativeProofWork(work, int64(len(name))+1) || work != nil && work.CheckAlloc(int64(len(name))*4+128) != nil {
			return false
		}
		var hash uint32
		for _, unit := range utf16.Encode([]rune(name)) {
			hash = hash*31 + uint32(unit)
		}
		key := int(int32(hash))
		if buckets[key] == nil {
			buckets[key] = map[string]bool{}
		}
		buckets[key][name] = true
	}
	if len(buckets) != len(hashes) {
		return false
	}
	ordinals := map[string]int{}
	seenOrdinals := map[int]bool{}
	for hash, bucket := range buckets {
		p, exists := hashes[hash]
		if !exists {
			return false
		}
		seen := map[string]bool{}
		for p != stage {
			if len(seen) >= len(bucket) || !proof.take(&p, core.OP_ALOAD_1) {
				return false
			}
			name, known := proof.text(&p)
			if !known || !bucket[name] || seen[name] || !proof.member(&p, core.OP_INVOKEVIRTUAL, "java/lang/String", "equals", "(Ljava/lang/Object;)Z") {
				return false
			}
			negative, known := proof.branch(&p, core.OP_IFEQ)
			if !known {
				return false
			}
			ordinal, known := proof.anyInteger(&p)
			if !known || ordinal == -1 || seenOrdinals[ordinal] || !proof.take(&p, core.OP_ISTORE_2) {
				return false
			}
			if p != stage {
				target, known := proof.branch(&p, core.OP_GOTO)
				if !known || target != stage {
					return false
				}
			}
			ordinals[name], seen[name], seenOrdinals[ordinal] = ordinal, true, true
			p = negative
		}
		if len(seen) != len(bucket) {
			return false
		}
	}
	for name, tuple := range names {
		p, exists := cases[ordinals[name]]
		if !exists || occurrences[name] < 1 || occurrences[name] > 256 {
			return false
		}
		// javac retains one checked return route for each source factory,
		// including repeated references to the same implementation. Its next
		// candidate is the negative edge of every guard in the current route.
		// Require the original multiplicity and exact tuple on every route;
		// neither a class name nor redundant-looking code can skip a check.
		for occurrence := 0; occurrence < occurrences[name]; occurrence++ {
			if !proof.take(&p, core.OP_ALOAD_0) || !proof.member(&p, core.OP_INVOKEVIRTUAL, "java/lang/invoke/SerializedLambda", "getImplMethodKind", "()I") || !proof.integer(&p, 6) {
				return false
			}
			next, known := proof.branch(&p, core.OP_IF_ICMPNE)
			if !known {
				return false
			}
			for _, field := range [][2]string{{"getFunctionalInterfaceClass", tuple.functional}, {"getFunctionalInterfaceMethodName", tuple.name}, {"getFunctionalInterfaceMethodSignature", tuple.sam}, {"getImplClass", tuple.implementation.owner}, {"getImplMethodSignature", tuple.implementation.descriptor}} {
				if !proof.take(&p, core.OP_ALOAD_0) || !proof.member(&p, core.OP_INVOKEVIRTUAL, "java/lang/invoke/SerializedLambda", field[0], "()Ljava/lang/String;") {
					return false
				}
				text, known := proof.text(&p)
				if !known || text != field[1] || !proof.member(&p, core.OP_INVOKEVIRTUAL, "java/lang/Object", "equals", "(Ljava/lang/Object;)Z") {
					return false
				}
				if target, known := proof.branch(&p, core.OP_IFEQ); !known || target != next {
					return false
				}
			}
			if p >= len(ops) || !nativeEnumOpcode(ops[p], core.OP_INVOKEDYNAMIC) || len(ops[p].Data) != 4 || ops[p].Data[2] != 0 || ops[p].Data[3] != 0 {
				return false
			}
			actual, known := serializableReferenceTupleAt(object, bootstrap, binary.BigEndian.Uint16(ops[p].Data))
			if !known || actual != tuple || !proof.take(&p, core.OP_INVOKEDYNAMIC) || !proof.take(&p, core.OP_ARETURN) {
				return false
			}
			if occurrence+1 == occurrences[name] {
				if next != failure {
					return false
				}
			} else if next != p {
				return false
			}
		}
	}
	p = failure
	if p >= len(ops) || !nativeEnumOpcode(ops[p], core.OP_NEW) || !nativeEnumClassOperand(object, ops[p], "java/lang/IllegalArgumentException") || !proof.take(&p, core.OP_NEW) || !proof.take(&p, core.OP_DUP) {
		return false
	}
	text, known := proof.text(&p)
	return known && text == "Invalid lambda deserialization" && proof.member(&p, core.OP_INVOKESPECIAL, "java/lang/IllegalArgumentException", "<init>", "(Ljava/lang/String;)V") && proof.take(&p, core.OP_ATHROW) && len(proof.used) == len(ops)
}

type serializableProtocolCursor struct {
	object  *ClassObject
	code    *CodeAttribute
	ops     []*core.OpCode
	work    *workbudget.Budget
	used    map[int]bool
	offsets map[int]int
}

func (c *serializableProtocolCursor) take(p *int, opcode int) bool {
	if *p < 0 || *p >= len(c.ops) || c.used[*p] || !nativeEnumOpcode(c.ops[*p], opcode) || !nativeProofWork(c.work, 1) {
		return false
	}
	c.used[*p] = true
	*p++
	return true
}
func (c *serializableProtocolCursor) member(p *int, opcode int, owner, name, descriptor string) bool {
	return *p >= 0 && *p < len(c.ops) && nativeEnumMemberOperand(c.object, c.ops[*p], opcode, owner, name, descriptor) && c.take(p, opcode)
}
func (c *serializableProtocolCursor) anyInteger(p *int) (int, bool) {
	if *p < 0 || *p >= len(c.ops) {
		return 0, false
	}
	op := c.ops[*p]
	n, known := nativeEnumOriginalInt(c.object, op)
	return n, known && c.take(p, op.Instr.OpCode)
}
func (c *serializableProtocolCursor) integer(p *int, want int) bool {
	n, known := c.anyInteger(p)
	return known && n == want
}
func (c *serializableProtocolCursor) text(p *int) (string, bool) {
	if *p < 0 || *p >= len(c.ops) {
		return "", false
	}
	op := c.ops[*p]
	if !nativeEnumOpcode(op, core.OP_LDC) && !nativeEnumOpcode(op, core.OP_LDC_W) {
		return "", false
	}
	index, known := nativeEnumCPIndex(op)
	entry, valid := serializableConstant(c.object, index).(*ConstantStringInfo)
	if !known || !valid || entry == nil {
		return "", false
	}
	text, known := sourceBridgeUTF8(c.object, entry.StringIndex)
	return text, known && c.take(p, op.Instr.OpCode)
}
func (c *serializableProtocolCursor) branch(p *int, opcode int) (int, bool) {
	if *p < 0 || *p >= len(c.ops) {
		return 0, false
	}
	op := c.ops[*p]
	pc, err := core.BranchTarget(int(op.CurrentOffset), op.Data, 2, len(c.code.Code))
	index, known := c.offsets[pc]
	return index, err == nil && known && c.take(p, opcode)
}
func (c *serializableProtocolCursor) switchAt(p *int) (map[int]int, int, bool) {
	if *p < 0 || *p >= len(c.ops) {
		return nil, 0, false
	}
	op := c.ops[*p]
	if !nativeEnumOpcode(op, core.OP_TABLESWITCH) && !nativeEnumOpcode(op, core.OP_LOOKUPSWITCH) || op.SwitchJmpCase == nil {
		return nil, 0, false
	}
	failure, known := c.offsets[int(op.SwitchDefaultOffset)]
	if !known || !c.take(p, op.Instr.OpCode) {
		return nil, 0, false
	}
	result := map[int]int{}
	valid := true
	op.SwitchJmpCase.ForEach(func(key int, pc int32) bool {
		index, known := c.offsets[int(pc)]
		if !known || !nativeProofWork(c.work, 1) {
			valid = false
			return false
		}
		if index != failure {
			result[key] = index
		}
		return true
	})
	return result, failure, valid
}
