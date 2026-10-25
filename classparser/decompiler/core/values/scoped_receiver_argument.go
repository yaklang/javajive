package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A parameterized receiver binds its class variables before argument typing.
// Restore that binder only from an exact original declaration and a complete,
// unique overload family. The binder and existing operand must have identical
// erasure, which can widen to the physical formal: substituting a recursively
// bounded caller variable into an Object-bounded interface does not make that
// variable's bound Object. The source view adds no narrower runtime check.
// Platform and archive declarations use the same metadata proof. No API names
// or guessed class signatures participate in this rule.
func (f *FunctionCallExpression) scopedReceiverArgument(i int, ctx *class_context.ClassContext) types.JavaType {
	if f == nil || ctx == nil || f.IsStatic || f.IsSpecialInvoke || f.Object == nil ||
		!f.HasOriginPC || f.OriginPC < 0 || f.OriginPC > 65535 ||
		(f.Kind != InvokeInterface && f.Kind != InvokeVirtual) ||
		i < 0 || i >= len(f.Arguments) || ctx.Getenv("JDEC_SCOPED_RECEIVER_ARGUMENT_OFF") != "" {
		return nil
	}
	params, result, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || len(params) != len(f.Arguments) || f.FuncType == nil || len(f.FuncType.ParamTypes) != len(params) ||
		bindingType(f.FuncType.ReturnType) != result || bindingType(f.FuncType.ParamTypes[i]) != params[i] {
		return nil
	}
	proof, known := boundedErasedResultContext(ctx)
	if !known {
		return nil
	}
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	raw, args := f.receiverParamTypeArgs(ctx)
	if strings.ReplaceAll(raw, ".", "/") != owner || len(args) == 0 || len(args) > 128 {
		return nil
	}
	meta, known := proof.InvocationMetadata(owner)
	if !known || !meta.Public || !meta.MembersComplete || !meta.ParentsComplete ||
		meta.IsInterface != (f.Kind == InvokeInterface) || !erasedResultUniqueTarget(meta, f) {
		return nil
	}
	family, err := callbinding.FamilyOf(callbinding.Witness{Owner: owner, Name: f.FunctionName, Desc: f.Descriptor}, proof.InvocationMetadata)
	if err != nil || !family.Complete || family.Proof != callbinding.Unique || len(family.Methods) != 1 || family.Target == nil {
		return nil
	}
	m := family.Target
	if m.Static || m.Bridge || m.Varargs || !m.Public || !m.Generic || len(m.Signature) > 8192 ||
		!strings.HasPrefix(m.Signature, "(") || !erasedResultSignatureWork(proof, m.Signature, meta.Signature) {
		return nil
	}
	erased, _, valid := types.EraseLexicalOwnerMethodSignatureWithThrows([]string{meta.Signature}, m.Signature)
	if !valid || erased != f.Descriptor {
		return nil
	}
	formals := types.ClassFormalTypeParamNames(meta.Signature)
	if len(formals) != len(args) {
		return nil
	}
	sigma := map[string]types.JavaType{}
	for n, name := range formals {
		if args[n] == nil || types.IsWildcardType(args[n]) {
			return nil
		}
		sigma[name] = args[n]
	}
	_, declared, _ := types.ParseMethodSignatureFull(m.Signature, proof)
	if len(declared) != len(params) {
		return nil
	}
	target := types.SubstituteTypeVars(declared[i], sigma)
	name, variable := types.TypeVariableName(target)
	if !variable || !ctx.IsTypeParam(name) {
		return nil
	}
	for _, shadow := range types.MethodFormalTypeParamNames(ctx.CurrentMethodSig) {
		if name == shadow {
			return nil
		}
	}
	physical, known := SourceTypeErasure(target, proof)
	if !known || !callbinding.Assignable(physical, params[i], proof.InvocationMetadata) || !ScopedErasureView(proof, target, f.Arguments[i]) {
		return nil
	}
	return target
}
