package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A result consumer can own erasure even when the getter has no arguments.
// Argument-driven inference cannot discover this obligation: a factory may
// look raw in the value model while Java infers Cell<Object>. Preserve that
// producer unchanged, view its same physical receiver class raw, and consume
// the result at the caller's independently established descriptor boundary.
// This is not permission to change a factory's array argument to E[]: such a
// change can steal another overload or change a downstream generic consumer.
func (f *FunctionCallExpression) planErasedZeroArgumentResult(ctx *class_context.ClassContext) (*FunctionCallExpression, bool) {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || ctx.SiblingClassSig == nil ||
		f.bindingPlanned || !f.HasOriginPC || f.OriginPC < 0 || f.OriginPC > 65535 ||
		f.IsStatic || f.IsSpecialInvoke || f.FunctionName == "<init>" ||
		(f.Kind != InvokeVirtual && f.Kind != InvokeInterface) || len(f.Arguments) != 0 || f.Object == nil {
		return nil, false
	}
	params, result, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || len(params) != 0 || !strings.HasPrefix(result, "L") {
		return nil, false
	}
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	receiver := "L" + owner + ";"
	if !erasedZeroArgumentReceiver(f.Object, receiver, ctx) {
		return nil, false
	}
	if parameter, known := UnpackSoltValue(f.Object).(*JavaRef); known {
		if _, parameterized := types.AsParameterizedType(parameter.Type()); parameterized {
			// Retain an already materialized parameter's source arguments.
			// This finite repair addresses erased factory/raw-entry views;
			// conflicting known parameterizations have their own use proof.
			return nil, false
		}
	}
	proof, known := boundedErasedResultContext(ctx)
	if !known {
		return nil, false
	}
	declaring, classSig, signature := erasedInvocationDeclaration(proof, owner, f.FunctionName, f.Descriptor)
	// The raw view keeps the same symbolic owner and access qualification.
	// Widening to an inherited declaring class needs a different access proof.
	if declaring != owner || len(signature) > 8192 || !strings.HasPrefix(signature, "()") || strings.Contains(signature, "^") {
		return nil, false
	}
	if !erasedResultSignatureWork(proof, signature, classSig) {
		return nil, false
	}
	erased, _, valid := types.EraseLexicalOwnerMethodSignatureWithThrows([]string{classSig}, signature)
	_, _, declared := types.ParseMethodSignatureFull(signature, proof)
	if !valid || erased != f.Descriptor || declared == nil || declared.IsArray() || bindingType(declared) != result ||
		!methodTypeMentionsFormal(declared, erasedInvocationBounds(classSig)) {
		return nil, false
	}
	if _, parameterized := types.AsParameterizedType(declared); !parameterized {
		return nil, false
	}
	family, err := callbinding.FamilyOf(callbinding.Witness{Owner: owner, Name: f.FunctionName, Desc: f.Descriptor}, proof.InvocationMetadata)
	meta, available := proof.InvocationMetadata(owner)
	if err != nil || !available || meta.Name != owner || meta.Signature != classSig || !meta.MembersComplete || !meta.ParentsComplete ||
		meta.IsInterface != (f.Kind == InvokeInterface) ||
		!family.Complete || family.Target == nil || family.Target.Signature != signature || !family.Target.Generic || family.Target.Static || family.Target.Bridge || family.Target.Varargs ||
		!family.Target.ExceptionsKnown || len(family.Target.Exceptions) != 0 || !erasedResultClassAccessible(owner, meta.Public, ctx) {
		return nil, false
	}
	if !erasedResultUniqueTarget(meta, f) {
		return nil, false
	}
	for _, rival := range family.Methods {
		p, _, e := callbinding.Descriptor(rival.Desc)
		if e != nil || len(p) == 0 && rival.Desc != f.Descriptor {
			return nil, false
		}
	}
	object := f.Object
	if producer, ok := UnpackSoltValue(object).(*FunctionCallExpression); ok {
		// The receiver is already an erased result consumer. Propagate that
		// permission through proved fluent edges, keeping their argument tuples.
		if planned, ok := producer.PlanErasedResultChain(proof, receiver); ok {
			remaining := 128
			if erasedClassOnlyProjection(producer, planned, proof, &remaining) {
				object = planned
			}
		}
	}
	if proof.Work != nil && proof.Work.Charge(workbudget.CounterNodeCopies, 2) != nil {
		return nil, false
	}
	out := f.Clone()
	out.Object = &CastExpression{Value: object, TargetType: types.NewJavaClass(strings.ReplaceAll(owner, "/", ".")), Binding: true, OriginPC: f.OriginPC}
	out.bindingPlanned = true
	return out, true
}

// An ordinary method's class formals do not participate in method inference.
// For one exact this-call, a declared Container<E> parameter can therefore own
// a same-erasure result view of a zero-argument getter. Spell the full selected
// parameter outside its raw bridge so a parameterized rival cannot steal the
// call. Method formals, raw receivers, multiple arguments and unknown overload
// relationships deliberately supply no permission.
func (f *FunctionCallExpression) erasedClassArgumentResultView(ctx *class_context.ClassContext) JavaValue {
	if f == nil || ctx == nil || ctx.IsStatic || f.IsStatic || f.bindingPlanned || !f.HasOriginPC || f.OriginPC < 0 || f.OriginPC > 65535 ||
		(f.Kind != InvokeVirtual && f.Kind != InvokeSpecial) || f.FunctionName == "<init>" ||
		len(f.Arguments) != 1 || !sameErasureClassName(f.ClassName, ctx.ClassName) {
		return nil
	}
	this, known := UnpackSoltValue(f.Object).(*JavaRef)
	if !known || this == nil || !this.IsThis {
		return nil
	}
	if slot, original := this.OriginalReceiverSlot(); !original || slot != 0 {
		return nil
	}
	if name, original := types.RawClassFQN(this.Type()); !original || !sameErasureClassName(name, ctx.ClassName) {
		return nil
	}
	getter, known := UnpackSoltValue(f.Arguments[0]).(*FunctionCallExpression)
	if !known || getter == nil || len(getter.Arguments) != 0 {
		return nil
	}
	physical, _, err := callbinding.Descriptor(f.Descriptor)
	_, result, resultErr := callbinding.Descriptor(getter.Descriptor)
	if err != nil || resultErr != nil || len(physical) != 1 || !strings.HasPrefix(physical[0], "L") || result != physical[0] {
		return nil
	}
	proof, known := boundedErasedResultContext(ctx)
	if !known {
		return nil
	}
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	declaring, classSig, signature := erasedInvocationDeclaration(proof, owner, f.FunctionName, f.Descriptor)
	if declaring != owner || classSig != ctx.ClassSig || len(signature) > 8192 || !strings.HasPrefix(signature, "(") {
		return nil
	}
	if !erasedResultSignatureWork(proof, signature, classSig) {
		return nil
	}
	erased, _, valid := types.EraseLexicalOwnerMethodSignatureWithThrows([]string{classSig}, signature)
	_, refs, bound := types.SignatureTypeVariableReferences(signature)
	_, params, _ := types.ParseMethodSignatureFull(signature, proof)
	if !valid || !bound || erased != f.Descriptor || err != nil || len(physical) != 1 || len(params) != 1 || params[0] == nil || params[0].IsArray() ||
		bindingType(params[0]) != physical[0] || !sourceDenotableJavaType(params[0], ctx) {
		return nil
	}
	if _, parameterized := types.AsParameterizedType(params[0]); !parameterized {
		return nil
	}
	classFormals := map[string]bool{}
	for _, name := range types.ClassFormalTypeParamNames(classSig) {
		classFormals[name] = true
	}
	for _, name := range refs {
		if !classFormals[name] || !ctx.IsTypeParam(name) || ctx.RawEraseTypeVars[name] || ctx.StandaloneEraseTypeVars[name] != "" || ctx.ForceParamEraseTypeVars[name] != "" {
			return nil
		}
		for _, shadow := range types.MethodFormalTypeParamNames(ctx.CurrentMethodSig) {
			if name == shadow {
				return nil
			}
		}
	}
	family, err := callbinding.FamilyOf(callbinding.Witness{Owner: owner, Name: f.FunctionName, Desc: f.Descriptor}, proof.InvocationMetadata)
	meta, available := proof.InvocationMetadata(owner)
	if err != nil || !available || meta.Signature != classSig || !meta.MembersComplete || !meta.ParentsComplete || !family.Complete || family.Target == nil ||
		family.Target.Signature != signature || !family.Target.Generic || family.Target.Static || family.Target.Bridge || family.Target.Varargs {
		return nil
	}
	if !erasedResultUniqueTarget(meta, f) {
		return nil
	}
	for _, rival := range family.Methods {
		p, _, e := callbinding.Descriptor(rival.Desc)
		if e != nil || rival.Varargs {
			return nil
		}
		if rival.Desc == f.Descriptor || len(p) != 1 {
			continue
		}
		if p[0] == physical[0] || !callbinding.Reference(p[0]) {
			return nil
		}
		// A named container cannot acquire an array conversion. For other
		// heads require the entire nominal hierarchy, not a failed lookup.
		if strings.HasPrefix(p[0], "[") {
			continue
		}
		accepts, complete := checkCastAncestorClosure(callbinding.Name(physical[0]), callbinding.Name(p[0]), proof.InvocationMetadata)
		if !complete || accepts && rival.Generic {
			return nil
		}
	}
	planned, known := getter.planErasedZeroArgumentResult(proof)
	if !known {
		return nil
	}
	if proof.Work != nil && proof.Work.Charge(workbudget.CounterNodeCopies, 1) != nil {
		return nil
	}
	return &CastExpression{Value: planned, TargetType: params[0], OriginPC: getter.OriginPC}
}

func erasedResultUniqueTarget(meta callbinding.Class, f *FunctionCallExpression) bool {
	matches := 0
	for _, method := range meta.Methods {
		if method.Name == f.FunctionName && method.Desc == f.Descriptor {
			matches++
		}
	}
	return matches == 1
}

func erasedResultSignatureWork(ctx *class_context.ClassContext, method, class string) bool {
	return ctx.Work == nil || ctx.Work.CheckAlloc(int64(len(method)+len(class))*32+16384) == nil &&
		ctx.Work.Charge(workbudget.CounterGraphScans, int64(len(method))+1) == nil
}

// The new result permission erases class members, not method inference. Check
// every changed node of the optional fluent plan. New binding casts must be
// widenings of original descriptor-seeded operands; an unchanged child stays
// untouched. This also prevents a nested method-generic factory from borrowing
// the outer getter's result consumer.
func erasedClassOnlyProjection(before, after JavaValue, ctx *class_context.ClassContext, remaining *int) bool {
	*remaining--
	if *remaining < 0 || isNilJavaValue(before) || isNilJavaValue(after) ||
		ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
		return false
	}
	if sameOriginalValueIdentity(before, after) {
		return true
	}
	if cast, known := after.(*CastExpression); known {
		if !cast.Binding {
			return false // a copied original cast needs a separate inner proof
		}
		physical := declaredArrayOperandDescriptor(before, ctx)
		if literal, known := before.(*JavaLiteral); known && IsNullLiteral(literal) {
			physical = "null"
		}
		target := ReferenceTypeDescriptor(cast.TargetType, ctx)
		if physical == "" || target == "" || !callbinding.Assignable(physical, target, ctx.InvocationMetadata) {
			return false
		}
		return erasedClassOnlyProjection(before, cast.Value, ctx, remaining)
	}
	original, known := before.(*FunctionCallExpression)
	projected, copied := after.(*FunctionCallExpression)
	if !known || !copied || !original.HasOriginPC || original.Witness() != projected.Witness() || original.IsStatic || projected.IsStatic ||
		original.IsSpecialInvoke || projected.IsSpecialInvoke || len(original.Arguments) != len(projected.Arguments) {
		return false
	}
	owner := strings.ReplaceAll(original.ClassName, ".", "/")
	declaring, classSig, signature := erasedInvocationDeclaration(ctx, owner, original.FunctionName, original.Descriptor)
	if declaring == "" || len(types.ClassFormalTypeParamNames(classSig)) == 0 || !strings.HasPrefix(signature, "(") || len(signature) > 8192 ||
		!erasedResultSignatureWork(ctx, signature, classSig) {
		return false
	}
	erased, _, valid := types.EraseLexicalOwnerMethodSignatureWithThrows([]string{classSig}, signature)
	if !valid || erased != original.Descriptor || !erasedClassOnlyProjection(original.Object, projected.Object, ctx, remaining) {
		return false
	}
	for i, argument := range original.Arguments {
		if !erasedClassOnlyProjection(argument, projected.Arguments[i], ctx, remaining) {
			return false
		}
	}
	return true
}

func erasedZeroArgumentReceiver(value JavaValue, descriptor string, ctx *class_context.ClassContext) bool {
	switch v := UnpackSoltValue(value).(type) {
	case *FunctionCallExpression:
		if v == nil || !v.HasOriginPC || v.OriginPC < 0 || v.OriginPC > 65535 || v.IsSpecialInvoke ||
			v.FuncType == nil || v.FuncType.ReturnType == nil ||
			(v.Kind != InvokeStatic && v.Kind != InvokeVirtual && v.Kind != InvokeInterface) {
			return false
		}
		_, result, err := callbinding.Descriptor(v.Descriptor)
		return err == nil && result == descriptor && bindingType(v.Type()) == descriptor
	case *JavaRef:
		if v == nil || v.StackVar != nil || v.CustomValue != nil {
			return false
		}
		if slot, known := v.OriginalReceiverSlot(); known && slot == 0 && !ctx.IsStatic {
			return descriptor == "L"+strings.ReplaceAll(ctx.ClassName, ".", "/")+";"
		}
		return declaredArrayOperandDescriptor(v, ctx) == descriptor
	default:
		return false
	}
}

func erasedResultClassAccessible(owner string, public bool, ctx *class_context.ClassContext) bool {
	if !strings.Contains(owner, "/") && ctx.IsTypeParam(owner) {
		return false
	}
	if ctx.SiblingClassAccessible != nil {
		accessible, known := ctx.SiblingClassAccessible(owner)
		return known && accessible
	}
	pkg := func(name string) string {
		name = strings.ReplaceAll(name, ".", "/")
		if i := strings.LastIndexByte(name, '/'); i >= 0 {
			return name[:i]
		}
		return ""
	}
	return public || pkg(owner) == pkg(ctx.ClassName)
}

// Existing hierarchy solvers have finite structural limits. Also charge their
// warmed metadata queries to this request; cached declarations are not free.
func boundedErasedResultContext(ctx *class_context.ClassContext) (*class_context.ClassContext, bool) {
	if ctx == nil || ctx.InvocationMetadata == nil || ctx.SiblingClassSig == nil ||
		ctx.Work != nil && ctx.Work.CheckAlloc(16384) != nil {
		return nil, false
	}
	take := func(units int64) bool {
		return ctx.Work == nil || ctx.Work.Charge(workbudget.CounterGraphScans, units) == nil
	}
	if !take(1) {
		return nil, false
	}
	if ctx.Work != nil && ctx.Work.Charge(workbudget.CounterNodeCopies, 1) != nil {
		return nil, false
	}
	out := *ctx
	remaining := 256
	out.InvocationMetadata = func(name string) (callbinding.Class, bool) {
		remaining--
		if remaining < 0 || !take(1) {
			return callbinding.Class{}, false
		}
		c, known := ctx.InvocationMetadata(name)
		if !known || c.Name != name || len(c.Methods) > 256 || len(c.Parents) > 64 || len(c.Signature) > 8192 ||
			!take(int64(len(c.Methods)+len(c.Parents)+len(c.Signature))+1) {
			return callbinding.Class{}, false
		}
		return c, true
	}
	out.SiblingClassSig = func(name string) (string, map[string]string, bool) {
		remaining--
		if remaining < 0 || !take(1) {
			return "", nil, false
		}
		signature, methods, known := ctx.SiblingClassSig(name)
		if !known || len(signature) > 8192 || len(methods) > 256 || !take(int64(len(signature)+len(methods))+1) {
			return "", nil, false
		}
		return signature, methods, true
	}
	return &out, true
}
