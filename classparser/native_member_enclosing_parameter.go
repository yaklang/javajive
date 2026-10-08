package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

type nativeMemberEnclosingParameter struct {
	member      *nativeMemberClass
	method      *MemberInfo
	constructor *nativeMemberConstructor
	descriptor  string
	loadPC      int
}

// A constructor may reuse its hidden enclosing parameter after initializing
// THIS. This is the same nullable lexical capture, not a nonnull qualifier.
// Bind it to the exact original member/constructor and unchanged physical slot;
// an ordinary same-typed parameter or a pre-delegation load grants no ownership.
func nativeMemberConstructorEnclosingParameter(obj *ClassObject, method *MemberInfo, code *CodeAttribute, ops []*core.OpCode, allocation int, current, target *nativeMemberClass, work *workbudget.Budget) *nativeMemberEnclosingParameter {
	if obj == nil || method == nil || code == nil || current == nil || target == nil || current.object != obj || current.static || target.static || current.owner != target.owner || method.AccessFlags&8 != 0 || len(obj.Methods) > 4096 || len(method.Attributes) > 4096 || len(ops) > 65535 || allocation < 0 || allocation+2 >= len(ops) || !nativeProofWork(work, int64(len(ops)+len(obj.Methods)+len(method.Attributes)+1)) || work != nil && work.CheckAlloc(96) != nil {
		return nil
	}
	name, nk := sourceBridgeUTF8(obj, method.NameIndex)
	descriptor, dk := sourceBridgeUTF8(obj, method.DescriptorIndex)
	params, result, err := callbinding.Descriptor(descriptor)
	ctor := current.constructors[descriptor]
	if !nk || !dk || name != "<init>" || err != nil || result != "V" || len(params) == 0 || params[0] != "L"+current.owner+";" || ctor == nil || ctor.descriptor != descriptor || ctor.delegatePC < 0 || !nativeMemberSuperCaptureDeclaration(current, work) {
		return nil
	}
	declarations := 0
	for _, candidate := range obj.Methods {
		if candidate == nil {
			return nil
		}
		n, nok := sourceBridgeUTF8(obj, candidate.NameIndex)
		d, dok := sourceBridgeUTF8(obj, candidate.DescriptorIndex)
		if !nok || !dok {
			return nil
		}
		if n == name && d == descriptor {
			if candidate != method {
				return nil
			}
			declarations++
		}
	}
	codeCount := 0
	for _, attr := range method.Attributes {
		if original, ok := attr.(*CodeAttribute); ok {
			if original != code {
				return nil
			}
			codeCount++
		}
	}
	if declarations != 1 || codeCount != 1 || len(code.Code) == 0 || len(code.Code) > 65535 || code.MaxLocals < 2 {
		return nil
	}
	newOp, duplicate, load := ops[allocation], ops[allocation+1], ops[allocation+2]
	if newOp == nil || newOp.Instr == nil || newOp.Instr.OpCode != core.OP_NEW || len(newOp.Data) != 2 || duplicate == nil || duplicate.Instr == nil || duplicate.Instr.OpCode != core.OP_DUP || load == nil || !constructorMotionLoad(load, params[0]) || core.GetRetrieveIdx(load) != 1 || int(newOp.CurrentOffset) <= ctor.delegatePC || int(load.CurrentOffset) <= int(newOp.CurrentOffset) {
		return nil
	}
	owner, known := sourceBridgeClassName(obj, core.Convert2bytesToInt(newOp.Data))
	if !known || target.object == nil || owner != target.object.GetClassName() {
		return nil
	}
	for _, op := range ops {
		if op == nil || op.Instr == nil {
			return nil
		}
		// A wide store at zero overlaps the enclosing slot as well. Preserve
		// the initial receiver and capture identity across the entire method.
		if slot := core.GetStoreIdx(op); slot == 0 || slot == 1 {
			return nil
		}
	}
	return &nativeMemberEnclosingParameter{member: current, method: method, constructor: ctor, descriptor: descriptor, loadPC: int(load.CurrentOffset)}
}

// The renderer checks the exact simulator parameter retained before source
// parameters are projected. A copied ref, mutable value, custom expression or
// another constructor cannot borrow the original bytecode certificate.
func (c *ClassObjectDumper) nativeMemberConstructorEnclosingOperand(value any, proof *nativeMemberEnclosingParameter, ctx *class_context.ClassContext) bool {
	if c == nil || proof == nil || ctx == nil || ctx.IsStatic || ctx.FunctionName != "<init>" || ctx.CurrentMethodDesc != proof.descriptor || c.obj == nil || strings.ReplaceAll(ctx.ClassName, ".", "/") != c.obj.GetClassName() || proof.member != c.nativeMemberCurrent || proof.member == nil || proof.member.object != c.obj || proof.method != c.CurrentMethod || proof.constructor != proof.member.constructors[proof.descriptor] || c.nativeConstructorEnclosing == nil || !nativeProofWork(c.Work, 4) {
		return false
	}
	v, ok := value.(values.JavaValue)
	if !ok {
		return false
	}
	v, ok = nativeMemberEnclosingUnpack(v, c.Work)
	if !ok || v != c.nativeConstructorEnclosing || !nativeMemberSourceEnclosingParameter(v, ctx, proof.member.owner) {
		return false
	}
	if slot, original := c.nativeConstructorEnclosing.OriginalParameterSlot(); !original || slot != 1 {
		return false
	}
	erasure, known := values.SourceTypeErasure(v.Type(), ctx)
	return known && erasure == "L"+proof.member.owner+";"
}

// Qualifying the TYPE selects the original sibling declaration without adding
// the null check of an expression-qualified NEW. A same-named member or method
// formal may shadow the short name. Retain the proved enclosing generic view;
// a raw outer with a parameterized child is not a legal Java allocation type.
func (c *ClassObjectDumper) nativeMemberEnclosingParameterAllocationName(plan *nativeMemberAllocation, family *nativeMemberFamily, ctx *class_context.ClassContext) (string, bool) {
	if c == nil || plan == nil || plan.child == nil || plan.child.object == nil || plan.enclosingParameter == nil || family == nil || family.failed || ctx == nil || !nativeProofWork(c.Work, 4) {
		return "", false
	}
	name, known := family.sourceName(plan.child.object.GetClassName())
	if !known || name == "" {
		return "", false
	}
	// A shadowed leading package/type identifier has no proven escape in this
	// namespace. Source shortening or a physical binary name cannot repair it.
	leading := strings.SplitN(name, ".", 2)[0]
	if ctx.LexicalTypeNames[leading] || ctx.IsTypeParam(leading) {
		return "", false
	}
	if plan.child.outerFormalCount == 0 {
		return name, true
	}
	if c.nativeOuterContext == nil || len(c.nativeOuterContext.ClassTypeParams) != plan.child.outerFormalCount || len(c.nativeOuterContext.ClassTypeParams) > 256 || c.Work != nil && c.Work.CheckAlloc(int64(len(c.nativeOuterContext.ClassTypeParams))*32) != nil {
		return "", false
	}
	args := make([]types.JavaType, len(c.nativeOuterContext.ClassTypeParams))
	for i, formal := range c.nativeOuterContext.ClassTypeParams {
		if !nativeProofWork(c.Work, 1) {
			return "", false
		}
		args[i] = types.NewJavaClass(formal)
	}
	view := types.NewParameterizedType(strings.ReplaceAll(plan.child.owner, "/", "."), args)
	if !nativeMemberEnclosingTypeDenotable(c, view) {
		return "", false
	}
	return view.String(ctx) + "." + plan.child.name, true
}
