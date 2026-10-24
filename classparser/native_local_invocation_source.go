package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A stable local can retain the selected producer's generic source view even
// when its computational word stays erased. This profile proves a zero-argument
// static call or a call on the original descriptor-seeded THIS. Other receivers,
// producer operands, joins and writes need their own def-use proofs. No source
// declaration, runtime call, cast or family state is changed by this query.
func (c *ClassObjectDumper) nativeSourceLocalInvocation(ctx *class_context.ClassContext, method *MemberInfo, code *CodeAttribute, body []statements.Statement, params []values.JavaValue, ref *values.JavaRef) (*values.FunctionCallExpression, bool) {
	if c == nil || c.obj == nil || ctx == nil || method == nil || code == nil || ref == nil || ref.Id == nil || strings.ReplaceAll(ctx.ClassName, ".", "/") != c.obj.GetClassName() || !nativeProofWork(c.Work, 1) {
		return nil, false
	}
	name, nk := sourceBridgeUTF8(c.obj, method.NameIndex)
	desc, dk := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	if !nk || !dk || name != ctx.FunctionName || desc != ctx.CurrentMethodDesc || (method.AccessFlags&8 != 0) != ctx.IsStatic {
		return nil, false
	}
	matches := 0
	for _, original := range c.obj.Methods {
		if original == nil || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		n, _ := sourceBridgeUTF8(c.obj, original.NameIndex)
		d, _ := sourceBridgeUTF8(c.obj, original.DescriptorIndex)
		if n == name && d == desc {
			if original != method {
				return nil, false
			}
			matches++
		}
	}
	if matches != 1 {
		return nil, false
	}
	bodies := 0
	for _, attribute := range method.Attributes {
		if actual, known := attribute.(*CodeAttribute); known {
			if actual != code {
				return nil, false
			}
			bodies++
		}
	}
	if bodies != 1 {
		return nil, false
	}
	declaration, stable := nativeCaptureDeclaration(body, ref, false, c.Work)
	if !stable || declaration == nil || !declaration.HasOriginPC {
		return nil, false
	}
	left, known := nativeMemberEnclosingUnpack(declaration.LeftValue, c.Work)
	declared, declaredRef := left.(*values.JavaRef)
	if !known || !declaredRef || declared == nil || declared != ref && !declared.SameOriginalLocalDeclaration(ref) {
		return nil, false
	}
	pc, slot, sealed := ref.OriginalLocalDeclaration(declaration.JavaValue)
	declPC, declSlot, declSealed := declared.OriginalLocalDeclaration(declaration.JavaValue)
	producer, known := nativeMemberEnclosingUnpack(declaration.JavaValue, c.Work)
	call, callable := producer.(*values.FunctionCallExpression)
	if !sealed || !declSealed || pc != declPC || slot != declSlot || pc != declaration.OriginPC || !known || !callable || call == nil || !call.HasOriginPC || len(call.Arguments) != 0 || call.IsSpecialInvoke || call.FunctionName == "<init>" {
		return nil, false
	}
	ir, fn, known := c.nativeOriginalMethodSnapshot(method, code)
	if !known {
		return nil, false
	}
	storeFound, callFound := false, false
	var uses []ssabuild.Origin
	for _, record := range fn.Instructions {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		ins, found := ir.InstrByID(methodir.InstrID(record.PC))
		if !found {
			return nil, false
		}
		if int(record.PC) == pc {
			if storeFound || !core.LocalAccessOf(ins.Opcode).Write || ins.Local != slot || len(record.Uses) != 1 || record.Uses[0].Kind != ssabuild.OriginInstr || int(record.Uses[0].PC) != call.OriginPC {
				return nil, false
			}
			storeFound = true
		}
		if int(record.PC) == call.OriginPC {
			kind := values.InvokeVirtual
			if ins.Opcode == core.OP_INVOKESTATIC {
				kind = values.InvokeStatic
			} else if ins.Opcode == core.OP_INVOKEINTERFACE {
				kind = values.InvokeInterface
			} else if ins.Opcode != core.OP_INVOKEVIRTUAL {
				return nil, false
			}
			if callFound || call.Kind != kind || call.IsStatic != (kind == values.InvokeStatic) || strings.ReplaceAll(call.ClassName, ".", "/") != ins.Class || call.FunctionName != ins.Member || call.Descriptor != ins.Desc {
				return nil, false
			}
			callFound, uses = true, record.Uses
		}
	}
	if !storeFound || !callFound {
		return nil, false
	}
	proof := call.Clone()
	if call.IsStatic {
		return proof, len(uses) == 0
	}
	receiver, known := nativeMemberEnclosingUnpack(call.Object, c.Work)
	this, knownRef := receiver.(*values.JavaRef)
	if !known || !knownRef || this == nil || this.Id == nil || len(uses) != 1 || uses[0].Kind != ssabuild.OriginParam || uses[0].Slot != 0 {
		return nil, false
	}
	if slot, seeded := this.OriginalReceiverSlot(); !seeded || slot != 0 {
		return nil, false
	}
	actualThis := false
	for _, param := range params {
		if r, ok := param.(*values.JavaRef); ok && r != nil && r.Id == this.Id {
			_, seeded := r.OriginalReceiverSlot()
			actualThis = actualThis || seeded
		}
	}
	erased, known := values.SourceTypeErasure(this.Type(), ctx)
	if !actualThis || !known || erased != "L"+c.obj.GetClassName()+";" {
		return nil, false
	}
	classSig, classKnown := nativeMethodLocalOriginalSignature(c.obj, c.obj.Attributes, c.Work)
	methodSig, methodKnown := nativeMethodLocalOriginalSignature(c.obj, method.Attributes, c.Work)
	if !classKnown || !methodKnown || classSig != ctx.ClassSig || methodSig != ctx.CurrentMethodSig {
		return nil, false
	}
	formals := types.ClassFormalTypeParamNames(classSig)
	if len(formals) > 64 {
		return nil, false
	}
	args := make([]types.JavaType, len(formals))
	for i, formal := range formals {
		for _, shadow := range types.MethodFormalTypeParamNames(methodSig) {
			if formal == shadow {
				return nil, false
			}
		}
		args[i] = types.NewJavaTypeVariable(formal)
	}
	view := this.Type().Copy()
	if len(args) != 0 {
		view = types.NewParameterizedType(strings.ReplaceAll(c.obj.GetClassName(), "/", "."), args)
	}
	proof.Object = values.NewJavaRef(utils.NewRootVariableId(), nil, view)
	return proof, true
}
