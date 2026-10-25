package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A fresh receiver's declaration can carry class-dependent source arguments
// even when its originating external field is represented only by its JVM
// descriptor. We need not guess that field's concrete type substitution:
// prove the producer's generic result and this consumer's class-dependent
// input separately. The surrounding result-use proof owns any raw view.
func (f *FunctionCallExpression) freshReceiverClassArgumentConstraints(meta callbinding.Class, ctx *class_context.ClassContext) bool {
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	if meta.Name != owner || !meta.MembersComplete || !meta.ParentsComplete || !f.HasOriginPC || f.OriginPC < 0 || f.OriginPC > 65535 || f.IsStatic || (f.Kind != InvokeVirtual && f.Kind != InvokeInterface) || !erasedResultUniqueTarget(meta, f) {
		return false
	}
	producer, known := UnpackSoltValue(f.Object).(*FunctionCallExpression)
	if !known || producer == nil || !producer.HasOriginPC || producer.OriginPC < 0 || producer.OriginPC > 65535 || producer.IsSpecialInvoke || (producer.Kind != InvokeVirtual && producer.Kind != InvokeInterface && producer.Kind != InvokeStatic) {
		return false
	}
	inputs, ret, err := callbinding.Descriptor(producer.Descriptor)
	if err != nil || len(inputs) != len(producer.Arguments) || ret != "L"+owner+";" {
		return false
	}
	declaring, classSig, signature := erasedInvocationDeclaration(ctx, strings.ReplaceAll(producer.ClassName, ".", "/"), producer.FunctionName, producer.Descriptor)
	if declaring == "" || !strings.HasPrefix(signature, "(") || !erasedResultSignatureWork(ctx, signature, classSig) {
		return false
	}
	erased, _, valid := types.EraseLexicalOwnerMethodSignatureWithThrows([]string{classSig}, signature)
	_, _, result := types.ParseMethodSignatureFull(signature, ctx)
	if _, parameterized := types.AsParameterizedType(result); !valid || erased != producer.Descriptor || !parameterized || bindingType(result) != ret {
		return false
	}
	return f.declaredClassArgumentConstraints(meta, ctx)
}

// A generic result alone places no class-dependent constraint on an input.
// Retain a parameterized receiver for methods whose inputs are independent of
// class binders: raw-erasing it would lose a caller's V result, including in a
// protected return where adding a compensating cast can move failure timing.
func (f *FunctionCallExpression) declaredClassArgumentConstraints(meta callbinding.Class, ctx *class_context.ClassContext) bool {
	bounds := erasedInvocationBounds(meta.Signature)
	if len(bounds) == 0 {
		return false
	}
	for _, method := range meta.Methods {
		if method.Name != f.FunctionName || method.Desc != f.Descriptor {
			continue
		}
		if !method.Generic || method.Static || method.Bridge || method.Varargs || !strings.HasPrefix(method.Signature, "(") || !erasedResultSignatureWork(ctx, method.Signature, meta.Signature) {
			return false
		}
		erased, _, valid := types.EraseLexicalOwnerMethodSignatureWithThrows([]string{meta.Signature}, method.Signature)
		if !valid || erased != f.Descriptor {
			return false
		}
		_, params, _ := types.ParseMethodSignatureFull(method.Signature, ctx)
		for _, param := range params {
			if methodTypeMentionsFormal(param, bounds) {
				return true
			}
		}
	}
	return false
}
