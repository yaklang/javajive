package core

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// An ordinary invocation needs its own source-view proof. A constructor's
// operand context does not establish one for a nested call. Here the original
// declared method has no generic Signature and every source formal
// retains its physical descriptor. Its final array operand can therefore keep
// the same non-poly view when a private DUP local is replaced by its allocation.
// This is discovery evidence only; complete ownership and motion are separate.
func (d *Decompiler) ordinaryInvocationArraySourceView(call *values.FunctionCallExpression, invoke *OpCode) bool {
	if d == nil || d.FunctionContext == nil || d.FunctionContext.InvocationMetadata == nil || call == nil ||
		call.IsSpecialInvoke || !call.HasOriginPC ||
		call.FunctionName == "<init>" || call.FunctionName == "<clinit>" || call.FuncType == nil ||
		len(call.Arguments) == 0 || len(call.Arguments) > 128 || len(call.Descriptor) > 8192 || len(call.ClassName) > 65535 ||
		invoke == nil || invoke.Instr == nil ||
		int(invoke.CurrentOffset) != call.OriginPC || !sameBranchArrayInvocation(d.invokeFuncCall[invoke], call) ||
		!d.delegationArrayCallOperands(call, invoke) {
		return false
	}
	// Match the original invocation category. An instance call additionally
	// retains the exact erased receiver view; a constructor or another call's
	// context cannot grant that source view to this operand.
	static := call.IsStatic && call.Kind == values.InvokeStatic && invoke.Instr.OpCode == OP_INVOKESTATIC
	instance := !call.IsStatic && (call.Kind == values.InvokeVirtual && invoke.Instr.OpCode == OP_INVOKEVIRTUAL || call.Kind == values.InvokeInterface && invoke.Instr.OpCode == OP_INVOKEINTERFACE)
	if !static && !instance {
		return false
	}
	if instance {
		// A real fixed source declaration supplies lookup evidence. A
		// generic/opaque producer expression does not acquire that evidence
		// from the JVM result erasure or from this array's formal type.
		ref, declared := values.UnpackSoltValue(call.Object).(*values.JavaRef)
		if !declared || ref == nil || ref.CustomValue != nil || ref.StackVar != nil || ref.Type() == nil {
			return false
		}
		raw, nominal := types.RawClassFQN(ref.Type())
		if !nominal || strings.ReplaceAll(raw, ".", "/") != strings.ReplaceAll(call.ClassName, ".", "/") {
			return false
		}
		receiver, known := values.SourceTypeErasure(ref.Type(), d.FunctionContext)
		if !known || receiver != "L"+strings.ReplaceAll(call.ClassName, ".", "/")+";" {
			return false
		}
	}
	if d.Work != nil && (d.Work.CheckAlloc(int64(len(call.Descriptor)+len(call.ClassName)+len(call.Arguments)+1)*64) != nil ||
		d.Work.Charge(workbudget.CounterGraphScans, int64(len(call.Descriptor))+1) != nil) {
		return false
	}
	params, result, err := callbinding.Descriptor(call.Descriptor)
	if err != nil || len(params) != len(call.Arguments) || len(params) != len(call.FuncType.ParamTypes) ||
		!strings.HasPrefix(params[len(params)-1], "[") {
		return false
	}
	ctx := d.FunctionContext
	for i, param := range call.FuncType.ParamTypes {
		if descriptor, known := values.SourceTypeErasure(param, ctx); !known || descriptor != params[i] {
			return false
		}
	}
	if descriptor, known := values.SourceTypeErasure(call.FuncType.ReturnType, ctx); !known || descriptor != result {
		return false
	}
	owner := strings.ReplaceAll(call.ClassName, ".", "/")
	metadata, known := ctx.InvocationMetadata(owner)
	if !known || metadata.Name != owner || !metadata.MembersComplete || len(metadata.Methods) > 4096 ||
		d.Work != nil && (d.Work.CheckAlloc(int64(len(metadata.Methods)+1)*16) != nil ||
			d.Work.Charge(workbudget.CounterGraphScans, int64(len(metadata.Methods))+int64(len(call.Descriptor))+1) != nil) {
		return false
	}
	if instance && (!metadata.Public || metadata.IsInterface != (call.Kind == values.InvokeInterface)) {
		return false
	}
	found := 0
	for _, method := range metadata.Methods {
		if method.Name == call.FunctionName && method.Desc == call.Descriptor {
			if method.Static != call.IsStatic || instance && !method.Public || method.Generic || method.Bridge || method.Signature != "" {
				return false
			}
			found++
		}
	}
	return found == 1
}

// Reopen the entire original allocation/fill/use packet before bypassing the
// general DUP prohibition. Removed initializer stores are the only extra uses;
// no source local, publication, extra call, changed effect window or alternate
// entry can inherit the final invocation's ownership.
func (d *Decompiler) ordinaryInvocationArrayPacket(call *values.FunctionCallExpression, ref *values.JavaRef, array *values.NewExpression) bool {
	if call == nil || array == nil || ref == nil || ref.CustomValue != nil || ref.StackVar != nil || !array.HasOriginPC || !array.HasEvaluationEndPC ||
		len(call.Arguments) == 0 || !delegationArraySameRef(call.Arguments[len(call.Arguments)-1], ref) {
		return false
	}
	allocation, invoke := d.opcodeAtOffset(array.OriginPC), d.opcodeAtOffset(call.OriginPC)
	if !d.ordinaryInvocationArraySourceView(call, invoke) || allocation == nil || len(allocation.Target) != 1 ||
		!d.privateCompletedArrayAtUse(array, ref, allocation.Target[0], invoke, 0, map[*values.NewExpression]bool{}, 0) {
		return false
	}
	if d.Work != nil && d.Work.CheckAlloc(int64(len(d.opCodes)+len(array.Initializer)+1)*32) != nil {
		return false
	}
	var stores []*OpCode
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}
		if int(op.CurrentOffset) > array.OriginPC && int(op.CurrentOffset) <= array.EvaluationEndPC &&
			isArrayElementStore(op.Instr.OpCode) && len(op.stackConsumed) == 3 && delegationArraySameRef(op.stackConsumed[2], ref) {
			stores = append(stores, op)
		}
	}
	return len(stores) == len(array.Initializer) && d.privateArrayValueDAG(allocation, invoke, ref, array, stores, array.Initializer, 0, nil)
}
