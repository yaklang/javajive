package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// This proves only that an original implementation declaration has exclusively
// factory uses. It grants no capture, invocation, access, lexical scope or
// source-publication permission. Native scopes retain their separate producer
// and ownership proofs. In particular, a request role or ACC_SYNTHETIC alone
// cannot authorize dropping an observable original declaration.
func nativeLambdaBodyOriginalUseClosed(object *ClassObject, implementation *MemberInfo, work *workbudget.Budget, metadata ...*lambdaParameterMetadata) bool {
	if len(metadata) > 1 {
		return false
	}
	var parameters *lambdaParameterMetadata
	if len(metadata) == 1 {
		parameters = metadata[0]
		if parameters == nil || !parameters.schemaMatches(object, implementation, work) {
			return false
		}
	}
	if object == nil || implementation == nil || len(object.Methods) > 4096 || len(object.ConstantPool) > 65535 || implementation.AccessFlags&^uint16(0x100a) != 0 || implementation.AccessFlags&0x1002 != 0x1002 || !nativeProofWork(work, int64(len(object.Methods)+len(object.ConstantPool))) {
		return false
	}
	name, named := sourceBridgeUTF8(object, implementation.NameIndex)
	descriptor, typed := sourceBridgeUTF8(object, implementation.DescriptorIndex)
	if !named || !typed || name == "<init>" || name == "<clinit>" {
		return false
	}
	for _, attribute := range implementation.Attributes {
		switch attribute.(type) {
		case *CodeAttribute, *ExceptionsAttribute:
		case *UnparsedAttribute:
			if parameters == nil || attribute != parameters.table {
				return false
			}
		default:
			// Generic signatures, annotations and unknown implementation metadata
			// have no regeneration receipt in this body-source domain.
			return false
		}
	}
	bytes, declarations, implementationCodes := 0, 0, 0
	for _, method := range object.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return false
		}
		n, nk := sourceBridgeUTF8(object, method.NameIndex)
		d, dk := sourceBridgeUTF8(object, method.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		if n == name && d == descriptor {
			if method != implementation {
				return false
			}
			declarations++
		}
		codes := 0
		for _, attribute := range method.Attributes {
			if raw, ok := attribute.(*UnparsedAttribute); ok && (raw == nil || raw.Name == "Code") {
				return false
			}
			if code, ok := attribute.(*CodeAttribute); ok {
				if code == nil || len(code.Code) > (1<<20)-bytes {
					return false
				}
				codes++
				bytes += len(code.Code)
			}
		}
		if codes > 1 {
			return false
		}
		if method == implementation {
			implementationCodes = codes
		}
	}
	if declarations != 1 || implementationCodes != 1 || work != nil && work.CheckAlloc(int64(bytes)*32+int64(len(object.ConstantPool))*32) != nil {
		return false
	}
	constant := func(index uint16) ConstantInfo {
		if index == 0 || int(index) > len(object.ConstantPool) {
			return nil
		}
		return object.ConstantPool[index-1]
	}
	handles := map[uint16]bool{}
	textBytes := 0
	for index, item := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false
		}
		if text, ok := item.(*ConstantUtf8Info); ok {
			if text == nil || len(text.Value) > (8<<20)-textBytes {
				return false
			}
			textBytes += len(text.Value)
			// This inventory reads the constant's kind and O(1) string
			// length; it never scans the unused UTF8 payload. Keep the
			// aggregate payload cap, and charge the actual CP-entry visit
			// above. Named packets separately hash every consumed CP byte.
		}
		handle, ok := item.(*ConstantMethodHandleInfo)
		if !ok {
			continue
		}
		if handle == nil {
			return false
		}
		target, valid := nativeLambdaOriginalHandleTarget(object, handle)
		if valid && target.owner == object.GetClassName() && target.name == name && target.descriptor == descriptor {
			if !nativeLambdaImplementationHandle(object, implementation, handle, target) {
				return false
			}
			handles[uint16(index+1)] = true
		}
	}
	if len(handles) != 1 {
		return false
	}
	var bootstrap *BootstrapMethodsAttribute
	for _, attribute := range object.Attributes {
		if table, ok := attribute.(*BootstrapMethodsAttribute); ok {
			if table == nil || bootstrap != nil {
				return false
			}
			bootstrap = table
		}
	}
	if bootstrap == nil || len(bootstrap.BootstrapMethods) > 4096 || int(bootstrap.NumBootstrapMethods) != len(bootstrap.BootstrapMethods) {
		return false
	}
	allowed := map[int]bool{}
	arguments := 0
	for index, row := range bootstrap.BootstrapMethods {
		if row == nil || len(row.BootstrapArguments) > 65535-arguments || int(row.NumBootstrapArguments) != len(row.BootstrapArguments) || handles[row.BootstrapMethodRef] || !nativeProofWork(work, int64(len(row.BootstrapArguments)+1)) {
			return false
		}
		arguments += len(row.BootstrapArguments)
		uses := 0
		for ordinal, arg := range row.BootstrapArguments {
			if handles[arg] {
				if ordinal != 1 {
					return false
				}
				uses++
			}
		}
		if uses == 0 {
			continue
		}
		if uses != 1 || len(allowed) >= 64 || len(row.BootstrapArguments) < 3 {
			return false
		}
		handle, ok := constant(row.BootstrapMethodRef).(*ConstantMethodHandleInfo)
		if !ok || handle == nil || handle.ReferenceKind != 6 {
			return false
		}
		target, valid := nativeLambdaOriginalHandleTarget(object, handle)
		if !valid || target.interfaceRef || target.owner != "java/lang/invoke/LambdaMetafactory" {
			return false
		}
		n, d := target.name, target.descriptor
		if n == "metafactory" {
			if len(row.BootstrapArguments) != 3 || d != "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodHandle;Ljava/lang/invoke/MethodType;)Ljava/lang/invoke/CallSite;" {
				return false
			}
		} else if n == "altMetafactory" {
			if d != "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;[Ljava/lang/Object;)Ljava/lang/invoke/CallSite;" || !nativeLambdaBodyMarkerArguments(row.BootstrapArguments, constant) {
				return false
			}
		} else {
			return false
		}
		for _, ordinal := range []int{0, 2} {
			kind, ok := constant(row.BootstrapArguments[ordinal]).(*ConstantMethodTypeInfo)
			if !ok || kind == nil {
				return false
			}
			if _, valid := sourceBridgeUTF8(object, kind.DescriptorIndex); !valid {
				return false
			}
		}
		allowed[index] = true
	}
	if len(allowed) == 0 {
		return false
	}
	for _, item := range object.ConstantPool {
		if dynamic, ok := item.(*ConstantDynamicInfo); ok && (dynamic == nil || allowed[int(dynamic.BootstrapMethodAttrIndex)]) {
			return false
		}
	}
	sites := 0
	usedRows := map[int]bool{}
	var factory *lambdaSourceFactory
	for _, method := range object.Methods {
		for _, attribute := range method.Attributes {
			code, ok := attribute.(*CodeAttribute)
			if !ok {
				continue
			}
			if !nativeProofWork(work, int64(len(code.Code))) {
				return false
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
			decoder.Work = work
			if decoder.ParseOpcode() != nil {
				return false
			}
			for _, op := range constructorMotionOps(decoder) {
				if !nativeProofWork(work, 1) || !nativeLambdaExclusiveOpcodeUse(object, object.GetClassName(), name, descriptor, handles, op) {
					return false
				}
				if method == implementation && parameters != nil && core.LocalAccessOf(op.Instr.OpCode).Write {
					// A fixed SAM binder is not later local-slot storage. Keep this
					// first domain to unchanged entry words, including both wide words.
					if slot := core.GetStoreIdx(op); slot < 0 || slot < parameters.words {
						return false
					}
				}
				if op.Instr.OpCode != core.OP_INVOKEDYNAMIC {
					continue
				}
				if len(op.Data) != 4 || op.Data[2] != 0 || op.Data[3] != 0 {
					return false
				}
				dynamic, ok := constant(core.Convert2bytesToInt(op.Data[:2])).(*ConstantInvokeDynamicInfo)
				if !ok || dynamic == nil {
					return false
				}
				row := int(dynamic.BootstrapMethodAttrIndex)
				if !allowed[row] {
					continue
				}
				if sites >= 64 {
					return false
				}
				if parameters != nil {
					origin := &lambdaSourceFactory{object: object, method: method, code: code, pc: int(op.CurrentOffset)}
					_, named := lambdaFactoryNameComponent(origin)
					nt, typed := constant(dynamic.NameAndTypeIndex).(*ConstantNameAndTypeInfo)
					// A nested factory may reside in an actual synthetic body.
					// This use query grants no lexical permission: its caller must
					// close that body's own factory chain and source registration.
					if factory != nil || !named || method.AccessFlags&(0x0040|0x0100|0x0400) != 0 || method.AccessFlags&0x1000 != 0 && method.AccessFlags != 0x100a && method.AccessFlags != 0x1002 || !typed || nt == nil {
						return false
					}
					factoryDescriptor, known := sourceBridgeUTF8(object, nt.DescriptorIndex)
					captures, result, err := callbinding.Descriptor(factoryDescriptor)
					if !known || err != nil || len(captures) > 64 || len(result) < 3 || result[0] != 'L' || result[len(result)-1] != ';' {
						return false
					}
					if parameters.receiverWords != 0 {
						if len(captures) == 0 || captures[0] != "L"+object.GetClassName()+";" || method.AccessFlags&8 != 0 {
							return false
						}
						captures = captures[1:]
					}
					parameters.captureTypes = captures
					factory = origin
				}
				usedRows[row] = true
				sites++
			}
		}
	}
	closed := sites != 0 && len(usedRows) == len(allowed)
	if closed && parameters != nil {
		if factory == nil || len(factory.code.Code) > 65535 || work != nil && work.CheckAlloc(int64(len(factory.code.Code))) != nil {
			return false
		}
		if !factory.seal(work) {
			return false
		}
		parameters.factory = factory
	}
	return closed
}

// altMetafactory's existing marker-only source domain. Serializable lambdas and
// explicit bridges retain the core's refusal; this is no new lambda capability.
func nativeLambdaBodyMarkerArguments(args []uint16, constant func(uint16) ConstantInfo) bool {
	integer := func(index int) (int, bool) {
		if index >= len(args) {
			return 0, false
		}
		value, ok := constant(args[index]).(*ConstantIntegerInfo)
		if !ok || value == nil {
			return 0, false
		}
		return int(value.Value), true
	}
	flags, valid := integer(3)
	if !valid || flags&^6 != 0 {
		return false
	}
	index := 4
	if flags&2 != 0 {
		count, valid := integer(index)
		index++
		if !valid || count < 0 || count > 64 || count > len(args)-index {
			return false
		}
		for end := index + count; index < end; index++ {
			if value, ok := constant(args[index]).(*ConstantClassInfo); !ok || value == nil {
				return false
			}
		}
	}
	if flags&4 != 0 {
		count, valid := integer(index)
		if !valid || count != 0 {
			return false
		}
		index++
	}
	return index == len(args)
}

// Shared by the original native scope proof and the source-body declaration
// proof. Inspect every original opcode, including unreachable uses.
func nativeLambdaExclusiveOpcodeUse(object *ClassObject, owner, name, descriptor string, handles map[uint16]bool, op *core.OpCode) bool {
	if object == nil || op == nil || op.Instr == nil {
		return false
	}
	switch op.Instr.OpCode {
	case core.OP_LDC, core.OP_LDC_W, core.OP_LDC2_W:
		var index uint16
		if len(op.Data) == 1 {
			index = uint16(op.Data[0])
		} else if len(op.Data) == 2 {
			index = core.Convert2bytesToInt(op.Data)
		} else {
			return false
		}
		return !handles[index]
	case core.OP_INVOKESTATIC, core.OP_INVOKEVIRTUAL, core.OP_INVOKEINTERFACE, core.OP_INVOKESPECIAL:
		ref := constructorMotionMember(object, op, op.Instr.OpCode)
		return ref != nil && (ref.Name != owner || ref.Member != name || ref.Description != descriptor)
	}
	return true
}
