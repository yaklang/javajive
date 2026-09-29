package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A raw List can be applicable to both List<? extends String> and
// Collection<? extends Number>; neither parameterized formal is more specific.
// Keep the bytecode-selected declaration by recovering its exact formal, only
// when its erasure already matches the argument. This adds no runtime check.
// Callee method variables are a separate namespace, even when their spelling
// matches a caller variable. They need inference, never a copied name.
func (f *FunctionCallExpression) parameterizedOverloadArgCast(i int, ctx *class_context.ClassContext) string {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || f.Descriptor == "" ||
		f.FunctionName == "<init>" || i < 0 || i >= len(f.Arguments) ||
		!f.isCurrentClass(ctx) {
		return ""
	}
	// Only the current declaring class provides a lexical accessibility proof
	// for its generic bounds. A foreign public method can mention a private
	// bound that cannot legally be named in this caller's cast.
	arg := f.Arguments[i]
	if arg == nil || arg.Type() == nil || isWitnessLambdaArg(UnpackSoltValue(arg)) {
		return ""
	}
	actual, raw := arg.Type().RawType().(*types.JavaClass)
	if !raw || actual == nil || !sameErasureClassName(witnessRawClassName(f.witnessDescriptorParamType(i)), actual.Name) {
		return ""
	}
	params, _, methodFormals := f.genericMethodSignature(ctx)
	if len(methodFormals) != 0 || i >= len(params) || !sourceDenotableJavaType(params[i], ctx) {
		return ""
	}
	// A class variable hidden by the caller's own method variable cannot be
	// spelled unambiguously at this call site. Equal names are not identities.
	for _, local := range types.MethodFormalTypeParamNames(ctx.CurrentMethodSig) {
		for _, class := range ctx.ClassTypeParams {
			if local == class && javaTypeMentionsNames(params[i], []string{class}) {
				return ""
			}
		}
	}
	formal, parameterized := types.AsParameterizedType(params[i])
	if !parameterized || !sameErasureClassName(formal.RawClassName, actual.Name) {
		return ""
	}
	kind := callbinding.Virtual
	if f.IsStatic {
		kind = callbinding.Static
	} else if f.Kind == InvokeInterface {
		kind = callbinding.Interface
	}
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	declaration, known := ctx.InvocationMetadata(owner)
	if !known || declaration.Name != owner || !declaration.MembersComplete {
		return ""
	}
	family, err := callbinding.FamilyOf(callbinding.Witness{
		Owner: owner, Name: f.FunctionName,
		Desc: f.Descriptor, Kind: kind,
	}, ctx.InvocationMetadata)
	// Positive evidence of a competing declaration does not require guessing
	// what an unavailable ancestor declares. We restore the selected formal;
	// this is not a claim that the entire overload family has been enumerated.
	if err != nil || family.Proof != callbinding.Compete || family.Target == nil || family.Target.Varargs || family.Target.Bridge {
		return ""
	}
	actualDescriptor := bindingType(arg.Type())
	for _, candidate := range family.Methods {
		if candidate.Static != f.IsStatic || candidate.Desc == f.Descriptor {
			continue
		}
		other, _, err := callbinding.Descriptor(candidate.Desc)
		if err != nil || i >= len(other) || other[i] == actualDescriptor || !callbinding.Reference(other[i]) {
			continue
		}
		assignable := callbinding.Assignable(actualDescriptor, other[i], ctx.InvocationMetadata)
		if !assignable && strings.HasPrefix(other[i], "L") {
			// The invocation catalog may omit a platform method table while the
			// type hierarchy still proves List <: Collection. Reuse that positive
			// subtype evidence without pretending its member table is complete.
			otherClass := strings.ReplaceAll(other[i][1:len(other[i])-1], "/", ".")
			assignable = types.IsReferenceSubtypeBridged(actual.Name, otherClass, ctx.SiblingSuperTypes)
		}
		if assignable {
			return params[i].String(ctx)
		}
	}
	return ""
}
