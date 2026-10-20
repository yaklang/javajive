package javaclassparser

import (
	"bytes"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A varargs reference can have a compiler implementation even though no source
// lambda body exists. This packet proves only the original adapter's transfer:
// one bound receiver, unchanged fixed arguments, a fresh ordered argument pack,
// the selected invocation, and either its unchanged result or void discard.
// It grants no factory, source overload, accessibility or declaration-consumption
// permission. Those obligations must be proved at the actual factory site.
// JLS 15.13.1/15.13.3 and 15.12.4.2:
// https://docs.oracle.com/javase/specs/jls/se8/html/jls-15.html#jls-15.13.1
type lambdaVarargsAdapter struct {
	object     *ClassObject
	method     *MemberInfo
	code       *CodeAttribute
	codeBytes  []byte
	member     values.JavaClassMember
	opcode     int
	parameters []string
	result     string
	packCount  int
}

func (c *ClassObjectDumper) originalVarargsAdapterShape(method *MemberInfo) (*lambdaVarargsAdapter, bool) {
	if c == nil || c.obj == nil || method == nil || method.AccessFlags != 0x100a || len(c.obj.Methods) > 4096 || len(method.Attributes) > 64 {
		return nil, false
	}
	owned := 0
	for _, original := range c.obj.Methods {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if original == method {
			owned++
		}
	}
	descriptor, typed := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	if !typed || len(descriptor) > 65535 || !nativeProofWork(c.Work, int64(len(descriptor))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(descriptor))*32+256) != nil {
		return nil, false
	}
	parameters, result, err := callbinding.Descriptor(descriptor)
	if owned != 1 || !typed || err != nil || len(parameters) < 1 || len(parameters) > 255 || !strings.HasPrefix(parameters[0], "L") {
		return nil, false
	}
	var code *CodeAttribute
	for _, attribute := range method.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		switch a := attribute.(type) {
		case *CodeAttribute:
			if a == nil || code != nil {
				return nil, false
			}
			code = a
		case *UnparsedAttribute:
			if a == nil || a.Name != "MethodParameters" {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	if code == nil || len(code.Code) > 4096 || len(code.ExceptionTable) != 0 || !nativeProofWork(c.Work, int64(len(code.Code))+1) {
		return nil, false
	}
	// Read and validate the physical table without interpreting its entries as
	// source SAM formals. Compiler-role/name regeneration is a separate query.
	if _, known := readLambdaParameterMetadata(c.obj, method, c.Work); !known {
		return nil, false
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, i) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return nil, false
	}
	ops := constructorMotionOps(decoder)
	if len(ops) < 5 || len(ops) > 1024 {
		return nil, false
	}
	invoke := -1
	for i, op := range ops {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if op.Instr.OpCode == core.OP_INVOKEVIRTUAL || op.Instr.OpCode == core.OP_INVOKEINTERFACE {
			if invoke != -1 {
				return nil, false
			}
			invoke = i
		}
	}
	if invoke < 0 {
		return nil, false
	}
	member := constructorMotionMember(c.obj, ops[invoke], ops[invoke].Instr.OpCode)
	if member == nil || member.Member == "<init>" || member.Member == "<clinit>" || parameters[0] != "L"+member.Name+";" {
		return nil, false
	}
	formals, returned, err := callbinding.Descriptor(member.Description)
	if err != nil || len(formals) == 0 || !strings.HasPrefix(formals[len(formals)-1], "[") {
		return nil, false
	}
	if ops[invoke].Instr.OpCode == core.OP_INVOKEINTERFACE {
		words := 1
		for _, formal := range formals {
			words++
			if formal == "J" || formal == "D" {
				words++
			}
		}
		if words > 255 || int(ops[invoke].Data[2]) != words {
			return nil, false
		}
	}
	fixed := len(formals) - 1
	count := len(parameters) - 1 - fixed
	if count < 0 || count > 254 {
		return nil, false
	}
	component := formals[fixed][1:]
	slots := make([]int, len(parameters))
	word := 0
	for i, parameter := range parameters {
		slots[i] = word
		word++
		if parameter == "J" || parameter == "D" {
			word++
		}
	}
	index := 0
	load := func(parameter int, expected string) bool {
		if parameter >= len(parameters) || index >= invoke || parameters[parameter] != expected || core.GetRetrieveIdx(ops[index]) != slots[parameter] || !constructorMotionLoad(ops[index], expected) {
			return false
		}
		index++
		return true
	}
	literal := func(want int) bool {
		if index >= invoke {
			return false
		}
		value, known := constructorOriginalIntLiteral(c.obj, ops[index])
		if !known || !value.knownInt || value.intWord != int32(want) {
			return false
		}
		index++
		return true
	}
	if !load(0, parameters[0]) {
		return nil, false
	}
	for i := 0; i < fixed; i++ {
		if !load(i+1, formals[i]) {
			return nil, false
		}
	}
	if !literal(count) || index >= invoke || !lambdaVarargsArrayAllocation(c.obj, ops[index], component) {
		return nil, false
	}
	index++
	store, known := lambdaVarargsArrayStore(component)
	if !known {
		return nil, false
	}
	for i := 0; i < count; i++ {
		if index >= invoke || ops[index].Instr.OpCode != core.OP_DUP || len(ops[index].Data) != 0 {
			return nil, false
		}
		index++
		if !literal(i) || !load(fixed+1+i, component) || index >= invoke || ops[index].Instr.OpCode != store || len(ops[index].Data) != 0 {
			return nil, false
		}
		index++
	}
	if index != invoke {
		return nil, false
	}
	index++
	var resultClosed bool
	index, resultClosed = c.originalVarargsResultConversion(code, ops, index, returned, result)
	if !resultClosed {
		return nil, false
	}
	ret := core.OP_RETURN
	if result != "V" {
		switch result[0] {
		case 'L', '[':
			ret = core.OP_ARETURN
		case 'J':
			ret = core.OP_LRETURN
		case 'F':
			ret = core.OP_FRETURN
		case 'D':
			ret = core.OP_DRETURN
		default:
			ret = core.OP_IRETURN
		}
	}
	if index != len(ops)-1 || ops[index].Instr.OpCode != ret || len(ops[index].Data) != 0 {
		return nil, false
	}
	if _, _, known := c.nativeOriginalMethodSnapshot(method, code); !known || c.Work != nil && c.Work.CheckAlloc(int64(len(code.Code))+int64(len(parameters))*32+256) != nil {
		return nil, false
	}
	return &lambdaVarargsAdapter{object: c.obj, method: method, code: code, codeBytes: bytes.Clone(code.Code), member: *member, opcode: ops[invoke].Instr.OpCode, parameters: parameters, result: result, packCount: count}, true
}

func lambdaVarargsArrayAllocation(object *ClassObject, op *core.OpCode, component string) bool {
	if op == nil || op.Instr == nil || component == "" {
		return false
	}
	if op.Instr.OpCode == core.OP_ANEWARRAY && len(op.Data) == 2 {
		name, known := sourceBridgeClassName(object, core.Convert2bytesToInt(op.Data))
		if !known {
			return false
		}
		if !strings.HasPrefix(name, "[") {
			name = "L" + name + ";"
		}
		return component == name
	}
	if op.Instr.OpCode != core.OP_NEWARRAY || len(op.Data) != 1 || len(component) != 1 {
		return false
	}
	kinds := map[byte]string{4: "Z", 5: "C", 6: "F", 7: "D", 8: "B", 9: "S", 10: "I", 11: "J"}
	return kinds[op.Data[0]] == component
}

func lambdaVarargsArrayStore(component string) (int, bool) {
	if strings.HasPrefix(component, "L") || strings.HasPrefix(component, "[") {
		return core.OP_AASTORE, true
	}
	stores := map[string]int{"Z": core.OP_BASTORE, "B": core.OP_BASTORE, "C": core.OP_CASTORE, "S": core.OP_SASTORE, "I": core.OP_IASTORE, "J": core.OP_LASTORE, "F": core.OP_FASTORE, "D": core.OP_DASTORE}
	op, known := stores[component]
	return op, known
}
