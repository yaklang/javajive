package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Java does not infer a receiver factory's method variables from a subsequent
// member invocation's arguments. A caller's T and factory's fresh T therefore
// need not agree, even with identically spelled recursive bounds. At a fixed
// primitive/void consumer, use the same physical receiver raw instead. JLS 4.8
// erases the class member's inputs; no result constraint flows into the factory.
//
// This finite proof requires a zero-input, uniquely bound static factory and
// one original class-generic member declaration. It never instantiates either
// binder, changes the producer, or recursively plans an argument. Original
// parameter/CHECKCAST evidence makes every new argument view a widening, so
// bridge checks, callback exceptions and evaluation order stay at their JVM
// boundaries. Method-generic consumers and inferred/poly operands need their
// own proof and supply no permission here.
func (f *FunctionCallExpression) planErasedFactoryReceiver(ctx *class_context.ClassContext) (*FunctionCallExpression, bool) {
	if f == nil || ctx == nil || f.bindingPlanned || !f.HasOriginPC || f.OriginPC < 0 || f.OriginPC > 65535 ||
		f.IsStatic || f.IsSpecialInvoke || f.FunctionName == "<init>" ||
		(f.Kind != InvokeVirtual && f.Kind != InvokeInterface) {
		return nil, false
	}
	producer, original := UnpackSoltValue(f.Object).(*FunctionCallExpression)
	if !original || producer == nil || !producer.HasOriginPC || producer.OriginPC < 0 || producer.OriginPC > 65535 ||
		producer.IsSpecialInvoke || !producer.IsStatic || producer.Kind != InvokeStatic || producer.FunctionName == "<init>" ||
		len(producer.Arguments) != 0 || producer.FuncType == nil || producer.FuncType.ReturnType == nil {
		return nil, false
	}
	params, result, err := callbinding.Descriptor(f.Descriptor)
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	receiver := "L" + owner + ";"
	inputs, produced, factoryErr := callbinding.Descriptor(producer.Descriptor)
	if err != nil || factoryErr != nil || len(params) == 0 || len(params) != len(f.Arguments) || len(inputs) != 0 ||
		callbinding.Reference(result) || produced != receiver || bindingType(producer.Type()) != receiver {
		return nil, false
	}
	if f.FuncType == nil || f.FuncType.ReturnType == nil || len(f.FuncType.ParamTypes) != len(params) ||
		bindingType(f.FuncType.ReturnType) != result || f.SourceReturnType != nil && bindingType(f.SourceReturnType) != result {
		return nil, false
	}
	proof, known := boundedErasedResultContext(ctx)
	if !known || !erasedZeroInputFactoryDeclaration(producer, receiver, proof) {
		return nil, false
	}
	declaring, classSig, signature := erasedInvocationDeclaration(proof, owner, f.FunctionName, f.Descriptor)
	if declaring != owner || len(signature) > 8192 || !strings.HasPrefix(signature, "(") || strings.Contains(signature, "^") ||
		!erasedResultSignatureWork(proof, signature, classSig) {
		return nil, false
	}
	meta, known := proof.InvocationMetadata(owner)
	if !known || meta.Signature != classSig || meta.IsInterface != (f.Kind == InvokeInterface) ||
		!erasedFactoryDeclarationAccessible(owner, meta.Public, ctx) || !erasedResultUniqueTarget(meta, f) {
		return nil, false
	}
	erased, _, valid := types.EraseLexicalOwnerMethodSignatureWithThrows([]string{classSig}, signature)
	_, declared, ret := types.ParseMethodSignatureFull(signature, proof)
	if !valid || erased != f.Descriptor || len(declared) != len(params) || ret == nil || bindingType(ret) != result {
		return nil, false
	}
	classFormals := map[string]string{}
	for _, name := range types.ClassFormalTypeParamNames(classSig) {
		classFormals[name] = "class binder"
	}
	usesFormal := false
	for _, parameter := range declared {
		usesFormal = usesFormal || methodTypeMentionsFormal(parameter, classFormals)
	}
	if !usesFormal {
		return nil, false
	}
	family, err := callbinding.FamilyOf(callbinding.Witness{Owner: owner, Name: f.FunctionName, Desc: f.Descriptor}, proof.InvocationMetadata)
	if err != nil || !family.Complete || family.Proof != callbinding.Unique || len(family.Methods) != 1 || family.Target == nil ||
		family.Target.Signature != signature || !family.Target.Generic || !family.Target.Public || family.Target.Static || family.Target.Bridge ||
		family.Target.Varargs || !family.Target.ExceptionsKnown || len(family.Target.Exceptions) != 0 {
		return nil, false
	}
	for i, argument := range f.Arguments {
		if !erasedFactoryFixedOperand(argument) {
			return nil, false
		}
		physical := declaredArrayOperandDescriptor(argument, ctx)
		if IsNullLiteral(UnpackSoltValue(argument)) {
			physical = "null"
		}
		if physical == "" || !callbinding.Assignable(physical, params[i], proof.InvocationMetadata) {
			return nil, false
		}
	}
	if proof.Work != nil && proof.Work.Charge(workbudget.CounterNodeCopies, int64(len(params))+2) != nil {
		return nil, false
	}
	out := f.Clone()
	out.Object = &CastExpression{Value: f.Object, TargetType: types.NewJavaClass(strings.ReplaceAll(owner, "/", ".")), Binding: true, OriginPC: f.OriginPC}
	for i, descriptor := range params {
		if callbinding.Reference(descriptor) {
			typ, _ := types.ParseDescriptor(descriptor)
			out.Arguments[i] = &CastExpression{Value: f.Arguments[i], TargetType: typ, Binding: true, OriginPC: f.OriginPC}
		}
	}
	out.bindingPlanned = true
	return out, true
}

func erasedFactoryFixedOperand(value JavaValue) bool {
	switch v := UnpackSoltValue(value).(type) {
	case *JavaRef:
		return v != nil && v.StackVar == nil && v.CustomValue == nil
	case *CastExpression:
		// The caller separately requires an original CHECKCAST witness. This
		// existing cast already fixes its child's inference context.
		return v != nil
	case *JavaLiteral:
		return IsNullLiteral(v)
	default:
		return false
	}
}

func erasedZeroInputFactoryDeclaration(f *FunctionCallExpression, result string, ctx *class_context.ClassContext) bool {
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	declaring, classSig, signature := erasedInvocationDeclaration(ctx, owner, f.FunctionName, f.Descriptor)
	if declaring != owner || len(signature) > 8192 || !strings.HasPrefix(signature, "<") || strings.Contains(signature, "^") ||
		!erasedResultSignatureWork(ctx, signature, classSig) {
		return false
	}
	meta, known := ctx.InvocationMetadata(owner)
	if !known || !erasedFactoryDeclarationAccessible(owner, meta.Public, ctx) || !erasedResultUniqueTarget(meta, f) {
		return false
	}
	family, err := callbinding.FamilyOf(callbinding.Witness{Owner: owner, Name: f.FunctionName, Desc: f.Descriptor}, ctx.InvocationMetadata)
	if err != nil || !family.Complete || family.Proof != callbinding.Unique || len(family.Methods) != 1 || family.Target == nil ||
		family.Target.Signature != signature || !family.Target.Public || !family.Target.Static || !family.Target.Generic ||
		family.Target.Bridge || family.Target.Varargs || !family.Target.ExceptionsKnown || len(family.Target.Exceptions) != 0 {
		return false
	}
	// A static factory cannot borrow a class or enclosing binder. Its own
	// method Signature is checked against the original table above; the
	// class Signature supplies no substitution here. In particular an absent
	// class attribute and a resolver's normalized Object parent are equivalent
	// for this proof. Validate every method reference in its own binder scope,
	// including recursive/dependent formal bounds.
	formals, references, valid := types.SignatureTypeVariableReferences(signature)
	owned := map[string]string{}
	for _, name := range formals {
		owned[name] = "method binder"
	}
	for _, name := range references {
		if owned[name] == "" {
			return false
		}
	}
	erased, _, erasedOK := types.EraseLexicalOwnerMethodSignatureWithThrows(nil, signature)
	_, parameters, ret := types.ParseMethodSignatureFull(signature, ctx)
	if !valid || !erasedOK || erased != f.Descriptor || len(parameters) != 0 || ret == nil || bindingType(ret) != result || !methodTypeMentionsFormal(ret, owned) {
		return false
	}
	_, parameterized := types.AsParameterizedType(ret)
	return parameterized
}

// Declaration access and source ownership are separate facts. Complete
// invocation metadata carries accessibility relative to this output package;
// a source resolver's unknown does not negate that positive declaration fact.
// A known denial or lexical type-variable shadowing still refuses the view.
// This grants no ownership of the dependency's source or member bodies.
func erasedFactoryDeclarationAccessible(owner string, public bool, ctx *class_context.ClassContext) bool {
	if !strings.Contains(owner, "/") && ctx.IsTypeParam(owner) {
		return false
	}
	if ctx.SiblingClassAccessible != nil {
		if accessible, known := ctx.SiblingClassAccessible(owner); known {
			return accessible
		}
	}
	return public
}
