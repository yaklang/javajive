package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A call on THIS is ordinarily an observation/publication boundary. Admit only
// an exact own declaration with proved closed dispatch whose original body is an
// bounded return or one nonvolatile field/literal/parameter read and return. There is no
// virtual override, allocation, receiver publication or throwable computation.
// The caller still needs a closed-finalizer proof: method entry can fail, e.g.
// with StackOverflowError, even when its entire body has no throwing opcode.
// No method name participates in this proof.
func (c *ClassObjectDumper) constructorReceiverReadOnlyMethod(obj *ClassObject, member *values.JavaClassMember, opcode int, writes map[string]bool, remaining *int, arguments ...constructorEffectValue) (constructorEffectValue, bool) {
	return c.constructorReceiverReadOnlyMethodWithStorage(obj, member, opcode, writes, remaining, nil, arguments...)
}

// Alias evidence follows the operation which reads the original receiver heap,
// not the result's computational type. An independent parameter/null return
// cannot retrieve a self-reference retained in THIS; a reference GETFIELD can.
// The field read is recorded only after its complete declaration/body proof.
func (c *ClassObjectDumper) constructorReceiverReadOnlyMethodWithStorage(obj *ClassObject, member *values.JavaClassMember, opcode int, writes map[string]bool, remaining *int, storage *constructorSelfStorageProof, arguments ...constructorEffectValue) (constructorEffectValue, bool) {
	binding, known := c.constructorReceiverBindMethod(obj, member, opcode, remaining, arguments...)
	if !known {
		return constructorEffectValue{}, false
	}
	return c.constructorReceiverReadOnlyBoundMethod(obj, binding, writes, remaining, storage, arguments...)
}

type constructorReceiverMethodBinding struct {
	parameters []string
	result     string
	target     *MemberInfo
	code       *CodeAttribute
}

func (c *ClassObjectDumper) constructorReceiverBindMethod(obj *ClassObject, member *values.JavaClassMember, opcode int, remaining *int, arguments ...constructorEffectValue) (*constructorReceiverMethodBinding, bool) {
	if c == nil || remaining == nil || obj == nil || member == nil || member.Name != obj.GetClassName() || obj.AccessFlags&0x0200 != 0 || opcode != core.OP_INVOKEVIRTUAL && opcode != core.OP_INVOKESPECIAL {
		return nil, false
	}
	params, result, err := callbinding.Descriptor(member.Description)
	if err != nil || len(params) != len(arguments) || nativeMemberParameterWidth(params) > 254 || !nativeProofWork(c.Work, int64(len(params))) {
		return nil, false
	}
	for i, parameter := range params {
		if arguments[i].kind != constructorEffectType(parameter).kind || arguments[i].receiver || arguments[i].allocation != 0 {
			return nil, false
		}
	}
	target, found := c.constructorReceiverOwnMethod(obj, member.Member, member.Description, remaining)
	if !found || target == nil || target.AccessFlags&(0x0008|0x0020|0x0100|0x0400) != 0 || !c.constructorReceiverMethodDispatchClosed(obj, target, member, opcode, remaining) {
		return nil, false
	}
	var code *CodeAttribute
	for _, attribute := range target.Attributes {
		if candidate, ok := attribute.(*CodeAttribute); ok {
			if candidate == nil || code != nil {
				return nil, false
			}
			code = candidate
		}
	}
	if code == nil || int(code.MaxLocals) < nativeMemberParameterWidth(params)+1 {
		return nil, false
	}
	return &constructorReceiverMethodBinding{params, result, target, code}, true
}

// The readonly and general effect languages share one exact binding. Reuse that
// binding only inside this synchronous attempt, on a fresh owned original and
// with no intervening external or root-table observation. Otherwise the general
// proof repeats its original binding, including mutable root dispatch queries.
func (c *ClassObjectDumper) constructorReceiverMethodWithStorage(obj *ClassObject, member *values.JavaClassMember, opcode int, writes, active map[string]bool, remaining *int, depth int, aliases *constructorSelfStorageProof, arguments ...constructorEffectValue) (constructorEffectValue, bool) {
	binding, known := c.constructorReceiverBindMethod(obj, member, opcode, remaining, arguments...)
	if !known {
		return constructorEffectValue{}, false
	}
	e := c.constructorProfileEvidence
	var epoch uint64
	if e != nil {
		epoch = e.observationEpoch
	}
	value, accepted := c.constructorReceiverReadOnlyBoundMethod(obj, binding, writes, remaining, aliases, arguments...)
	if accepted {
		return value, true
	}
	if e == nil || !e.inBody || !e.eligible || e.inconsistent || obj == c.obj || !e.parsedOriginals[obj] || e.observationEpoch != epoch {
		binding = nil
	}
	return c.constructorReceiverClosedMethodWithBinding(obj, member, opcode, writes, active, remaining, depth, aliases, binding, arguments...)
}

func (c *ClassObjectDumper) constructorReceiverReadOnlyBoundMethod(obj *ClassObject, binding *constructorReceiverMethodBinding, writes map[string]bool, remaining *int, storage *constructorSelfStorageProof, arguments ...constructorEffectValue) (constructorEffectValue, bool) {
	params, result, code := binding.parameters, binding.result, binding.code
	if len(code.ExceptionTable) != 0 || len(code.Code) > 16 {
		return constructorEffectValue{}, false
	}
	if result == "V" {
		return constructorEffectValue{}, constructorReadOnlyVoidBody(code.Code, remaining, c.Work)
	}
	if !nativeProofWork(c.Work, int64(len(code.Code))) {
		return constructorEffectValue{}, false
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return constructorEffectValue{}, false
	}
	ops := constructorMotionOps(decoder)
	*remaining -= len(ops)
	if *remaining < 0 {
		return constructorEffectValue{}, false
	}
	value := constructorEffectType(result)
	returnOpcode := map[byte]int{'I': core.OP_IRETURN, 'J': core.OP_LRETURN, 'F': core.OP_FRETURN, 'D': core.OP_DRETURN, 'L': core.OP_ARETURN}[value.kind]
	if len(ops) < 2 || returnOpcode == 0 || ops[len(ops)-1].Instr.OpCode != returnOpcode || len(ops[len(ops)-1].Data) != 0 || int(code.MaxStack) < value.width() {
		return constructorEffectValue{}, false
	}
	if len(ops) == 2 {
		// Bind a physical parameter slot, not its source name or logical index.
		// The caller proved each actual value receiver-free before method entry;
		// returning that same value cannot observe or publish the fresh THIS.
		// Category-2 second words and ALOAD_0 never acquire this certificate.
		slot := 1
		for i, parameter := range params {
			if core.GetRetrieveIdx(ops[0]) == slot && constructorMotionLoad(ops[0], parameter) && arguments[i].kind == value.kind {
				returned := arguments[i]
				// JVMS 6.5 IRETURN narrows B/C/S and masks Z at the return
				// boundary, even without an I2B/I2C/I2S in the original body.
				// Carrying the input word unchanged can hide a later receiver
				// publication by pruning the wrong constructor branch.
				if returned.knownInt {
					switch result {
					case "B":
						returned.intWord = int32(int8(returned.intWord))
					case "C":
						returned.intWord = int32(uint16(returned.intWord))
					case "S":
						returned.intWord = int32(int16(returned.intWord))
					case "Z":
						returned.intWord &= 1
					}
				}
				return returned, true
			}
			slot += arguments[i].width()
		}
		descriptor, known := constructorMotionLiteral(obj, ops[0])
		if !known {
			return constructorEffectValue{}, false
		}
		if descriptor == "null" {
			return value, value.kind == 'L'
		}
		// String/class literals may resolve or allocate; they are not inert reads.
		literal := constructorEffectType(descriptor)
		return value, literal.kind != 'L' && literal.kind == value.kind
	}
	if len(ops) != 3 || core.GetRetrieveIdx(ops[0]) != 0 || !constructorMotionLoad(ops[0], "Ljava/lang/Object;") {
		return constructorEffectValue{}, false
	}
	field := constructorMotionMember(obj, ops[1], core.OP_GETFIELD)
	if field == nil || constructorEffectType(field.Description).kind != value.kind {
		return constructorEffectValue{}, false
	}
	identity, known := c.constructorEffectField(obj, field, false, remaining)
	accepted := known && !writes[identity]
	if accepted && value.kind == 'L' && storage != nil {
		storage.referenceRead = true
	}
	return value, accepted
}

// The readonly void language is NOP* RETURN NOP*, with the same 16-byte
// bound as the general readonly proof. Its two one-byte opcodes need no
// constant-pool or operand decoding. Reject the first forbidden instruction
// rather than decoding an entire mutating/calling body before rejecting it.
// NOPs remain transparent to the shared instruction cap, as in motionOps;
// every inspected byte still charges the request's native work budget.
func constructorReadOnlyVoidBody(code []byte, remaining *int, work *workbudget.Budget) bool {
	if remaining == nil || len(code) > 16 {
		return false
	}
	returned := false
	for _, opcode := range code {
		if !nativeProofWork(work, 1) {
			return false
		}
		if int(opcode) == core.OP_NOP {
			continue
		}
		*remaining--
		if *remaining < 0 {
			return false
		}
		switch int(opcode) {
		case core.OP_RETURN:
			if returned {
				return false
			}
			returned = true
		default:
			return false
		}
	}
	return returned
}
